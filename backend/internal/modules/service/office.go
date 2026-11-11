package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/evinki/platform/internal/app"
	"github.com/evinki/platform/internal/modules/surveys"
	"github.com/evinki/platform/internal/modules/warehouse"
	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
)

func Mount(r chi.Router, d *app.Deps) {
	m := &mod{d}
	m.mountTech(r)

	// customer portal: ask for service on a guarantee
	r.With(auth.Require(auth.PCustomerPortal)).Post("/my/service-requests", httpx.H(m.customerRequest))
	r.With(auth.Require(auth.PCustomerPortal)).Get("/my/repairs", httpx.H(m.customerRepairs))

	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PServiceRead))
		r.Get("/admin/pipeline", httpx.H(m.pipeline))
		r.Get("/admin/repairs", httpx.H(m.repairs))
		r.Get("/admin/repairs/{id}", httpx.H(m.repair))
		r.Get("/admin/part-requests", httpx.H(m.requests))
		r.Get("/admin/part-requests/{id}", httpx.H(m.request))
		r.Get("/admin/shipments", httpx.H(m.shipments))
		r.Get("/admin/shipments/{id}", httpx.H(m.shipment))
		r.Get("/admin/technicians", httpx.H(m.technicians))
		r.Get("/admin/technicians/{id}", httpx.H(m.technician))
		r.Get("/admin/technicians/{id}/ledger", httpx.H(m.technicianLedger))
	})
	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PServiceManage))
		r.Post("/admin/repairs", httpx.H(m.officeNewRepair))
		r.Post("/admin/repairs/{id}/assign", httpx.H(m.assign))
		r.Post("/admin/repairs/{id}/review", httpx.H(m.review))
		r.Post("/admin/repairs/{id}/customer-paid", httpx.H(m.customerPaid))
		r.Post("/admin/part-requests/{id}/deliver", httpx.H(m.deliver))
		r.Post("/admin/part-requests/{id}/not-delivered", httpx.H(m.notDelivered))
		r.Post("/admin/part-requests/{id}/cancel", httpx.H(m.cancelRequest))
		r.Post("/admin/shipments/{id}/receive", httpx.H(m.receive))
		r.Post("/admin/shipments/{id}/invoice", httpx.H(m.invoice))
		r.Post("/admin/shipments/{id}/pay", httpx.H(m.pay))
		r.Post("/admin/shipments/{id}/reject", httpx.H(m.rejectShipment))
	})
	r.With(auth.Require(auth.PServiceDelete)).Delete("/admin/shipments/{id}", httpx.H(m.deleteShipment))
	r.With(auth.Require(auth.PServiceDelete)).Delete("/admin/repairs/{id}", httpx.H(m.deleteRepair))
	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PTechniciansMgt))
		r.Post("/admin/technicians", httpx.H(m.createTechnician))
		r.Post("/admin/technicians-import", httpx.H(m.importTechnicians))
		r.Get("/admin/technicians-import/template", httpx.H(m.importTemplate))
		r.Patch("/admin/technicians/{id}", httpx.H(m.patchTechnician))
		r.Post("/admin/technicians/{id}/review", httpx.H(m.reviewTechnician))
		r.Post("/admin/technicians/{id}/ledger", httpx.H(m.ledgerEntry))
	})
}

// ───────── pipeline ─────────

// The stage of a case is the FIRST one that applies, so a repair waiting both
// for a part and for a review reports the review - what has to happen first.
const stageSQL = `CASE
  WHEN r.status='submitted' THEN 'awaiting_review'
  WHEN r.status IN ('rejected','cancelled') THEN 'closed'
  WHEN r.status IN ('requested','assigned','in_progress') AND NOT EXISTS (SELECT 1 FROM part_requests pr WHERE pr.repair_id=r.id AND pr.status IN ('pending','approved') AND pr.deleted_at IS NULL)
       THEN CASE WHEN r.status='requested' THEN 'unassigned' ELSE 'in_progress' END
  WHEN EXISTS (SELECT 1 FROM part_requests pr WHERE pr.repair_id=r.id AND pr.status='pending' AND pr.deleted_at IS NULL) THEN 'parts_to_approve'
  WHEN EXISTS (SELECT 1 FROM part_requests pr WHERE pr.repair_id=r.id AND pr.status='approved' AND pr.deleted_at IS NULL) THEN 'parts_in_transit'
  WHEN EXISTS (SELECT 1 FROM repair_parts rp WHERE rp.repair_id=r.id AND rp.return_state='pending') THEN 'parts_to_return'
  WHEN EXISTS (SELECT 1 FROM repair_parts rp JOIN shipments s ON s.id=rp.shipment_id AND s.deleted_at IS NULL WHERE rp.repair_id=r.id AND s.status='sent') THEN 'parcel_in_transit'
  WHEN EXISTS (SELECT 1 FROM repair_parts rp JOIN shipments s ON s.id=rp.shipment_id AND s.deleted_at IS NULL WHERE rp.repair_id=r.id AND s.status='received') THEN 'awaiting_invoice'
  WHEN EXISTS (SELECT 1 FROM repair_parts rp JOIN shipments s ON s.id=rp.shipment_id AND s.deleted_at IS NULL WHERE rp.repair_id=r.id AND s.status='invoiced') THEN 'awaiting_payment'
  ELSE 'complete' END`

func (m *mod) pipeline(w http.ResponseWriter, r *http.Request) error {
	stage, q := httpx.Q(r, "stage"), httpx.Q(r, "q")
	pg := httpx.Paging(r, 200)
	base := `FROM (SELECT r.*, ` + stageSQL + ` AS stage FROM repairs r WHERE r.deleted_at IS NULL) r
	  JOIN customers c ON c.id=r.customer_id LEFT JOIN products p ON p.id=r.product_id
	  WHERE ($1='' OR r.number ILIKE '%'||$1||'%' OR c.full_name ILIKE '%'||$1||'%' OR c.phone LIKE '%'||$1||'%' OR EXISTS(SELECT 1 FROM guarantees g WHERE g.id=r.guarantee_id AND g.code ILIKE '%'||$1||'%'))`
	counts, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_object_agg(stage,n),'{}') FROM (SELECT r.stage, COUNT(*) AS n `+base+` GROUP BY r.stage) x`, q)
	if err != nil {
		return err
	}
	items, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
	  SELECT r.id,r.number,r.status,r.stage,r.guarantee_was_expired,r.updated_at AS stage_since,
	    EXTRACT(DAY FROM NOW()-r.updated_at)::int AS days_in_stage,
	    (r.stage NOT IN ('complete','closed') AND NOW()-r.updated_at > INTERVAL '7 days') AS needs_attention,
	    c.full_name AS customer_name,c.phone AS customer_phone,c.city AS customer_city,p.name AS product_name,
	    (SELECT code FROM guarantees WHERE id=r.guarantee_id) AS guarantee_code,
	    (SELECT COALESCE(NULLIF(full_name,''),username) FROM users WHERE id=r.technician_id) AS technician_name,r.technician_id
	  `+base+` AND ($2='' OR r.stage=$2) ORDER BY (r.stage IN ('complete','closed')), r.updated_at LIMIT $3 OFFSET $4) t`, q, stage, pg.Limit, pg.Offset)
	if err != nil {
		return err
	}
	raw(w, 200, []byte(`{"counts":`+string(counts)+`,"items":`+string(items)+`}`))
	return nil
}

func (m *mod) repairs(w http.ResponseWriter, r *http.Request) error {
	pg := httpx.Paging(r, 200)
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
	  SELECT r.id,r.number,r.status,r.source,r.complaint,r.guarantee_was_expired,r.customer_charge,r.created_at,
	    c.full_name AS customer_name,c.city AS customer_city,p.name AS product_name,
	    (SELECT COALESCE(NULLIF(full_name,''),username) FROM users WHERE id=r.technician_id) AS technician_name
	  FROM repairs r JOIN customers c ON c.id=r.customer_id LEFT JOIN products p ON p.id=r.product_id
	  WHERE r.deleted_at IS NULL AND ($1='' OR r.status=$1) AND ($2=0 OR r.technician_id=$2)
	    AND ($3='' OR r.number ILIKE '%'||$3||'%' OR c.full_name ILIKE '%'||$3||'%' OR c.phone LIKE '%'||$3||'%')
	  ORDER BY r.id DESC LIMIT $4 OFFSET $5) t`, httpx.Q(r, "status"), qint(r, "technician_id"), httpx.Q(r, "q"), pg.Limit, pg.Offset)
	if err != nil {
		return err
	}
	rawJSON(w, b)
	return nil
}

func qint(r *http.Request, k string) int64 { n, _ := httpx.QInt(r, k); return n }

func (m *mod) repair(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	b, err := repairDetail(r.Context(), m.DB, id)
	if err != nil {
		return err
	}
	rawJSON(w, b)
	return nil
}

// officeNewRepair: the office opens a case for a customer who phoned in, optionally assigning a technician.
func (m *mod) officeNewRepair(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		GuaranteeID  int64  `json:"guarantee_id"`
		TechnicianID *int64 `json:"technician_id"`
		Complaint    string `json:"complaint"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	id, err := m.openRepair(ctx, in.GuaranteeID, in.TechnicianID, in.Complaint, "office", auth.From(ctx).UserID)
	if err != nil {
		return err
	}
	m.Audit.Log(r, "repair.create", "repair", id, "office")
	b, err := repairDetail(ctx, m.DB, id)
	if err != nil {
		return err
	}
	raw(w, http.StatusCreated, b)
	return nil
}

func (m *mod) openRepair(ctx context.Context, guaranteeID int64, tech *int64, complaint, source string, by int64) (int64, error) {
	var id int64
	err := m.DB.Tx(ctx, func(tx pgx.Tx) error {
		type g struct {
			Cust    int64  `db:"customer_id"`
			Prod    int64  `db:"product_id"`
			Expired bool   `db:"expired"`
			Status  string `db:"status"`
		}
		gg, e := db.One[g](ctx, tx, `SELECT customer_id,product_id,status,(CURRENT_DATE > expiry_date OR status='expired') AS expired FROM guarantees WHERE id=$1 AND deleted_at IS NULL`, guaranteeID)
		if e != nil {
			return httpx.BadRequest("گارانتی پیدا نشد")
		}
		if gg.Status != "approved" && gg.Status != "renewed" && gg.Status != "expired" {
			return httpx.Conflict("این گارانتی فعال نیست")
		}
		status := "requested"
		if tech != nil {
			status = "assigned"
		}
		id, e = db.Scalar[int64](ctx, tx, `INSERT INTO repairs(guarantee_id,customer_id,product_id,technician_id,status,source,complaint,guarantee_was_expired,created_by)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`, guaranteeID, gg.Cust, gg.Prod, tech, status, source, strings.TrimSpace(complaint), gg.Expired, by)
		if e != nil {
			return e
		}
		if tech != nil {
			notifyUser(ctx, tx, *tech, "تعمیر جدید به شما محول شد", "یک درخواست سرویس جدید دارید", fmt.Sprintf("/tech/repairs/%d", id))
		} else {
			notifyStaff(ctx, tx, auth.PServiceManage, "", "درخواست سرویس جدید", "یک درخواست سرویس بدون تکنسین ثبت شد", fmt.Sprintf("/admin/repairs/%d", id))
		}
		return nil
	})
	return id, err
}

func (m *mod) customerRequest(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		GuaranteeID int64  `json:"guarantee_id"`
		Complaint   string `json:"complaint"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	me := auth.From(ctx).UserID
	owns, _ := db.Scalar[bool](ctx, m.DB, `SELECT EXISTS(SELECT 1 FROM guarantees g JOIN customers c ON c.id=g.customer_id
		WHERE g.id=$1 AND (c.user_id=$2 OR c.phone=(SELECT phone FROM users WHERE id=$2 AND phone<>'')))`, in.GuaranteeID, me)
	if !owns {
		return httpx.ErrNotFound
	}
	if strings.TrimSpace(in.Complaint) == "" {
		return httpx.BadRequest("مشکل دستگاه را توضیح دهید")
	}
	id, err := m.openRepair(ctx, in.GuaranteeID, nil, in.Complaint, "customer", me)
	if err != nil {
		return err
	}
	httpx.Created(w, map[string]any{"id": id})
	return nil
}

func (m *mod) customerRepairs(w http.ResponseWriter, r *http.Request) error {
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
	  SELECT r.id,r.number,r.status,r.complaint,r.created_at,r.customer_charge,r.customer_paid,p.name AS product_name,
	    (SELECT COALESCE(NULLIF(full_name,''),username) FROM users WHERE id=r.technician_id) AS technician_name
	  FROM repairs r JOIN customers c ON c.id=r.customer_id LEFT JOIN products p ON p.id=r.product_id
	  WHERE r.deleted_at IS NULL AND (c.user_id=$1 OR c.phone=(SELECT phone FROM users WHERE id=$1 AND phone<>'')) ORDER BY r.id DESC LIMIT 50) t`, uid(r))
	if err != nil {
		return err
	}
	rawJSON(w, b)
	return nil
}

func (m *mod) assign(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		TechnicianID int64 `json:"technician_id"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	ok, _ := db.Scalar[bool](ctx, m.DB, `SELECT EXISTS(SELECT 1 FROM technicians t JOIN users u ON u.id=t.user_id WHERE t.user_id=$1 AND t.status='approved' AND u.is_active)`, in.TechnicianID)
	if !ok {
		return httpx.BadRequest("تکنسین فعال پیدا نشد")
	}
	tag, err := m.DB.Exec(ctx, `UPDATE repairs SET technician_id=$2, status=CASE WHEN status='requested' THEN 'assigned' ELSE status END, updated_at=NOW()
		WHERE id=$1 AND status IN ('requested','assigned','in_progress','waiting_parts') AND deleted_at IS NULL`, id, in.TechnicianID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.Conflict("این تعمیر قابل محول کردن نیست")
	}
	notifyUser(ctx, m.DB, in.TechnicianID, "تعمیر جدید به شما محول شد", "یک درخواست سرویس جدید دارید", fmt.Sprintf("/tech/repairs/%d", id))
	m.Audit.Log(r, "repair.assign", "repair", id, fmt.Sprint(in.TechnicianID))
	return m.repair(w, r)
}

// review approves or rejects a filed report. Approval pays the technician's
// labour (in-warranty work) and invites the customer to rate the service.
func (m *mod) review(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Decision string `json:"decision"`
		Notes    string `json:"notes"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.Decision != "approve" && in.Decision != "reject" {
		return httpx.BadRequest("تصمیم نامعتبر است")
	}
	if in.Decision == "reject" && strings.TrimSpace(in.Notes) == "" {
		return httpx.BadRequest("دلیل رد را بنویسید")
	}
	ctx := r.Context()
	me := auth.From(ctx).UserID
	err = m.DB.Tx(ctx, func(tx pgx.Tx) error {
		type rp struct {
			Tech    *int64 `db:"technician_id"`
			Expired bool   `db:"guarantee_was_expired"`
			Number  string `db:"number"`
		}
		v, e := db.One[rp](ctx, tx, `SELECT technician_id,guarantee_was_expired,number FROM repairs WHERE id=$1 AND status='submitted' AND deleted_at IS NULL FOR UPDATE`, id)
		if e != nil {
			return httpx.Conflict("این تعمیر در انتظار بررسی نیست")
		}
		to := "approved"
		if in.Decision == "reject" {
			to = "rejected"
		}
		if _, e = tx.Exec(ctx, `UPDATE repairs SET status=$2, reviewed_by=$3, reviewed_at=NOW(), review_notes=$4, updated_at=NOW(),
			completed_at=CASE WHEN $2='approved' THEN NOW() END WHERE id=$1`, id, to, me, in.Notes); e != nil {
			return e
		}
		if to == "rejected" {
			// nothing to send back for a repair that did not stand
			if _, e = tx.Exec(ctx, `UPDATE repair_parts SET return_state='none' WHERE repair_id=$1 AND return_state='pending'`, id); e != nil {
				return e
			}
			if v.Tech != nil {
				notifyUser(ctx, tx, *v.Tech, "گزارش تعمیر رد شد", v.Number+": "+in.Notes, fmt.Sprintf("/tech/repairs/%d", id))
			}
			return nil
		}
		if v.Tech != nil {
			if !v.Expired {
				labor, _ := db.Scalar[int64](ctx, tx, `SELECT COALESCE(SUM(qty*fee),0)::bigint FROM repair_services WHERE repair_id=$1`, id)
				if labor > 0 {
					if e = creditLedger(ctx, tx, *v.Tech, "labor", labor, "repair", id, "اجرت تعمیر "+v.Number, &me); e != nil {
						return e
					}
				}
			}
			notifyUser(ctx, tx, *v.Tech, "گزارش تعمیر تایید شد", v.Number, fmt.Sprintf("/tech/repairs/%d", id))
		}
		return surveys.TriggerRepair(ctx, tx, id)
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "repair.review", "repair", id, in.Decision)
	return m.repair(w, r)
}

func (m *mod) customerPaid(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	if _, err = m.DB.Exec(r.Context(), `UPDATE repairs SET customer_paid=TRUE WHERE id=$1`, id); err != nil {
		return err
	}
	return m.repair(w, r)
}

func (m *mod) deleteRepair(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	_, err = m.DB.Exec(r.Context(), `UPDATE repairs SET deleted_at=NOW() WHERE id=$1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	// parts that were queued for a parcel are released
	_, _ = m.DB.Exec(r.Context(), `UPDATE repair_parts SET return_state='none' WHERE repair_id=$1 AND return_state='pending'`, id)
	m.Audit.Log(r, "repair.delete", "repair", id, "")
	httpx.NoContent(w)
	return nil
}

// ───────── part requests (office) ─────────

func (m *mod) requests(w http.ResponseWriter, r *http.Request) error {
	pg := httpx.Paging(r, 200)
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
	  SELECT pr.id,pr.number,pr.status,pr.notes,pr.created_at,pr.guarantee_was_expired,(SELECT number FROM repairs WHERE id=pr.repair_id) AS repair_number,
	    (SELECT COALESCE(NULLIF(full_name,''),username) FROM users WHERE id=pr.technician_id) AS technician_name,
	    (SELECT COUNT(*) FROM part_request_items WHERE request_id=pr.id)::int AS item_count
	  FROM part_requests pr WHERE pr.deleted_at IS NULL AND ($1='' OR pr.status=$1) AND ($2=0 OR pr.technician_id=$2)
	  ORDER BY (pr.status='pending') DESC, pr.id DESC LIMIT $3 OFFSET $4) t`, httpx.Q(r, "status"), qint(r, "technician_id"), pg.Limit, pg.Offset)
	if err != nil {
		return err
	}
	rawJSON(w, b)
	return nil
}

func (m *mod) request(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	b, err := requestDetail(r.Context(), m.DB, id)
	if err != nil {
		return err
	}
	rawJSON(w, b)
	return nil
}

// deliver approves (if needed) and delivers: part lines move from the central
// warehouse into the technician's own warehouse in one approved document.
func (m *mod) deliver(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Notes string `json:"notes"`
	}
	_ = httpx.Decode(r, &in)
	ctx := r.Context()
	me := auth.From(ctx).UserID
	err = m.DB.Tx(ctx, func(tx pgx.Tx) error {
		type rq struct {
			Tech   int64  `db:"technician_id"`
			Status string `db:"status"`
			Number string `db:"number"`
			Repair *int64 `db:"repair_id"`
		}
		q, e := db.One[rq](ctx, tx, `SELECT technician_id,status,number,repair_id FROM part_requests WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id)
		if e != nil {
			return e
		}
		if q.Status != "pending" && q.Status != "approved" && q.Status != "not_delivered" {
			return httpx.Conflict("وضعیت درخواست اجازه تحویل نمی‌دهد")
		}
		lines, e := db.All[warehouse.Line](ctx, tx, `SELECT part_id, SUM(qty)::int AS qty FROM part_request_items WHERE request_id=$1 AND kind='part' GROUP BY part_id`, id)
		_ = lines
		type ln struct {
			PartID int64 `db:"part_id"`
			Qty    int   `db:"qty"`
		}
		ls, e := db.All[ln](ctx, tx, `SELECT part_id, SUM(qty)::int AS qty FROM part_request_items WHERE request_id=$1 AND kind='part' GROUP BY part_id`, id)
		if e != nil {
			return e
		}
		var docID *int64
		if len(ls) > 0 {
			central, e := warehouse.CentralID(ctx, tx)
			if e != nil {
				return e
			}
			dest, e := warehouse.TechWarehouse(ctx, tx, q.Tech)
			if e != nil {
				return e
			}
			wl := make([]warehouse.Line, len(ls))
			for i, l := range ls {
				wl[i] = warehouse.Line{PartID: l.PartID, Qty: l.Qty, Note: q.Number}
			}
			sid := id
			d, e := warehouse.CreateDoc(ctx, tx, warehouse.DocIn{Direction: "out", Reason: "tech_delivery", WarehouseID: central, DestID: &dest,
				Approved: true, SourceType: "part_request", SourceID: &sid, CreatedBy: &me, Lines: wl})
			if e != nil {
				return e // includes "insufficient stock" with the part name
			}
			docID = &d
		}
		if _, e = tx.Exec(ctx, `UPDATE part_requests SET status='delivered', delivered_at=NOW(), reviewed_by=$2, reviewed_at=COALESCE(reviewed_at,NOW()),
			review_notes=COALESCE(NULLIF($3,''),review_notes), stock_document_id=$4, updated_at=NOW() WHERE id=$1`, id, me, in.Notes, docID); e != nil {
			return e
		}
		if q.Repair != nil {
			if _, e = tx.Exec(ctx, `UPDATE repairs SET status='in_progress', updated_at=NOW() WHERE id=$1 AND status='waiting_parts'
				AND NOT EXISTS (SELECT 1 FROM part_requests WHERE repair_id=$1 AND status IN ('pending','approved') AND deleted_at IS NULL AND id<>$2)`, *q.Repair, id); e != nil {
				return e
			}
		}
		notifyUser(ctx, tx, q.Tech, "قطعات شما تحویل شد", q.Number+" به انبار شما اضافه شد", "/tech/stock")
		return nil
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "part_request.deliver", "part_request", id, "")
	return m.request(w, r)
}

func (m *mod) notDelivered(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Notes string `json:"notes"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	tag, err := m.DB.Exec(r.Context(), `UPDATE part_requests SET status='not_delivered', review_notes=$2, reviewed_by=$3, reviewed_at=NOW(), updated_at=NOW()
		WHERE id=$1 AND status IN ('pending','approved') AND deleted_at IS NULL`, id, in.Notes, auth.From(r.Context()).UserID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.Conflict("وضعیت درخواست اجازه این کار را نمی‌دهد")
	}
	return m.request(w, r)
}

func (m *mod) cancelRequest(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	tag, err := m.DB.Exec(r.Context(), `UPDATE part_requests SET status='cancelled', updated_at=NOW() WHERE id=$1 AND status IN ('pending','approved','not_delivered')`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.Conflict("این درخواست قابل لغو نیست")
	}
	return m.request(w, r)
}

// ───────── shipments ─────────

func shipmentList(where string) string {
	return `SELECT COALESCE(json_agg(t),'[]') FROM (
	  SELECT s.id,s.number,s.status,s.method,s.tracking_code,s.sent_on,s.invoice_total,s.created_at,s.received_at,s.invoiced_at,s.paid_at,
	    (SELECT COUNT(*) FROM repair_parts WHERE shipment_id=s.id)::int AS part_count,
	    (SELECT COALESCE(NULLIF(full_name,''),username) FROM users WHERE id=s.technician_id) AS technician_name,s.technician_id
	  FROM shipments s WHERE s.deleted_at IS NULL AND ` + where + ` ORDER BY s.id DESC LIMIT 200) t`
}

func shipmentDetail(ctx context.Context, q db.Q, id int64, onlyTech int64) (json.RawMessage, error) {
	b, err := db.Scalar[json.RawMessage](ctx, q, `SELECT row_to_json(t) FROM (
	  SELECT s.id,s.number,s.status,s.method,s.tracking_code,s.sent_on,s.notes,s.receive_notes,s.review_notes,s.invoice_total,s.payment_ref,s.payment_notes,
	    s.created_at,s.received_at,s.invoiced_at,s.paid_at,s.technician_id,
	    (SELECT COALESCE(NULLIF(full_name,''),username) FROM users WHERE id=s.technician_id) AS technician_name,
	    COALESCE((SELECT json_agg(x ORDER BY x.id) FROM (
	      SELECT rp.id,rp.qty,rp.condition_note,rp.buyback_price,rp.return_state,COALESCE(pt.name,rp.custom_name) AS name,pt.code,r.number AS repair_number
	      FROM repair_parts rp JOIN repairs r ON r.id=rp.repair_id LEFT JOIN parts pt ON pt.id=rp.part_id WHERE rp.shipment_id=s.id) x),'[]') AS parts
	  FROM shipments s WHERE s.id=$1 AND s.deleted_at IS NULL AND ($2=0 OR s.technician_id=$2)) t`, id, onlyTech)
	if err != nil || len(b) == 0 {
		return nil, httpx.ErrNotFound
	}
	return b, nil
}

func (m *mod) shipments(w http.ResponseWriter, r *http.Request) error {
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, shipmentList(`($1='' OR s.status=$1) AND ($2=0 OR s.technician_id=$2)`), httpx.Q(r, "status"), qint(r, "technician_id"))
	if err != nil {
		return err
	}
	summary, _ := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT row_to_json(t) FROM (SELECT
	  COALESCE(SUM(invoice_total) FILTER (WHERE status='invoiced'),0)::bigint AS payable,
	  COALESCE(SUM(invoice_total) FILTER (WHERE status='paid'),0)::bigint AS paid,
	  COUNT(*) FILTER (WHERE status='sent')::int AS to_receive FROM shipments WHERE deleted_at IS NULL) t`)
	raw(w, 200, []byte(`{"items":`+string(b)+`,"summary":`+string(summary)+`}`))
	return nil
}

func (m *mod) shipment(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	b, err := shipmentDetail(r.Context(), m.DB, id, 0)
	if err != nil {
		return err
	}
	rawJSON(w, b)
	return nil
}

// receive: tick what really arrived. By default it also invoices at the price
// list (buyback_price captured on each line), which is what removes the old
// "type a price for every part" step; prices can be overridden per line.
func (m *mod) receive(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		ReceivedIDs []int64          `json:"received_repair_part_ids"`
		Notes       string           `json:"notes"`
		AutoInvoice *bool            `json:"auto_invoice"`
		Prices      map[string]int64 `json:"prices"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if len(in.ReceivedIDs) == 0 {
		return httpx.BadRequest("حداقل یک قطعه باید رسیده باشد؛ در غیر این صورت مرسوله را رد کنید")
	}
	auto := in.AutoInvoice == nil || *in.AutoInvoice
	ctx := r.Context()
	me := auth.From(ctx).UserID
	err = m.DB.Tx(ctx, func(tx pgx.Tx) error {
		type sh struct {
			Tech   int64  `db:"technician_id"`
			Status string `db:"status"`
			Number string `db:"number"`
		}
		s, e := db.One[sh](ctx, tx, `SELECT technician_id,status,number FROM shipments WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id)
		if e != nil {
			return e
		}
		if s.Status != "sent" {
			return httpx.Conflict("فقط مرسوله «ارسال‌شده» قابل دریافت است")
		}
		tag, e := tx.Exec(ctx, `UPDATE repair_parts SET return_state='received' WHERE shipment_id=$1 AND id = ANY($2)`, id, in.ReceivedIDs)
		if e != nil {
			return e
		}
		if int(tag.RowsAffected()) != len(in.ReceivedIDs) {
			return httpx.BadRequest("قطعه نامعتبر در فهرست رسیده‌ها")
		}
		// whatever did not arrive goes back to the technician's "to send" list
		if _, e = tx.Exec(ctx, `UPDATE repair_parts SET return_state='pending', shipment_id=NULL WHERE shipment_id=$1 AND return_state='in_shipment'`, id); e != nil {
			return e
		}
		// the old units enter the returns warehouse
		type rl struct {
			Part int64 `db:"part_id"`
			Qty  int   `db:"qty"`
		}
		rls, e := db.All[rl](ctx, tx, `SELECT part_id, SUM(qty)::int AS qty FROM repair_parts WHERE shipment_id=$1 AND return_state='received' AND part_id IS NOT NULL GROUP BY part_id`, id)
		if e != nil {
			return e
		}
		if len(rls) > 0 {
			if whID, e := warehouse.KindID(ctx, tx, "returns"); e == nil {
				wl := make([]warehouse.Line, len(rls))
				for i, l := range rls {
					wl[i] = warehouse.Line{PartID: l.Part, Qty: l.Qty, Note: s.Number}
				}
				sid := id
				if _, e = warehouse.CreateDoc(ctx, tx, warehouse.DocIn{Direction: "in", Reason: "return_old", WarehouseID: whID, Approved: true,
					SourceType: "shipment", SourceID: &sid, CreatedBy: &me, Lines: wl}); e != nil {
					return e
				}
			}
		}
		if _, e = tx.Exec(ctx, `UPDATE shipments SET status='received', received_at=NOW(), received_by=$2, receive_notes=$3, updated_at=NOW() WHERE id=$1`, id, me, in.Notes); e != nil {
			return e
		}
		touchShipmentRepairs(ctx, tx, id)
		if auto {
			return invoiceTx(ctx, tx, id, s.Tech, s.Number, in.Prices, me)
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "shipment.receive", "shipment", id, "")
	return m.shipment(w, r)
}

func touchShipmentRepairs(ctx context.Context, q db.Q, shipmentID int64) {
	_, _ = q.Exec(ctx, `UPDATE repairs SET updated_at=NOW() WHERE id IN (SELECT repair_id FROM repair_parts WHERE shipment_id=$1)`, shipmentID)
}

// invoiceTx prices the received parts and credits the technician's account.
func invoiceTx(ctx context.Context, tx pgx.Tx, id, tech int64, number string, prices map[string]int64, by int64) error {
	for k, p := range prices {
		var rpid int64
		fmt.Sscan(k, &rpid)
		if p < 0 {
			return httpx.BadRequest("قیمت نمی‌تواند منفی باشد")
		}
		if _, e := tx.Exec(ctx, `UPDATE repair_parts SET buyback_price=$3 WHERE id=$1 AND shipment_id=$2 AND return_state='received'`, rpid, id, p); e != nil {
			return e
		}
	}
	total, err := db.Scalar[int64](ctx, tx, `SELECT COALESCE(SUM(qty*buyback_price),0)::bigint FROM repair_parts WHERE shipment_id=$1 AND return_state='received'`, id)
	if err != nil {
		return err
	}
	if total <= 0 {
		return httpx.BadRequest("جمع صورتحساب باید بزرگتر از صفر باشد؛ قیمت قطعات را در فهرست قیمت یا همین‌جا وارد کنید")
	}
	tag, err := tx.Exec(ctx, `UPDATE shipments SET status='invoiced', invoice_total=$2, invoiced_at=NOW(), invoiced_by=$3, updated_at=NOW() WHERE id=$1 AND status='received'`, id, total, by)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.Conflict("وضعیت مرسوله اجازه صدور صورتحساب نمی‌دهد")
	}
	if err = creditLedger(ctx, tx, tech, "part_buyback", total, "shipment", id, "خرید قطعات قدیمی "+number, &by); err != nil {
		return err
	}
	notifyUser(ctx, tx, tech, "صورتحساب مرسوله صادر شد", fmt.Sprintf("%s: %d", number, total), fmt.Sprintf("/tech/shipments/%d", id))
	touchShipmentRepairs(ctx, tx, id)
	return nil
}

func (m *mod) invoice(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Prices map[string]int64 `json:"prices"`
	}
	_ = httpx.Decode(r, &in)
	ctx := r.Context()
	err = m.DB.Tx(ctx, func(tx pgx.Tx) error {
		type sh struct {
			Tech   int64  `db:"technician_id"`
			Number string `db:"number"`
		}
		s, e := db.One[sh](ctx, tx, `SELECT technician_id,number FROM shipments WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id)
		if e != nil {
			return e
		}
		return invoiceTx(ctx, tx, id, s.Tech, s.Number, in.Prices, auth.From(ctx).UserID)
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "shipment.invoice", "shipment", id, "")
	return m.shipment(w, r)
}

func (m *mod) pay(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Reference string `json:"reference"`
		Notes     string `json:"notes"`
	}
	_ = httpx.Decode(r, &in)
	ctx := r.Context()
	me := auth.From(ctx).UserID
	err = m.DB.Tx(ctx, func(tx pgx.Tx) error {
		type sh struct {
			Tech   int64  `db:"technician_id"`
			Total  int64  `db:"invoice_total"`
			Number string `db:"number"`
		}
		s, e := db.One[sh](ctx, tx, `SELECT technician_id,invoice_total,number FROM shipments WHERE id=$1 AND status='invoiced' AND deleted_at IS NULL FOR UPDATE`, id)
		if e != nil {
			return httpx.Conflict("فقط مرسوله دارای صورتحساب قابل پرداخت است")
		}
		if _, e = tx.Exec(ctx, `UPDATE shipments SET status='paid', paid_at=NOW(), paid_by=$2, payment_ref=$3, payment_notes=$4, updated_at=NOW() WHERE id=$1`, id, me, in.Reference, in.Notes); e != nil {
			return e
		}
		if e = creditLedger(ctx, tx, s.Tech, "payment", -s.Total, "shipment", id, "پرداخت "+s.Number+" "+in.Reference, &me); e != nil {
			return e
		}
		notifyUser(ctx, tx, s.Tech, "پرداخت انجام شد", fmt.Sprintf("%s: %d", s.Number, s.Total), fmt.Sprintf("/tech/shipments/%d", id))
		touchShipmentRepairs(ctx, tx, id)
		return nil
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "shipment.pay", "shipment", id, in.Reference)
	return m.shipment(w, r)
}

func (m *mod) rejectShipment(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Notes string `json:"notes"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Notes) == "" {
		return httpx.BadRequest("دلیل رد را بنویسید")
	}
	err = m.DB.Tx(r.Context(), func(tx pgx.Tx) error {
		tag, e := tx.Exec(r.Context(), `UPDATE shipments SET status='rejected', review_notes=$2, updated_at=NOW() WHERE id=$1 AND status='sent' AND deleted_at IS NULL`, id, in.Notes)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return httpx.Conflict("فقط مرسوله «ارسال‌شده» قابل رد است")
		}
		return releaseParts(r.Context(), tx, id)
	})
	if err != nil {
		return err
	}
	return m.shipment(w, r)
}

func (m *mod) deleteShipment(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	err = m.DB.Tx(r.Context(), func(tx pgx.Tx) error {
		tag, e := tx.Exec(r.Context(), `UPDATE shipments SET deleted_at=NOW() WHERE id=$1 AND deleted_at IS NULL AND status IN ('sent','rejected','cancelled')`, id)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return httpx.Conflict("مرسوله‌ای که صورتحساب یا پرداخت دارد حذف نمی‌شود")
		}
		return releaseParts(r.Context(), tx, id)
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "shipment.delete", "shipment", id, "")
	httpx.NoContent(w)
	return nil
}

// ───────── technicians (office) ─────────

func (m *mod) technicians(w http.ResponseWriter, r *http.Request) error {
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
	  SELECT u.id,u.full_name,u.username,u.phone,u.is_active,t.status,t.province,t.city,t.applied_at,
	    COALESCE((SELECT SUM(amount) FROM technician_ledger WHERE technician_id=u.id),0)::bigint AS balance,
	    (SELECT COUNT(*) FROM repairs WHERE technician_id=u.id AND status IN ('assigned','in_progress','waiting_parts') AND deleted_at IS NULL)::int AS open_repairs
	  FROM technicians t JOIN users u ON u.id=t.user_id WHERE u.deleted_at IS NULL AND ($1='' OR t.status=$1)
	    AND ($2='' OR u.full_name ILIKE '%'||$2||'%' OR u.phone LIKE '%'||$2||'%' OR t.city ILIKE '%'||$2||'%')
	  ORDER BY (t.status='pending') DESC, u.full_name) t`, httpx.Q(r, "status"), httpx.Q(r, "q"))
	if err != nil {
		return err
	}
	rawJSON(w, b)
	return nil
}

func (m *mod) technician(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT row_to_json(t) FROM (
	  SELECT u.id,u.full_name,u.username,u.phone,u.is_active,tc.national_id,tc.province,tc.city,tc.address,tc.about,tc.status,tc.review_notes,tc.applied_at,
	    COALESCE((SELECT SUM(amount) FROM technician_ledger WHERE technician_id=u.id),0)::bigint AS balance,
	    COALESCE((SELECT SUM(sl.qty) FROM stock_levels sl JOIN warehouses w ON w.id=sl.warehouse_id WHERE w.technician_user_id=u.id),0)::int AS stock_units
	  FROM technicians tc JOIN users u ON u.id=tc.user_id WHERE tc.user_id=$1) t`, id)
	if err != nil || len(b) == 0 {
		return httpx.ErrNotFound
	}
	rawJSON(w, b)
	return nil
}

func (m *mod) technicianLedger(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
	  SELECT id,kind,amount,note,ref_type,ref_id,created_at FROM technician_ledger WHERE technician_id=$1 ORDER BY id DESC LIMIT 300) t`, id)
	if err != nil {
		return err
	}
	raw(w, 200, []byte(fmt.Sprintf(`{"balance":%d,"entries":%s}`, balance(r.Context(), m.DB, id), b)))
	return nil
}

func (m *mod) createTechnician(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		FullName   string `json:"full_name"`
		Phone      string `json:"phone"`
		Username   string `json:"username"`
		Password   string `json:"password"`
		NationalID string `json:"national_id"`
		Province   string `json:"province"`
		City       string `json:"city"`
		Address    string `json:"address"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	id, err := m.insertTechnician(r.Context(), techIn{FullName: in.FullName, Phone: httpx.Phone(in.Phone), Username: in.Username, Password: in.Password,
		NationalID: in.NationalID, Province: in.Province, City: in.City, Address: in.Address})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "technician.create", "technician", id, in.Username)
	httpx.Created(w, map[string]any{"id": id})
	return nil
}

func (m *mod) patchTechnician(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		FullName   *string `json:"full_name"`
		Phone      *string `json:"phone"`
		IsActive   *bool   `json:"is_active"`
		NationalID *string `json:"national_id"`
		Province   *string `json:"province"`
		City       *string `json:"city"`
		Address    *string `json:"address"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	var phone *string
	if in.Phone != nil {
		v := httpx.Phone(*in.Phone)
		phone = &v
	}
	ctx := r.Context()
	if _, err = m.DB.Exec(ctx, `UPDATE users SET full_name=COALESCE($2,full_name),phone=COALESCE($3,phone),is_active=COALESCE($4,is_active),updated_at=NOW() WHERE id=$1`, id, in.FullName, phone, in.IsActive); err != nil {
		return err
	}
	if _, err = m.DB.Exec(ctx, `UPDATE technicians SET national_id=COALESCE($2,national_id),province=COALESCE($3,province),city=COALESCE($4,city),address=COALESCE($5,address) WHERE user_id=$1`,
		id, in.NationalID, in.Province, in.City, in.Address); err != nil {
		return err
	}
	if in.IsActive != nil && !*in.IsActive {
		m.Auth.RevokeAll(ctx, id)
	}
	m.Audit.Log(r, "technician.update", "technician", id, "")
	return m.technician(w, r)
}

func (m *mod) reviewTechnician(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Decision string `json:"decision"`
		Notes    string `json:"notes"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	to := map[string]string{"approve": "approved", "reject": "rejected"}[in.Decision]
	if to == "" {
		return httpx.BadRequest("تصمیم نامعتبر است")
	}
	tag, err := m.DB.Exec(r.Context(), `UPDATE technicians SET status=$2, review_notes=$3, reviewed_at=NOW(), reviewed_by=$4 WHERE user_id=$1`, id, to, in.Notes, auth.From(r.Context()).UserID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.ErrNotFound
	}
	notifyUser(r.Context(), m.DB, id, "نتیجه بررسی درخواست", map[string]string{"approved": "حساب تکنسینی شما فعال شد", "rejected": "درخواست شما تایید نشد"}[to], "/tech")
	m.Audit.Log(r, "technician.review", "technician", id, in.Decision)
	return m.technician(w, r)
}

// ledgerEntry lets the office record a manual payment or correction. The
// ledger itself can never be edited, so corrections are new signed rows.
func (m *mod) ledgerEntry(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Kind   string `json:"kind"` // payment | adjustment
		Amount int64  `json:"amount"`
		Note   string `json:"note"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if (in.Kind != "payment" && in.Kind != "adjustment") || in.Amount == 0 || strings.TrimSpace(in.Note) == "" {
		return httpx.BadRequest("نوع، مبلغ و توضیح لازم است")
	}
	amount := in.Amount
	if in.Kind == "payment" && amount > 0 {
		amount = -amount // payments always reduce what the company owes
	}
	me := auth.From(r.Context()).UserID
	err = m.DB.Tx(r.Context(), func(tx pgx.Tx) error {
		return creditLedger(r.Context(), tx, id, in.Kind, amount, "manual", 0, in.Note, &me)
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "ledger."+in.Kind, "technician", id, fmt.Sprint(amount))
	return m.technicianLedger(w, r)
}
