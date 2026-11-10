package warehouse

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/evinki/platform/internal/app"
	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
	"github.com/evinki/platform/internal/platform/jalali"
	"github.com/evinki/platform/internal/platform/settings"
)

type mod struct{ *app.Deps }

func Mount(r chi.Router, d *app.Deps) {
	m := &mod{d}
	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PWhRead))
		r.Get("/wh/meta", httpx.H(m.meta))
		r.Get("/wh/items", httpx.H(m.items))
		r.Get("/wh/items/scan/{token}", httpx.H(m.scan))
		r.Get("/wh/documents", httpx.H(m.listDocs))
		r.Get("/wh/documents/{id}", httpx.H(m.getDoc))
		r.Get("/wh/counts", httpx.H(m.listCounts))
		r.Get("/wh/counts/{id}", httpx.H(m.getCount))
		r.Get("/wh/workers", httpx.H(m.workers))
	})
	r.With(auth.Require(auth.PWhRequest, auth.PWhWrite)).Post("/wh/documents", httpx.H(m.createDoc))
	r.With(auth.Require(auth.PWhRequest, auth.PWhWrite)).Post("/wh/documents/{id}/cancel", httpx.H(m.cancelDoc))
	r.With(auth.Require(auth.PWhApprove)).Post("/wh/documents/{id}/approve", httpx.H(m.approveDoc))
	r.With(auth.Require(auth.PWhApprove)).Post("/wh/documents/{id}/reject", httpx.H(m.rejectDoc))
	r.With(auth.Require(auth.PWhDelete)).Delete("/wh/documents/{id}", httpx.H(m.deleteDoc))

	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PWhRequest, auth.PWhWrite, auth.PWhManage))
		r.Post("/wh/counts", httpx.H(m.newCount))
		r.Put("/wh/counts/{id}/lines", httpx.H(m.countLine))
		r.Post("/wh/counts/{id}/submit", httpx.H(m.submitCount))
	})
	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PWhManage))
		r.Post("/wh/counts/{id}/apply", httpx.H(m.applyCount))
		r.Post("/wh/counts/{id}/reject", httpx.H(m.rejectCount))
		r.Get("/wh/exports/movements.csv", httpx.H(m.exportMovements))
		r.Get("/wh/exports/sepidar.txt", httpx.H(m.exportSepidar))
		r.Post("/wh/warehouses", httpx.H(m.newWarehouse))
	})
}

func (m *mod) meta(w http.ResponseWriter, r *http.Request) error {
	whs, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t ORDER BY t.id),'[]') FROM
		(SELECT id,code,title,kind FROM warehouses WHERE is_active AND kind<>'technician') t`)
	if err != nil {
		return err
	}
	reasons := []Reason{}
	for _, x := range Reasons {
		if !x.Internal {
			reasons = append(reasons, x)
		}
	}
	rs, _ := json.Marshal(reasons)
	s, _ := settings.All(r.Context(), m.DB)
	set, _ := json.Marshal(map[string]any{"check_stock": s["wh_check_stock"], "approval_out": s["wh_approval_out"],
		"approval_in": s["wh_approval_in"], "count_blind": s["wh_count_blind"]})
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"warehouses":%s,"reasons":%s,"settings":%s}`, whs, rs, set)
	return nil
}

func (m *mod) whParam(r *http.Request) (int64, error) {
	if id, ok := httpx.QInt(r, "warehouse_id"); ok {
		return id, nil
	}
	return CentralID(r.Context(), m.DB)
}

func (m *mod) items(w http.ResponseWriter, r *http.Request) error {
	wh, err := m.whParam(r)
	if err != nil {
		return err
	}
	pg := httpx.Paging(r, 500)
	low := httpx.Q(r, "low") == "1"
	where := ` WHERE p.deleted_at IS NULL AND p.is_active AND ($2='' OR p.name ILIKE '%'||$2||'%' OR p.code ILIKE '%'||$2||'%' OR p.part_no ILIKE '%'||$2||'%' OR p.sepidar_code=$2)
		AND (NOT $3 OR COALESCE(sl.qty,0) <= p.min_stock)`
	from := ` FROM parts p LEFT JOIN stock_levels sl ON sl.part_id=p.id AND sl.warehouse_id=$1`
	total, err := db.Scalar[int](r.Context(), m.DB, `SELECT COUNT(*)`+from+where, wh, httpx.Q(r, "q"), low)
	if err != nil {
		return err
	}
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
		SELECT p.id,p.code,p.sepidar_code,p.part_no,p.name,p.variant,p.material,p.unit,p.location,p.min_stock,p.image,p.qr_token,
		  COALESCE(sl.qty,0)::int AS stock, (COALESCE(sl.qty,0) <= p.min_stock) AS low`+from+where+
		` ORDER BY p.name, p.id LIMIT $4 OFFSET $5) t`, wh, httpx.Q(r, "q"), low, pg.Limit, pg.Offset)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"items":%s,"total":%d,"page":%d,"limit":%d,"warehouse_id":%d}`, raw, total, pg.Page, pg.Limit, wh)
	return nil
}

// scan resolves what a QR/barcode scanner read: "WH:<token>", a bare token or a part code.
func (m *mod) scan(w http.ResponseWriter, r *http.Request) error {
	tok := strings.ToUpper(strings.TrimPrefix(strings.ToUpper(httpx.Digits(chi.URLParam(r, "token"))), "WH:"))
	wh, err := m.whParam(r)
	if err != nil {
		return err
	}
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT row_to_json(t) FROM (
		SELECT p.id,p.code,p.name,p.variant,p.unit,p.image,p.min_stock,p.qr_token,COALESCE(sl.qty,0)::int AS stock
		FROM parts p LEFT JOIN stock_levels sl ON sl.part_id=p.id AND sl.warehouse_id=$2
		WHERE p.deleted_at IS NULL AND (p.qr_token=$1 OR upper(p.code)=$1 OR p.sepidar_code=$1) LIMIT 1) t`, tok, wh)
	if err != nil || len(raw) == 0 {
		return httpx.E(404, "not_found", "قطعه‌ای با این کد پیدا نشد")
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
	return nil
}

// ───────── documents ─────────

type docIn struct {
	Direction   string `json:"direction"`
	Reason      string `json:"reason"`
	WarehouseID int64  `json:"warehouse_id"`
	DestID      *int64 `json:"dest_warehouse_id"`
	WorkerName  string `json:"worker_name"`
	Note        string `json:"note"`
	ClientRef   string `json:"client_ref"`
	Lines       []Line `json:"lines"`
}

func (m *mod) createDoc(w http.ResponseWriter, r *http.Request) error {
	var in docIn
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	p := auth.From(ctx)
	if rs, ok := reasonByKey(in.Reason); !ok || rs.Internal {
		return httpx.BadRequest("علت حرکت نامعتبر است")
	}
	if in.WarehouseID == 0 {
		id, err := CentralID(ctx, m.DB)
		if err != nil {
			return err
		}
		in.WarehouseID = id
	}
	// Scanner apps resend after a lost connection; the client_ref makes that safe.
	if in.ClientRef != "" {
		if id, err := db.Scalar[int64](ctx, m.DB, `SELECT id FROM stock_documents WHERE client_ref=$1`, in.ClientRef); err == nil {
			return m.writeDoc(w, r, id, http.StatusOK)
		}
	}
	needsApproval := false
	if !p.Can(auth.PWhWrite) {
		if in.Direction == "out" {
			needsApproval = settings.Bool(ctx, m.DB, "wh_approval_out")
		} else {
			needsApproval = settings.Bool(ctx, m.DB, "wh_approval_in")
		}
	}
	// adjustments and openings are an admin matter, never self-service
	if (in.Reason == "opening" || strings.HasPrefix(in.Reason, "adjust_")) && !p.Can(auth.PWhManage) {
		return httpx.ErrForbidden
	}
	var ref *string
	if in.ClientRef != "" {
		ref = &in.ClientRef
	}
	var id int64
	err := m.DB.Tx(ctx, func(tx pgx.Tx) error {
		var e error
		id, e = CreateDoc(ctx, tx, DocIn{Direction: in.Direction, Reason: in.Reason, WarehouseID: in.WarehouseID, DestID: in.DestID,
			WorkerName: strings.TrimSpace(in.WorkerName), Note: in.Note, Approved: !needsApproval, SourceType: "manual",
			CreatedBy: &p.UserID, ClientRef: ref, Lines: in.Lines})
		return e
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "stock.document", "stock_document", id, in.Reason)
	return m.writeDoc(w, r, id, http.StatusCreated)
}

const docSel = `SELECT d.id,d.doc_no,d.direction,d.reason,d.status,d.note,d.worker_name,d.source_type,d.source_id,d.created_at,d.decided_at,d.decision_note,
	d.deleted_at, d.delete_note, d.warehouse_id, d.dest_warehouse_id,
	w.title AS warehouse, (SELECT title FROM warehouses WHERE id=d.dest_warehouse_id) AS dest_warehouse,
	(SELECT COALESCE(NULLIF(full_name,''),username) FROM users WHERE id=d.created_by) AS created_by_name,
	(SELECT COALESCE(NULLIF(full_name,''),username) FROM users WHERE id=d.decided_by) AS decided_by_name,
	(SELECT COUNT(*) FROM stock_document_lines WHERE document_id=d.id)::int AS line_count,
	(SELECT COALESCE(SUM(qty),0) FROM stock_document_lines WHERE document_id=d.id)::int AS total_qty
	FROM stock_documents d JOIN warehouses w ON w.id=d.warehouse_id`

func (m *mod) writeDoc(w http.ResponseWriter, r *http.Request, id int64, status int) error {
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT row_to_json(t) FROM (`+docSel+` WHERE d.id=$1) t`, id)
	if err != nil {
		return err
	}
	lines, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t ORDER BY t.id),'[]') FROM (
		SELECT l.id,l.part_id,p.code,p.name,p.variant,p.unit,l.qty,l.note,l.stock_before,l.stock_after FROM stock_document_lines l JOIN parts p ON p.id=l.part_id WHERE l.document_id=$1) t`, id)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"document":%s,"lines":%s}`, raw, lines)
	return nil
}

func (m *mod) getDoc(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	return m.writeDoc(w, r, id, 200)
}

func (m *mod) listDocs(w http.ResponseWriter, r *http.Request) error {
	p := auth.From(r.Context())
	args := []any{httpx.Q(r, "status"), httpx.Q(r, "direction"), httpx.Q(r, "q"), httpx.Q(r, "from"), httpx.Q(r, "to")}
	where := ` WHERE ($1='' OR d.status=$1) AND ($2='' OR d.direction=$2)
		AND ($3='' OR d.doc_no::text=$3 OR d.worker_name ILIKE '%'||$3||'%' OR EXISTS (SELECT 1 FROM stock_document_lines l JOIN parts pt ON pt.id=l.part_id WHERE l.document_id=d.id AND (pt.name ILIKE '%'||$3||'%' OR pt.code ILIKE '%'||$3||'%')))
		AND ($4='' OR d.created_at >= $4::timestamptz) AND ($5='' OR d.created_at < ($5::date + 1))
		AND d.deleted_at IS NULL`
	// a storekeeper sees only the documents they filed
	if !p.Can(auth.PWhApprove) && !p.Can(auth.PWhManage) {
		args = append(args, p.UserID)
		where += " AND d.created_by=$6"
	}
	total, err := db.Scalar[int](r.Context(), m.DB, `SELECT COUNT(*) FROM stock_documents d`+where, args...)
	if err != nil {
		return err
	}
	pg := httpx.Paging(r, 200)
	n := len(args)
	args = append(args, pg.Limit, pg.Offset)
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, fmt.Sprintf(`SELECT COALESCE(json_agg(t),'[]') FROM (%s%s ORDER BY d.id DESC LIMIT $%d OFFSET $%d) t`,
		docSel, where, n+1, n+2), args...)
	if err != nil {
		return err
	}
	pending, _ := db.Scalar[int](r.Context(), m.DB, `SELECT COUNT(*) FROM stock_documents WHERE status='pending' AND deleted_at IS NULL`)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"items":%s,"total":%d,"page":%d,"limit":%d,"pending":%d}`, raw, total, pg.Page, pg.Limit, pending)
	return nil
}

func (m *mod) approveDoc(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Note string `json:"note"`
	}
	_ = httpx.Decode(r, &in)
	err = m.DB.Tx(r.Context(), func(tx pgx.Tx) error {
		return ApplyPending(r.Context(), tx, id, auth.From(r.Context()).UserID, in.Note)
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "stock.approve", "stock_document", id, "")
	return m.writeDoc(w, r, id, 200)
}

func (m *mod) rejectDoc(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Note string `json:"note"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Note) == "" {
		return httpx.BadRequest("دلیل رد را بنویسید")
	}
	tag, err := m.DB.Exec(r.Context(), `UPDATE stock_documents SET status='rejected', decided_by=$2, decided_at=NOW(), decision_note=$3
		WHERE id=$1 AND status='pending' AND deleted_at IS NULL AND source_type='manual'`, id, auth.From(r.Context()).UserID, in.Note)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.Conflict("این سند در انتظار تایید نیست")
	}
	m.Audit.Log(r, "stock.reject", "stock_document", id, in.Note)
	return m.writeDoc(w, r, id, 200)
}

func (m *mod) cancelDoc(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	p := auth.From(r.Context())
	tag, err := m.DB.Exec(r.Context(), `UPDATE stock_documents SET status='cancelled', decided_at=NOW(), decided_by=$2
		WHERE id=$1 AND status='pending' AND deleted_at IS NULL AND ($3 OR created_by=$2)`, id, p.UserID, p.Can(auth.PWhApprove))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.Conflict("فقط سند در انتظار تایید خودتان قابل لغو است")
	}
	return m.writeDoc(w, r, id, 200)
}

// deleteDoc keeps the row (history is never erased); an approved document's
// effect on stock is reversed, and who/when/why is recorded.
func (m *mod) deleteDoc(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Note string `json:"note"`
	}
	_ = httpx.Decode(r, &in)
	if strings.TrimSpace(in.Note) == "" {
		return httpx.BadRequest("دلیل حذف را بنویسید")
	}
	ctx := r.Context()
	err = m.DB.Tx(ctx, func(tx pgx.Tx) error {
		st, e := db.Scalar[string](ctx, tx, `SELECT status FROM stock_documents WHERE id=$1 AND deleted_at IS NULL AND source_type='manual' FOR UPDATE`, id)
		if e != nil {
			return httpx.Conflict("این سند قابل حذف نیست؛ اسناد گردش کار با خود گردش کار اصلاح می‌شوند")
		}
		if st == "approved" {
			if e = Reverse(ctx, tx, id); e != nil {
				return e
			}
		}
		_, e = tx.Exec(ctx, `UPDATE stock_documents SET deleted_at=NOW(), deleted_by=$2, delete_note=$3, status='cancelled' WHERE id=$1`, id, auth.From(ctx).UserID, in.Note)
		return e
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "stock.delete", "stock_document", id, in.Note)
	httpx.NoContent(w)
	return nil
}

func (m *mod) workers(w http.ResponseWriter, r *http.Request) error {
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(name ORDER BY name),'[]') FROM warehouse_workers WHERE active`)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
	return nil
}

func (m *mod) newWarehouse(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Code  string `json:"code"`
		Title string `json:"title"`
		Kind  string `json:"kind"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.Kind != "central" && in.Kind != "scrap" && in.Kind != "returns" && in.Kind != "products" {
		return httpx.BadRequest("نوع انبار نامعتبر است")
	}
	id, err := db.Scalar[int64](r.Context(), m.DB, `INSERT INTO warehouses(code,title,kind) VALUES($1,$2,$3) RETURNING id`, in.Code, in.Title, in.Kind)
	if err != nil {
		return err
	}
	httpx.Created(w, map[string]any{"id": id})
	return nil
}

// ───────── stock counts ─────────

func (m *mod) newCount(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		WarehouseID int64  `json:"warehouse_id"`
		Title       string `json:"title"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.WarehouseID == 0 {
		id, err := CentralID(r.Context(), m.DB)
		if err != nil {
			return err
		}
		in.WarehouseID = id
	}
	id, err := db.Scalar[int64](r.Context(), m.DB, `INSERT INTO stock_counts(warehouse_id,title,created_by) VALUES($1,$2,$3) RETURNING id`,
		in.WarehouseID, in.Title, auth.From(r.Context()).UserID)
	if err != nil {
		return err
	}
	httpx.Created(w, map[string]any{"id": id})
	return nil
}

func (m *mod) listCounts(w http.ResponseWriter, r *http.Request) error {
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t ORDER BY t.id DESC),'[]') FROM (
		SELECT c.id,c.title,c.status,c.created_at,c.warehouse_id,(SELECT COUNT(*) FROM stock_count_lines WHERE count_id=c.id)::int AS lines FROM stock_counts c) t`)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
	return nil
}

func (m *mod) getCount(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	blind := settings.Bool(r.Context(), m.DB, "wh_count_blind") && !auth.From(r.Context()).Can(auth.PWhManage)
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT row_to_json(t) FROM (
		SELECT c.id,c.title,c.status,c.warehouse_id,c.created_at,c.result,
		COALESCE((SELECT json_agg(x ORDER BY x.name) FROM (
		  SELECT l.part_id,p.code,p.name,l.counted,
		    CASE WHEN $2 AND c.status='open' THEN NULL ELSE COALESCE(l.expected,(SELECT qty FROM stock_levels WHERE warehouse_id=c.warehouse_id AND part_id=l.part_id),0) END AS expected
		  FROM stock_count_lines l JOIN parts p ON p.id=l.part_id WHERE l.count_id=c.id) x),'[]') AS lines
		FROM stock_counts c WHERE c.id=$1) t`, id, blind)
	if err != nil || len(raw) == 0 {
		return httpx.ErrNotFound
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
	return nil
}

func (m *mod) countLine(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		PartID  int64 `json:"part_id"`
		Counted int   `json:"counted"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	st, err := db.Scalar[string](ctx, m.DB, `SELECT status FROM stock_counts WHERE id=$1`, id)
	if err != nil || st != "open" {
		return httpx.Conflict("این انبارگردانی بسته شده است")
	}
	_, err = m.DB.Exec(ctx, `INSERT INTO stock_count_lines(count_id,part_id,counted,counted_by) VALUES($1,$2,$3,$4)
		ON CONFLICT (count_id,part_id) DO UPDATE SET counted=EXCLUDED.counted, counted_by=EXCLUDED.counted_by, counted_at=NOW()`,
		id, in.PartID, in.Counted, auth.From(ctx).UserID)
	httpx.NoContent(w)
	return err
}

func (m *mod) submitCount(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	// freeze what the system believed at submission time
	err = m.DB.Tx(r.Context(), func(tx pgx.Tx) error {
		tag, e := tx.Exec(r.Context(), `UPDATE stock_counts SET status='submitted', submitted_by=$2, submitted_at=NOW() WHERE id=$1 AND status='open'`, id, auth.From(r.Context()).UserID)
		if e != nil || tag.RowsAffected() == 0 {
			if e == nil {
				e = httpx.Conflict("این انبارگردانی قبلاً ارسال شده است")
			}
			return e
		}
		_, e = tx.Exec(r.Context(), `UPDATE stock_count_lines l SET expected=COALESCE((SELECT qty FROM stock_levels WHERE warehouse_id=c.warehouse_id AND part_id=l.part_id),0)
			FROM stock_counts c WHERE c.id=l.count_id AND c.id=$1`, id)
		return e
	})
	if err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

func (m *mod) rejectCount(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	_, err = m.DB.Exec(r.Context(), `UPDATE stock_counts SET status='rejected', decided_by=$2, decided_at=NOW() WHERE id=$1 AND status='submitted'`, id, auth.From(r.Context()).UserID)
	httpx.NoContent(w)
	return err
}

// applyCount turns differences into two approved adjustment documents.
func (m *mod) applyCount(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	ctx := r.Context()
	uid := auth.From(ctx).UserID
	var inUp, outUp int
	err = m.DB.Tx(ctx, func(tx pgx.Tx) error {
		type c struct {
			WH int64 `db:"warehouse_id"`
		}
		cc, e := db.One[c](ctx, tx, `SELECT warehouse_id FROM stock_counts WHERE id=$1 AND status='submitted' FOR UPDATE`, id)
		if e != nil {
			return httpx.Conflict("فقط انبارگردانی ارسال‌شده قابل اعمال است")
		}
		type diff struct {
			Part int64 `db:"part_id"`
			D    int   `db:"d"`
		}
		ds, e := db.All[diff](ctx, tx, `SELECT l.part_id, l.counted - COALESCE((SELECT qty FROM stock_levels WHERE warehouse_id=$2 AND part_id=l.part_id),0) AS d
			FROM stock_count_lines l WHERE l.count_id=$1`, id, cc.WH)
		if e != nil {
			return e
		}
		var up, down []Line
		for _, d := range ds {
			if d.D > 0 {
				up = append(up, Line{PartID: d.Part, Qty: d.D, Note: "انبارگردانی"})
			} else if d.D < 0 {
				down = append(down, Line{PartID: d.Part, Qty: -d.D, Note: "انبارگردانی"})
			}
		}
		sid := id
		if len(up) > 0 {
			if _, e = CreateDoc(ctx, tx, DocIn{Direction: "in", Reason: "adjust_in", WarehouseID: cc.WH, Approved: true, SourceType: "count", SourceID: &sid, CreatedBy: &uid, Lines: up}); e != nil {
				return e
			}
		}
		if len(down) > 0 {
			if _, e = CreateDoc(ctx, tx, DocIn{Direction: "out", Reason: "adjust_out", WarehouseID: cc.WH, Approved: true, SourceType: "count", SourceID: &sid, CreatedBy: &uid, Lines: down}); e != nil {
				return e
			}
		}
		inUp, outUp = len(up), len(down)
		_, e = tx.Exec(ctx, `UPDATE stock_counts SET status='applied', decided_by=$2, decided_at=NOW(), result=$3 WHERE id=$1`, id, uid,
			fmt.Sprintf("افزایش: %d قلم، کاهش: %d قلم", len(up), len(down)))
		return e
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "stock.count_apply", "stock_count", id, "")
	httpx.OK(w, map[string]any{"increased": inUp, "decreased": outUp})
	return nil
}

// ───────── exports ─────────

func (m *mod) exportMovements(w http.ResponseWriter, r *http.Request) error {
	rows, err := m.DB.Query(r.Context(), `SELECT d.doc_no, d.direction, d.reason, to_char(COALESCE(d.decided_at,d.created_at) AT TIME ZONE 'Asia/Tehran','YYYY-MM-DD HH24:MI'),
		p.code, p.name, l.qty, l.stock_before, l.stock_after, d.worker_name, d.status,
		(SELECT username FROM users WHERE id=d.created_by), (SELECT username FROM users WHERE id=d.decided_by), d.note
		FROM stock_documents d JOIN stock_document_lines l ON l.document_id=d.id JOIN parts p ON p.id=l.part_id
		WHERE d.deleted_at IS NULL AND ($1='' OR d.created_at >= $1::timestamptz) AND ($2='' OR d.created_at < ($2::date + 1))
		ORDER BY d.id, l.id`, httpx.Q(r, "from"), httpx.Q(r, "to"))
	if err != nil {
		return err
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="movements-`+time.Now().Format("20060102")+`.csv"`)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM so Excel reads Persian correctly
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"شماره سند", "جهت", "علت", "زمان (تهران)", "کد قطعه", "نام قطعه", "تعداد", "موجودی قبل", "موجودی بعد", "کارگر", "وضعیت", "ثبت‌کننده", "تاییدکننده", "توضیح"})
	for rows.Next() {
		var docNo int64
		var dir, reason, at, code, name, worker, status, note string
		var qty int
		var before, after *int
		var by, approver *string
		if err := rows.Scan(&docNo, &dir, &reason, &at, &code, &name, &qty, &before, &after, &worker, &status, &by, &approver, &note); err != nil {
			return err
		}
		_ = cw.Write([]string{fmt.Sprint(docNo), dir, reason, at, code, name, fmt.Sprint(qty), ptr(before), ptr(after), worker, status, sp(by), sp(approver), note})
	}
	cw.Flush()
	return rows.Err()
}

func ptr(n *int) string {
	if n == nil {
		return ""
	}
	return fmt.Sprint(*n)
}
func sp(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// exportSepidar writes the barcode text file Sepidar reads: one line per unit,
// containing the item's Sepidar code. ?doc_ids=1,2,3 selects approved documents.
func (m *mod) exportSepidar(w http.ResponseWriter, r *http.Request) error {
	var ids []int64
	for _, s := range strings.Split(httpx.Q(r, "doc_ids"), ",") {
		var n int64
		if _, err := fmt.Sscan(strings.TrimSpace(s), &n); err == nil && n > 0 {
			ids = append(ids, n)
		}
	}
	if len(ids) == 0 {
		return httpx.BadRequest("شناسه سند را مشخص کنید")
	}
	rows, err := m.DB.Query(r.Context(), `SELECT COALESCE(NULLIF(p.sepidar_code,''),p.code), l.qty FROM stock_document_lines l
		JOIN stock_documents d ON d.id=l.document_id AND d.status='approved' AND d.deleted_at IS NULL
		JOIN parts p ON p.id=l.part_id WHERE d.id = ANY($1) ORDER BY d.id, l.id`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="sepidar-barcode.txt"`)
	var b strings.Builder
	for rows.Next() {
		var code string
		var qty int
		if err := rows.Scan(&code, &qty); err != nil {
			return err
		}
		if qty > 50000 {
			qty = 50000
		}
		for i := 0; i < qty; i++ {
			b.WriteString(code)
			b.WriteString("\r\n")
		}
	}
	_, _ = w.Write([]byte(b.String()))
	_ = jalali.Now
	return rows.Err()
}
