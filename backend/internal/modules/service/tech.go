package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/evinki/platform/internal/modules/warehouse"
	"github.com/evinki/platform/internal/modules/warranty"
	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
)

func (m *mod) mountTech(r chi.Router) {
	// Anyone can ask to become a technician; an office user approves.
	r.Post("/public/technicians/apply", httpx.H(m.apply))

	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PTechPortal))
		r.Use(m.approvedTech)
		r.Get("/tech/dashboard", httpx.H(m.techDashboard))
		r.Get("/tech/guarantees/lookup", httpx.H(m.techLookup))

		r.Get("/tech/repairs", httpx.H(m.techRepairs))
		r.Post("/tech/repairs", httpx.H(m.techNewRepair))
		r.Get("/tech/repairs/{id}", httpx.H(m.techRepair))
		r.Patch("/tech/repairs/{id}", httpx.H(m.techPatchRepair))
		r.Post("/tech/repairs/{id}/start", httpx.H(m.techStart))
		r.Post("/tech/repairs/{id}/parts", httpx.H(m.techAddPart))
		r.Delete("/tech/repairs/{id}/parts/{pid}", httpx.H(m.techDelPart))
		r.Post("/tech/repairs/{id}/services", httpx.H(m.techAddService))
		r.Delete("/tech/repairs/{id}/services/{sid}", httpx.H(m.techDelService))
		r.Post("/tech/repairs/{id}/photos", httpx.H(m.techAddPhoto))
		r.Post("/tech/repairs/{id}/submit", httpx.H(m.techSubmit))
		r.Post("/tech/repairs/{id}/cancel", httpx.H(m.techCancel))
		r.Post("/tech/repairs/{id}/reopen", httpx.H(m.techReopen))

		r.Get("/tech/part-requests", httpx.H(m.techRequests))
		r.Post("/tech/part-requests", httpx.H(m.techNewRequest))
		r.Post("/tech/part-requests/{id}/cancel", httpx.H(m.techCancelRequest))

		r.Get("/tech/stock", httpx.H(m.techStock))
		r.Get("/tech/returns", httpx.H(m.techReturns))
		r.Get("/tech/shipments", httpx.H(m.techShipments))
		r.Post("/tech/shipments", httpx.H(m.techNewShipment))
		r.Get("/tech/shipments/{id}", httpx.H(m.techShipment))
		r.Post("/tech/shipments/{id}/cancel", httpx.H(m.techCancelShipment))
		r.Get("/tech/ledger", httpx.H(m.techLedger))
		r.Get("/tech/profile", httpx.H(m.techProfile))
	})
}

func uid(r *http.Request) int64 { return auth.From(r.Context()).UserID }

// approvedTech blocks technician accounts still waiting for approval.
func (m *mod) approvedTech(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st, err := db.Scalar[string](r.Context(), m.DB, `SELECT status FROM technicians WHERE user_id=$1`, uid(r))
		switch {
		case err != nil:
			httpx.Fail(w, r, httpx.E(403, "not_technician", "حساب تکنسین برای شما تعریف نشده است"))
		case st == "pending":
			httpx.Fail(w, r, httpx.E(403, "pending_approval", "درخواست شما در انتظار تایید است"))
		case st != "approved":
			httpx.Fail(w, r, httpx.E(403, "technician_rejected", "درخواست شما تایید نشده است"))
		default:
			next.ServeHTTP(w, r)
		}
	})
}

// ───────── registration ─────────

func (m *mod) apply(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		FullName   string `json:"full_name"`
		Phone      string `json:"phone"`
		Password   string `json:"password"`
		NationalID string `json:"national_id"`
		Province   string `json:"province"`
		City       string `json:"city"`
		Address    string `json:"address"`
		About      string `json:"about"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	in.Phone = httpx.Phone(in.Phone)
	if strings.TrimSpace(in.FullName) == "" || !httpx.ValidMobile(in.Phone) || len(in.Password) < 8 {
		return httpx.BadRequest("نام، موبایل معتبر و گذرواژه حداقل ۸ نویسه لازم است")
	}
	h, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	ctx := r.Context()
	err = m.DB.Tx(ctx, func(tx pgx.Tx) error {
		id, e := db.Scalar[int64](ctx, tx, `INSERT INTO users(username,full_name,phone,password_hash) VALUES($1,$2,$1,$3) RETURNING id`, in.Phone, strings.TrimSpace(in.FullName), h)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO user_roles(user_id,role_code) VALUES($1,'technician')`, id); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO technicians(user_id,national_id,province,city,address,about,status,applied_at) VALUES($1,$2,$3,$4,$5,$6,'pending',NOW())`,
			id, in.NationalID, in.Province, in.City, in.Address, in.About); e != nil {
			return e
		}
		notifyStaff(ctx, tx, auth.PTechniciansMgt, "", "درخواست همکاری تکنسین", in.FullName+" درخواست همکاری داده است", "/admin/technicians")
		return nil
	})
	if db.IsUnique(err) {
		return httpx.Conflict("با این شماره قبلاً ثبت‌نام شده است")
	}
	if err != nil {
		return err
	}
	httpx.Created(w, map[string]any{"status": "pending", "message": "درخواست شما ثبت شد و پس از بررسی فعال می‌شود"})
	return nil
}

// ───────── dashboard & lookup ─────────

func (m *mod) techDashboard(w http.ResponseWriter, r *http.Request) error {
	me := uid(r)
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT row_to_json(t) FROM (SELECT
	  (SELECT COUNT(*) FROM repairs WHERE technician_id=$1 AND status IN ('assigned','in_progress','waiting_parts') AND deleted_at IS NULL)::int AS open_repairs,
	  (SELECT COUNT(*) FROM repairs WHERE technician_id=$1 AND status='assigned' AND deleted_at IS NULL)::int AS to_start,
	  (SELECT COUNT(*) FROM repairs WHERE technician_id=$1 AND status='submitted' AND deleted_at IS NULL)::int AS awaiting_review,
	  (SELECT COUNT(*) FROM repairs WHERE technician_id=$1 AND status='rejected' AND deleted_at IS NULL)::int AS rejected,
	  (SELECT COUNT(*) FROM part_requests WHERE technician_id=$1 AND status IN ('pending','approved') AND deleted_at IS NULL)::int AS open_requests,
	  (SELECT COALESCE(SUM(rp.qty),0) FROM repair_parts rp JOIN repairs r ON r.id=rp.repair_id WHERE r.technician_id=$1 AND rp.return_state='pending' AND r.status IN ('submitted','approved') AND r.deleted_at IS NULL)::int AS parts_to_send,
	  (SELECT COUNT(*) FROM shipments WHERE technician_id=$1 AND status='sent' AND deleted_at IS NULL)::int AS parcels_in_transit,
	  (SELECT COALESCE(SUM(invoice_total),0) FROM shipments WHERE technician_id=$1 AND status='invoiced' AND deleted_at IS NULL)::bigint AS awaiting_payment,
	  (SELECT COALESCE(SUM(invoice_total),0) FROM shipments WHERE technician_id=$1 AND status='paid' AND deleted_at IS NULL)::bigint AS paid_total,
	  (SELECT COALESCE(SUM(sl.qty),0) FROM stock_levels sl JOIN warehouses w ON w.id=sl.warehouse_id WHERE w.technician_user_id=$1)::int AS stock_units,
	  (SELECT COALESCE(SUM(amount),0) FROM technician_ledger WHERE technician_id=$1)::bigint AS balance) t`, me)
	if err != nil {
		return err
	}
	rawJSON(w, b)
	return nil
}

func (m *mod) techLookup(w http.ResponseWriter, r *http.Request) error {
	code := strings.TrimSpace(httpx.Digits(httpx.Q(r, "code")))
	if code == "" {
		return httpx.BadRequest("کد گارانتی را وارد کنید")
	}
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT row_to_json(t) FROM (
	  SELECT g.id,g.code,g.status,g.purchase_date,g.expiry_date,g.golden_expiry_date,(g.expiry_date-CURRENT_DATE)::int AS days_remaining,
	    (CURRENT_DATE > g.expiry_date OR g.status NOT IN ('approved','renewed')) AS expired,
	    c.id AS customer_id,c.full_name AS customer_name,c.phone AS customer_phone,c.city AS customer_city,c.address AS customer_address,
	    p.id AS product_id,p.name AS product_name,
	    (SELECT COUNT(*) FROM repairs WHERE guarantee_id=g.id AND deleted_at IS NULL)::int AS previous_repairs
	  FROM guarantees g JOIN customers c ON c.id=g.customer_id JOIN products p ON p.id=g.product_id
	  WHERE g.code=$1 AND g.deleted_at IS NULL AND g.status IN ('pending','approved','renewed','expired') ORDER BY g.id DESC LIMIT 1) t`, code)
	if err != nil || len(b) == 0 {
		raw(w, 200, []byte(`{"found":false}`))
		return nil
	}
	raw(w, 200, []byte(`{"found":true,"guarantee":`+string(b)+`}`))
	return nil
}

// ───────── repairs ─────────

func (m *mod) techRepairs(w http.ResponseWriter, r *http.Request) error {
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
	  SELECT r.id,r.number,r.status,r.complaint,r.guarantee_was_expired,r.created_at,r.visit_at,c.full_name AS customer_name,c.city AS customer_city,
	    p.name AS product_name,(SELECT code FROM guarantees WHERE id=r.guarantee_id) AS guarantee_code,
	    EXISTS(SELECT 1 FROM repair_parts rp WHERE rp.repair_id=r.id AND rp.return_state='pending') AS has_parts_to_return
	  FROM repairs r JOIN customers c ON c.id=r.customer_id LEFT JOIN products p ON p.id=r.product_id
	  WHERE r.technician_id=$1 AND r.deleted_at IS NULL AND ($2='' OR r.status=$2)
	    AND ($3='' OR r.number ILIKE '%'||$3||'%' OR c.full_name ILIKE '%'||$3||'%' OR c.phone LIKE '%'||$3||'%')
	  ORDER BY CASE r.status WHEN 'assigned' THEN 0 WHEN 'in_progress' THEN 1 WHEN 'waiting_parts' THEN 2 WHEN 'rejected' THEN 3 ELSE 4 END, r.id DESC
	  LIMIT 200) t`, uid(r), httpx.Q(r, "status"), httpx.Q(r, "q"))
	if err != nil {
		return err
	}
	rawJSON(w, b)
	return nil
}

type newRepair struct {
	GuaranteeCode string `json:"guarantee_code"`
	Complaint     string `json:"complaint"`
	VisitAt       string `json:"visit_at"`
	ProductID     int64  `json:"product_id"`
	Customer      struct {
		FullName string `json:"full_name"`
		Phone    string `json:"phone"`
		City     string `json:"city"`
		Address  string `json:"address"`
	} `json:"customer"`
}

func (m *mod) techNewRepair(w http.ResponseWriter, r *http.Request) error {
	var in newRepair
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	me := uid(r)
	var id int64
	err := m.DB.Tx(ctx, func(tx pgx.Tx) error {
		var custID, prodID int64
		var gID *int64
		expired := true
		if c := strings.TrimSpace(httpx.Digits(in.GuaranteeCode)); c != "" {
			type g struct {
				ID      int64 `db:"id"`
				Cust    int64 `db:"customer_id"`
				Prod    int64 `db:"product_id"`
				Expired bool  `db:"expired"`
				Status  string `db:"status"`
			}
			gg, e := db.One[g](ctx, tx, `SELECT id,customer_id,product_id,status,(CURRENT_DATE > expiry_date) AS expired FROM guarantees
				WHERE code=$1 AND deleted_at IS NULL AND status IN ('pending','approved','renewed','expired') ORDER BY id DESC LIMIT 1`, c)
			if e != nil {
				return httpx.BadRequest("گارانتی با این کد پیدا نشد؛ می‌توانید تعمیر را بدون گارانتی ثبت کنید")
			}
			if gg.Status == "pending" {
				return httpx.Conflict("این گارانتی هنوز تایید نشده است")
			}
			gID, custID, prodID = &gg.ID, gg.Cust, gg.Prod
			expired = gg.Expired || gg.Status == "expired"
		} else {
			if strings.TrimSpace(in.Customer.FullName) == "" || in.ProductID == 0 {
				return httpx.BadRequest("کد گارانتی یا مشخصات مشتری و محصول لازم است")
			}
			var e error
			custID, e = warranty.EnsureCustomer(ctx, tx, in.Customer.FullName, in.Customer.Phone, "", "", in.Customer.City, in.Customer.Address)
			if e != nil {
				return e
			}
			prodID = in.ProductID
		}
		var visit *time.Time
		if in.VisitAt != "" {
			if t, e := time.Parse(time.RFC3339, in.VisitAt); e == nil {
				visit = &t
			}
		}
		var e error
		id, e = db.Scalar[int64](ctx, tx, `INSERT INTO repairs(guarantee_id,customer_id,product_id,technician_id,status,source,complaint,guarantee_was_expired,visit_at,started_at,created_by)
			VALUES($1,$2,$3,$4,'in_progress','technician',$5,$6,$7,NOW(),$4) RETURNING id`, gID, custID, prodID, me, strings.TrimSpace(in.Complaint), expired, visit)
		return e
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "repair.create", "repair", id, "")
	b, err := repairDetail(ctx, m.DB, id)
	if err != nil {
		return err
	}
	raw(w, http.StatusCreated, b)
	return nil
}

// mine loads a repair's status and checks it belongs to the caller.
func (m *mod) mine(r *http.Request) (id int64, status string, err error) {
	id, err = httpx.ID(r, "id")
	if err != nil {
		return
	}
	status, err = db.Scalar[string](r.Context(), m.DB, `SELECT status FROM repairs WHERE id=$1 AND technician_id=$2 AND deleted_at IS NULL`, id, uid(r))
	if err != nil {
		err = httpx.ErrNotFound
	}
	return
}

func (m *mod) techRepair(w http.ResponseWriter, r *http.Request) error {
	id, _, err := m.mine(r)
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

func (m *mod) techPatchRepair(w http.ResponseWriter, r *http.Request) error {
	id, st, err := m.mine(r)
	if err != nil {
		return err
	}
	if !editable(st) {
		return httpx.Conflict("این تعمیر دیگر قابل ویرایش نیست")
	}
	var in struct {
		Complaint *string `json:"complaint"`
		Diagnosis *string `json:"diagnosis"`
		Report    *string `json:"report"`
		VisitAt   *string `json:"visit_at"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	var visit *time.Time
	if in.VisitAt != nil && *in.VisitAt != "" {
		if t, e := time.Parse(time.RFC3339, *in.VisitAt); e == nil {
			visit = &t
		}
	}
	if _, err = m.DB.Exec(r.Context(), `UPDATE repairs SET complaint=COALESCE($2,complaint),diagnosis=COALESCE($3,diagnosis),report=COALESCE($4,report),
		visit_at=COALESCE($5,visit_at),updated_at=NOW() WHERE id=$1`, id, in.Complaint, in.Diagnosis, in.Report, visit); err != nil {
		return err
	}
	return m.techRepair(w, r)
}

func (m *mod) techStart(w http.ResponseWriter, r *http.Request) error {
	id, st, err := m.mine(r)
	if err != nil {
		return err
	}
	if st != "assigned" {
		return httpx.Conflict("فقط تعمیر محول‌شده قابل شروع است")
	}
	if _, err = m.DB.Exec(r.Context(), `UPDATE repairs SET status='in_progress', started_at=NOW(), updated_at=NOW() WHERE id=$1`, id); err != nil {
		return err
	}
	return m.techRepair(w, r)
}

func (m *mod) techReopen(w http.ResponseWriter, r *http.Request) error {
	id, st, err := m.mine(r)
	if err != nil {
		return err
	}
	if st != "rejected" {
		return httpx.Conflict("فقط تعمیر ردشده قابل بازگشایی است")
	}
	if _, err = m.DB.Exec(r.Context(), `UPDATE repairs SET status='in_progress', updated_at=NOW() WHERE id=$1`, id); err != nil {
		return err
	}
	return m.techRepair(w, r)
}

// available = what is in my warehouse minus what my other unsubmitted repairs already claim.
func (m *mod) available(r *http.Request, part int64, excludeRepair int64) (int, error) {
	ctx := r.Context()
	wh, err := warehouse.TechWarehouse(ctx, m.DB, uid(r))
	if err != nil {
		return 0, err
	}
	have := warehouse.Stock(ctx, m.DB, wh, part)
	claimed, _ := db.Scalar[int](ctx, m.DB, `SELECT COALESCE(SUM(rp.qty),0)::int FROM repair_parts rp JOIN repairs r ON r.id=rp.repair_id
		WHERE r.technician_id=$1 AND r.status IN ('assigned','in_progress','waiting_parts') AND rp.part_id=$2 AND rp.source='stock' AND r.id<>$3`, uid(r), part, excludeRepair)
	return have - claimed, nil
}

func (m *mod) techAddPart(w http.ResponseWriter, r *http.Request) error {
	id, st, err := m.mine(r)
	if err != nil {
		return err
	}
	if !editable(st) {
		return httpx.Conflict("این تعمیر دیگر قابل ویرایش نیست")
	}
	var in struct {
		PartID     *int64 `json:"part_id"`
		CustomName string `json:"custom_name"`
		Qty        int    `json:"qty"`
		Report     string `json:"report"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.Qty <= 0 {
		in.Qty = 1
	}
	ctx := r.Context()
	if in.PartID != nil {
		type p struct {
			Name   string `db:"name"`
			Return bool   `db:"return_required"`
			Price  int64  `db:"buyback_price"`
		}
		pt, err := db.One[p](ctx, m.DB, `SELECT name,return_required,buyback_price FROM parts WHERE id=$1 AND deleted_at IS NULL AND is_active`, *in.PartID)
		if err != nil {
			return httpx.BadRequest("قطعه پیدا نشد")
		}
		// Used on an existing line? merge quantities instead of duplicating.
		av, err := m.available(r, *in.PartID, id)
		if err != nil {
			return err
		}
		already, _ := db.Scalar[int](ctx, m.DB, `SELECT COALESCE(SUM(qty),0)::int FROM repair_parts WHERE repair_id=$1 AND part_id=$2 AND source='stock'`, id, *in.PartID)
		if av < already+in.Qty {
			b, _ := json.Marshal(map[string]any{"error": map[string]any{"code": "not_in_stock",
				"message": fmt.Sprintf("«%s» در انبار شما کافی نیست (موجود: %d). می‌توانید آن را درخواست کنید.", pt.Name, max(av-already, 0))},
				"part_id": *in.PartID, "available": max(av-already, 0)})
			raw(w, http.StatusConflict, b)
			return nil
		}
		state := "none"
		if pt.Return {
			state = "pending"
		}
		_, err = m.DB.Exec(ctx, `INSERT INTO repair_parts(repair_id,part_id,qty,report,source,return_required,return_state,buyback_price) VALUES($1,$2,$3,$4,'stock',$5,$6,$7)`,
			id, *in.PartID, in.Qty, in.Report, pt.Return, state, pt.Price)
		if err != nil {
			return err
		}
	} else {
		if strings.TrimSpace(in.CustomName) == "" {
			return httpx.BadRequest("قطعه یا نام قطعه را وارد کنید")
		}
		if _, err = m.DB.Exec(ctx, `INSERT INTO repair_parts(repair_id,custom_name,qty,report,source,return_required,return_state) VALUES($1,$2,$3,$4,'custom',FALSE,'none')`,
			id, strings.TrimSpace(in.CustomName), in.Qty, in.Report); err != nil {
			return err
		}
	}
	touchRepair(ctx, m.DB, id)
	return m.techRepair(w, r)
}

func (m *mod) techDelPart(w http.ResponseWriter, r *http.Request) error {
	id, st, err := m.mine(r)
	if err != nil {
		return err
	}
	if !editable(st) {
		return httpx.Conflict("این تعمیر دیگر قابل ویرایش نیست")
	}
	pid, err := httpx.ID(r, "pid")
	if err != nil {
		return err
	}
	if _, err = m.DB.Exec(r.Context(), `DELETE FROM repair_parts WHERE id=$1 AND repair_id=$2 AND shipment_id IS NULL`, pid, id); err != nil {
		return err
	}
	return m.techRepair(w, r)
}

func (m *mod) techAddService(w http.ResponseWriter, r *http.Request) error {
	id, st, err := m.mine(r)
	if err != nil {
		return err
	}
	if !editable(st) {
		return httpx.Conflict("این تعمیر دیگر قابل ویرایش نیست")
	}
	var in struct {
		ServiceID int64  `json:"service_id"`
		Qty       int    `json:"qty"`
		Report    string `json:"report"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.Qty <= 0 {
		in.Qty = 1
	}
	// the fee is copied now, so a later price change never alters a filed report
	if _, err = m.DB.Exec(r.Context(), `INSERT INTO repair_services(repair_id,service_id,qty,fee,report)
		SELECT $1,id,$3,fee,$4 FROM service_items WHERE id=$2 AND deleted_at IS NULL AND is_active`, id, in.ServiceID, in.Qty, in.Report); err != nil {
		return err
	}
	return m.techRepair(w, r)
}

func (m *mod) techDelService(w http.ResponseWriter, r *http.Request) error {
	id, st, err := m.mine(r)
	if err != nil {
		return err
	}
	if !editable(st) {
		return httpx.Conflict("این تعمیر دیگر قابل ویرایش نیست")
	}
	sid, err := httpx.ID(r, "sid")
	if err != nil {
		return err
	}
	if _, err = m.DB.Exec(r.Context(), `DELETE FROM repair_services WHERE id=$1 AND repair_id=$2`, sid, id); err != nil {
		return err
	}
	return m.techRepair(w, r)
}

func (m *mod) techAddPhoto(w http.ResponseWriter, r *http.Request) error {
	id, st, err := m.mine(r)
	if err != nil {
		return err
	}
	if !editable(st) && st != "submitted" {
		return httpx.Conflict("افزودن عکس ممکن نیست")
	}
	var in struct {
		File    string `json:"file"` // URL returned by POST /files
		Caption string `json:"caption"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if !strings.HasPrefix(in.File, "/api/v1/files/") {
		return httpx.BadRequest("فایل نامعتبر است")
	}
	if _, err = m.DB.Exec(r.Context(), `INSERT INTO repair_photos(repair_id,file,caption) VALUES($1,$2,$3)`, id, in.File, in.Caption); err != nil {
		return err
	}
	return m.techRepair(w, r)
}

func (m *mod) techCancel(w http.ResponseWriter, r *http.Request) error {
	id, st, err := m.mine(r)
	if err != nil {
		return err
	}
	if !editable(st) {
		return httpx.Conflict("فقط تعمیر در حال انجام قابل لغو است")
	}
	if _, err = m.DB.Exec(r.Context(), `UPDATE repairs SET status='cancelled', updated_at=NOW() WHERE id=$1`, id); err != nil {
		return err
	}
	return m.techRepair(w, r)
}

// techSubmit files the report: it consumes the new parts from the technician's
// own warehouse, prices out-of-warranty work, and alerts the office.
func (m *mod) techSubmit(w http.ResponseWriter, r *http.Request) error {
	id, st, err := m.mine(r)
	if err != nil {
		return err
	}
	if !editable(st) {
		return httpx.Conflict("این تعمیر قبلاً ارسال شده است")
	}
	ctx := r.Context()
	me := uid(r)
	err = m.DB.Tx(ctx, func(tx pgx.Tx) error {
		type rep struct {
			Report  string `db:"report"`
			Expired bool   `db:"guarantee_was_expired"`
			Number  string `db:"number"`
		}
		rp, e := db.One[rep](ctx, tx, `SELECT report,guarantee_was_expired,number FROM repairs WHERE id=$1 FOR UPDATE`, id)
		if e != nil {
			return e
		}
		nParts, _ := db.Scalar[int](ctx, tx, `SELECT COUNT(*) FROM repair_parts WHERE repair_id=$1`, id)
		nSvc, _ := db.Scalar[int](ctx, tx, `SELECT COUNT(*) FROM repair_services WHERE repair_id=$1`, id)
		if strings.TrimSpace(rp.Report) == "" && nParts+nSvc == 0 {
			return httpx.BadRequest("گزارش تعمیر یا حداقل یک قطعه/خدمت را ثبت کنید")
		}
		// consume the new parts from my own warehouse
		type use struct {
			Part int64 `db:"part_id"`
			Qty  int   `db:"qty"`
		}
		uses, e := db.All[use](ctx, tx, `SELECT part_id,SUM(qty)::int AS qty FROM repair_parts WHERE repair_id=$1 AND source='stock' AND part_id IS NOT NULL GROUP BY part_id`, id)
		if e != nil {
			return e
		}
		if len(uses) > 0 {
			wh, e := warehouse.TechWarehouse(ctx, tx, me)
			if e != nil {
				return e
			}
			lines := make([]warehouse.Line, len(uses))
			for i, u := range uses {
				lines[i] = warehouse.Line{PartID: u.Part, Qty: u.Qty, Note: rp.Number}
			}
			sid := id
			docID, e := warehouse.CreateDoc(ctx, tx, warehouse.DocIn{Direction: "out", Reason: "repair_use", WarehouseID: wh, Approved: true,
				SourceType: "repair", SourceID: &sid, CreatedBy: &me, Lines: lines})
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `UPDATE repair_parts SET stock_document_id=$2 WHERE repair_id=$1 AND source='stock'`, id, docID); e != nil {
				return e
			}
		}
		// out-of-warranty work is billed to the customer: parts at sale price + services
		var charge int64
		if rp.Expired {
			charge, _ = db.Scalar[int64](ctx, tx, `SELECT COALESCE((SELECT SUM(rp.qty*pt.sale_price) FROM repair_parts rp JOIN parts pt ON pt.id=rp.part_id WHERE rp.repair_id=$1),0)
				+ COALESCE((SELECT SUM(qty*fee) FROM repair_services WHERE repair_id=$1),0)`, id)
		}
		if _, e = tx.Exec(ctx, `UPDATE repairs SET status='submitted', submitted_at=NOW(), updated_at=NOW(), customer_charge=$2 WHERE id=$1`, id, charge); e != nil {
			return e
		}
		notifyStaff(ctx, tx, auth.PServiceManage, "repair_report", "گزارش تعمیر جدید", "تعمیر "+rp.Number+" برای بررسی ارسال شد", fmt.Sprintf("/admin/repairs/%d", id), rp.Number)
		return nil
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "repair.submit", "repair", id, "")
	return m.techRepair(w, r)
}

// ───────── part requests ─────────

type reqItem struct {
	Kind       string `json:"kind"`
	PartID     *int64 `json:"part_id"`
	ServiceID  *int64 `json:"service_id"`
	CustomName string `json:"custom_name"`
	Qty        int    `json:"qty"`
}

func (m *mod) techNewRequest(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		RepairID *int64    `json:"repair_id"`
		Notes    string    `json:"notes"`
		Items    []reqItem `json:"items"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if len(in.Items) == 0 {
		return httpx.BadRequest("حداقل یک قلم لازم است")
	}
	ctx := r.Context()
	var id int64
	err := m.DB.Tx(ctx, func(tx pgx.Tx) error {
		expired := false
		if in.RepairID != nil {
			v, e := db.Scalar[bool](ctx, tx, `SELECT guarantee_was_expired FROM repairs WHERE id=$1 AND technician_id=$2 AND deleted_at IS NULL`, *in.RepairID, uid(r))
			if e != nil {
				return httpx.BadRequest("تعمیر پیدا نشد")
			}
			expired = v
		}
		var e error
		id, e = db.Scalar[int64](ctx, tx, `INSERT INTO part_requests(technician_id,repair_id,notes,guarantee_was_expired) VALUES($1,$2,$3,$4) RETURNING id`, uid(r), in.RepairID, in.Notes, expired)
		if e != nil {
			return e
		}
		for _, it := range in.Items {
			if it.Qty <= 0 {
				it.Qty = 1
			}
			if it.Kind != "part" && it.Kind != "service" && it.Kind != "custom" {
				return httpx.BadRequest("نوع قلم نامعتبر است")
			}
			if _, e = tx.Exec(ctx, `INSERT INTO part_request_items(request_id,kind,part_id,service_id,custom_name,qty) VALUES($1,$2,$3,$4,$5,$6)`,
				id, it.Kind, it.PartID, it.ServiceID, strings.TrimSpace(it.CustomName), it.Qty); e != nil {
				return e
			}
		}
		if in.RepairID != nil {
			if _, e = tx.Exec(ctx, `UPDATE repairs SET status='waiting_parts', updated_at=NOW() WHERE id=$1 AND status IN ('assigned','in_progress')`, *in.RepairID); e != nil {
				return e
			}
		}
		num, _ := db.Scalar[string](ctx, tx, `SELECT number FROM part_requests WHERE id=$1`, id)
		notifyStaff(ctx, tx, auth.PServiceManage, "part_request", "درخواست قطعه جدید", "درخواست "+num+" در انتظار بررسی است", fmt.Sprintf("/admin/part-requests/%d", id), num)
		return nil
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "part_request.create", "part_request", id, "")
	b, err := requestDetail(ctx, m.DB, id)
	if err != nil {
		return err
	}
	raw(w, http.StatusCreated, b)
	return nil
}

func requestDetail(ctx interface {
	Deadline() (time.Time, bool)
	Done() <-chan struct{}
	Err() error
	Value(any) any
}, q db.Q, id int64) (json.RawMessage, error) {
	b, err := db.Scalar[json.RawMessage](ctx, q, `SELECT row_to_json(t) FROM (
	  SELECT pr.id,pr.number,pr.status,pr.notes,pr.review_notes,pr.created_at,pr.delivered_at,pr.guarantee_was_expired,pr.repair_id,
	    (SELECT number FROM repairs WHERE id=pr.repair_id) AS repair_number,pr.technician_id,
	    (SELECT COALESCE(NULLIF(full_name,''),username) FROM users WHERE id=pr.technician_id) AS technician_name,
	    COALESCE((SELECT json_agg(i ORDER BY i.id) FROM (SELECT pri.id,pri.kind,pri.part_id,pri.qty,COALESCE(pt.name,si.name,pri.custom_name) AS name,
	      COALESCE((SELECT qty FROM stock_levels sl JOIN warehouses w ON w.id=sl.warehouse_id AND w.kind='central' WHERE sl.part_id=pri.part_id LIMIT 1),0) AS central_stock
	      FROM part_request_items pri LEFT JOIN parts pt ON pt.id=pri.part_id LEFT JOIN service_items si ON si.id=pri.service_id WHERE pri.request_id=pr.id) i),'[]') AS items
	  FROM part_requests pr WHERE pr.id=$1 AND pr.deleted_at IS NULL) t`, id)
	if err != nil || len(b) == 0 {
		return nil, httpx.ErrNotFound
	}
	return b, nil
}

func (m *mod) techRequests(w http.ResponseWriter, r *http.Request) error {
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
	  SELECT pr.id,pr.number,pr.status,pr.notes,pr.review_notes,pr.created_at,pr.delivered_at,(SELECT number FROM repairs WHERE id=pr.repair_id) AS repair_number,
	    COALESCE((SELECT json_agg(i ORDER BY i.id) FROM (SELECT pri.kind,pri.qty,COALESCE(pt.name,si.name,pri.custom_name) AS name FROM part_request_items pri
	      LEFT JOIN parts pt ON pt.id=pri.part_id LEFT JOIN service_items si ON si.id=pri.service_id WHERE pri.request_id=pr.id) i),'[]') AS items
	  FROM part_requests pr WHERE pr.technician_id=$1 AND pr.deleted_at IS NULL AND ($2='' OR pr.status=$2) ORDER BY pr.id DESC LIMIT 100) t`, uid(r), httpx.Q(r, "status"))
	if err != nil {
		return err
	}
	rawJSON(w, b)
	return nil
}

func (m *mod) techCancelRequest(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	tag, err := m.DB.Exec(r.Context(), `UPDATE part_requests SET status='cancelled', updated_at=NOW() WHERE id=$1 AND technician_id=$2 AND status IN ('pending','approved')`, id, uid(r))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.Conflict("این درخواست قابل لغو نیست")
	}
	httpx.NoContent(w)
	return nil
}

// ───────── own stock, returns, shipments, ledger ─────────

func (m *mod) techStock(w http.ResponseWriter, r *http.Request) error {
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
	  SELECT p.id AS part_id,p.code,p.name,p.variant,p.image,sl.qty FROM stock_levels sl JOIN warehouses w ON w.id=sl.warehouse_id AND w.technician_user_id=$1
	  JOIN parts p ON p.id=sl.part_id WHERE sl.qty>0 ORDER BY p.name) t`, uid(r))
	if err != nil {
		return err
	}
	rawJSON(w, b)
	return nil
}

// techReturns lists replaced parts whose old unit still has to be sent back.
func (m *mod) techReturns(w http.ResponseWriter, r *http.Request) error {
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
	  SELECT rp.id,rp.qty,rp.return_state,COALESCE(pt.name,rp.custom_name) AS name,rp.buyback_price,rp.repair_id,r.number AS repair_number,
	    r.status AS repair_status,(SELECT full_name FROM customers WHERE id=r.customer_id) AS customer_name,rp.condition_note
	  FROM repair_parts rp JOIN repairs r ON r.id=rp.repair_id LEFT JOIN parts pt ON pt.id=rp.part_id
	  WHERE r.technician_id=$1 AND r.deleted_at IS NULL AND rp.return_state='pending' AND r.status IN ('submitted','approved')
	  ORDER BY r.id DESC, rp.id) t`, uid(r))
	if err != nil {
		return err
	}
	rawJSON(w, b)
	return nil
}

func (m *mod) techShipments(w http.ResponseWriter, r *http.Request) error {
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, shipmentList(`s.technician_id=$1 AND ($2='' OR s.status=$2)`), uid(r), httpx.Q(r, "status"))
	if err != nil {
		return err
	}
	rawJSON(w, b)
	return nil
}

func (m *mod) techShipment(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	b, err := shipmentDetail(r.Context(), m.DB, id, uid(r))
	if err != nil {
		return err
	}
	rawJSON(w, b)
	return nil
}

func (m *mod) techNewShipment(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		RepairPartIDs []int64 `json:"repair_part_ids"`
		Method        string  `json:"method"`
		TrackingCode  string  `json:"tracking_code"`
		SentOn        string  `json:"sent_on"`
		Notes         string  `json:"notes"`
		Conditions    map[string]string `json:"conditions"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if len(in.RepairPartIDs) == 0 {
		return httpx.BadRequest("قطعه‌ای انتخاب نشده است")
	}
	switch in.Method {
	case "post", "courier":
		if strings.TrimSpace(in.TrackingCode) == "" {
			return httpx.BadRequest("برای پست و پیک کد رهگیری لازم است")
		}
	case "in_person", "other":
	default:
		return httpx.BadRequest("روش ارسال نامعتبر است")
	}
	sent := time.Now().Format("2006-01-02")
	if s := httpx.Digits(strings.TrimSpace(in.SentOn)); s != "" {
		sent = s
	}
	ctx := r.Context()
	var id int64
	err := m.DB.Tx(ctx, func(tx pgx.Tx) error {
		// the ownership + state check and the claim happen in one statement, so a part cannot land in two parcels
		tag, e := tx.Exec(ctx, `UPDATE repair_parts rp SET return_state='in_shipment' FROM repairs r
			WHERE r.id=rp.repair_id AND r.technician_id=$2 AND rp.id = ANY($1) AND rp.return_state='pending' AND r.status IN ('submitted','approved')`, in.RepairPartIDs, uid(r))
		if e != nil {
			return e
		}
		if int(tag.RowsAffected()) != len(in.RepairPartIDs) {
			return httpx.Conflict("برخی قطعات انتخاب‌شده قابل ارسال نیستند (قبلاً ارسال شده یا متعلق به شما نیست)")
		}
		id, e = db.Scalar[int64](ctx, tx, `INSERT INTO shipments(technician_id,method,tracking_code,sent_on,notes) VALUES($1,$2,$3,$4::date,$5) RETURNING id`,
			uid(r), in.Method, strings.TrimSpace(in.TrackingCode), sent, in.Notes)
		if e != nil {
			return e
		}
		for _, rpid := range in.RepairPartIDs {
			if _, e = tx.Exec(ctx, `UPDATE repair_parts SET shipment_id=$2, condition_note=COALESCE(NULLIF($3,''),condition_note) WHERE id=$1`, rpid, id, in.Conditions[fmt.Sprint(rpid)]); e != nil {
				return e
			}
		}
		_, e = tx.Exec(ctx, `UPDATE repairs SET updated_at=NOW() WHERE id IN (SELECT repair_id FROM repair_parts WHERE shipment_id=$1)`, id)
		if e != nil {
			return e
		}
		notifyStaff(ctx, tx, auth.PServiceManage, "", "مرسوله قطعه جدید", "یک مرسوله قطعه قدیمی ارسال شد", fmt.Sprintf("/admin/shipments/%d", id))
		return nil
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "shipment.create", "shipment", id, "")
	b, err := shipmentDetail(ctx, m.DB, id, uid(r))
	if err != nil {
		return err
	}
	raw(w, http.StatusCreated, b)
	return nil
}

func (m *mod) techCancelShipment(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	err = m.DB.Tx(r.Context(), func(tx pgx.Tx) error {
		tag, e := tx.Exec(r.Context(), `UPDATE shipments SET status='cancelled', updated_at=NOW() WHERE id=$1 AND technician_id=$2 AND status='sent'`, id, uid(r))
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return httpx.Conflict("فقط مرسوله «ارسال‌شده» قابل لغو است")
		}
		return releaseParts(r.Context(), tx, id)
	})
	if err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

// releaseParts frees every part of a shipment so it can be sent again.
func releaseParts(ctx interface {
	Deadline() (time.Time, bool)
	Done() <-chan struct{}
	Err() error
	Value(any) any
}, tx pgx.Tx, shipmentID int64) error {
	_, err := tx.Exec(ctx, `UPDATE repair_parts SET return_state='pending', shipment_id=NULL WHERE shipment_id=$1 AND return_state IN ('in_shipment','not_received')`, shipmentID)
	return err
}

func (m *mod) techLedger(w http.ResponseWriter, r *http.Request) error {
	me := uid(r)
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
	  SELECT id,kind,amount,note,ref_type,ref_id,created_at FROM technician_ledger WHERE technician_id=$1 ORDER BY id DESC LIMIT 200) t`, me)
	if err != nil {
		return err
	}
	raw(w, 200, []byte(fmt.Sprintf(`{"balance":%d,"entries":%s}`, balance(r.Context(), m.DB, me), b)))
	return nil
}

func (m *mod) techProfile(w http.ResponseWriter, r *http.Request) error {
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT row_to_json(t) FROM (
	  SELECT u.id,u.full_name,u.phone,t.national_id,t.province,t.city,t.address,t.about,t.status FROM technicians t JOIN users u ON u.id=t.user_id WHERE t.user_id=$1) t`, uid(r))
	if err != nil {
		return err
	}
	rawJSON(w, b)
	return nil
}
