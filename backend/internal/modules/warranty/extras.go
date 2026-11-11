package warranty

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
	"github.com/evinki/platform/internal/platform/jalali"
)

func (m *mod) mountExtras(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PCatalogManage))
		r.Post("/admin/code-formula/preview", httpx.H(m.formulaPreview))
		r.Post("/admin/products/{id}/codes/generate", httpx.H(m.generateCodes))
	})
	r.With(auth.Require(auth.PCatalogDelete)).Post("/admin/products/{id}/merge", httpx.H(m.mergeProducts))

	r.With(auth.Require(auth.PWarrantyRead)).Get("/admin/guarantees-export", httpx.H(m.export))
	r.With(auth.Require(auth.PWarrantyRead, auth.PServiceRead)).Get("/admin/search", httpx.H(m.search))

	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PWarrantyDelete))
		r.Get("/admin/trash", httpx.H(m.trashSummary))
		r.Get("/admin/trash/{kind}", httpx.H(m.trashList))
		r.Post("/admin/trash/{kind}/{id}/restore", httpx.H(m.trashRestore))
	})
}

// ───────── formula builder ─────────

func (m *mod) formulaPreview(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Formula Formula `json:"formula"`
		Date    string  `json:"date"`
		Serial  int64   `json:"serial"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if err := in.Formula.Validate(); err != nil {
		httpx.OK(w, map[string]any{"valid": false, "error": err.Error()})
		return nil
	}
	d := time.Now()
	if in.Date != "" {
		if t, err := parseDate(in.Date); err == nil {
			d = t
		}
	}
	if in.Serial <= 0 {
		in.Serial = 1
	}
	code := in.Formula.Render(time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, jalali.Tehran), in.Serial)
	out := map[string]any{"valid": true, "example": code, "regex": in.Formula.Regex()}
	if mf, approx, ok := func() (time.Time, bool, bool) {
		p, ok := in.Formula.Parse(code)
		if !ok {
			return time.Time{}, false, false
		}
		return p.ManufactureDate()
	}(); ok {
		out["manufacture_date"], out["date_approx"] = mf.Format("2006-01-02"), approx
	}
	httpx.OK(w, out)
	return nil
}

// generateCodes produces a batch of codes (for printing on cards / loading into the production line).
func (m *mod) generateCodes(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Date  string `json:"date"`
		Start int64  `json:"start"`
		Count int    `json:"count"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT code_formula FROM products WHERE id=$1`, id)
	if err != nil {
		return err
	}
	f := parseFormula(raw)
	if f == nil {
		return httpx.BadRequest("برای این محصول فرمول کد تعریف نشده است")
	}
	if in.Count < 1 || in.Count > 5000 {
		return httpx.BadRequest("تعداد باید بین ۱ تا ۵۰۰۰ باشد")
	}
	if in.Start < 1 {
		in.Start = 1
	}
	d := time.Now()
	if in.Date != "" {
		t, e := parseDate(in.Date)
		if e != nil {
			return httpx.BadRequest("تاریخ نامعتبر است")
		}
		d = t
	}
	at := time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, jalali.Tehran)
	codes := make([]string, 0, in.Count)
	for i := 0; i < in.Count; i++ {
		codes = append(codes, f.Render(at, in.Start+int64(i)))
	}
	m.Audit.Log(r, "product.codes_generated", "product", id, fmt.Sprintf("%d codes from %d", in.Count, in.Start))
	if httpx.Q(r, "format") == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="codes.csv"`)
		_, _ = w.Write([]byte(strings.Join(codes, "\r\n") + "\r\n"))
		return nil
	}
	httpx.OK(w, map[string]any{"codes": codes})
	return nil
}

// ───────── merge two products ─────────

// mergeProducts moves everything that points at product {id} (guarantees, repairs, orders, carts ...) to the
// target, fills the target's empty fields from it and retires the source. Used when the guarantee system's
// "FZ530" and the shop's "FZ530 vacuum cleaner" turn out to be one product.
func (m *mod) mergeProducts(w http.ResponseWriter, r *http.Request) error {
	from, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Into int64 `json:"into"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.Into == 0 || in.Into == from {
		return httpx.BadRequest("محصول مقصد را انتخاب کنید")
	}
	moved := map[string]int64{}
	err = m.DB.Tx(r.Context(), func(tx pgx.Tx) error {
		ctx := r.Context()
		if n, _ := db.Scalar[int](ctx, tx, `SELECT COUNT(*) FROM products WHERE id IN ($1,$2) AND deleted_at IS NULL`, from, in.Into); n != 2 {
			return httpx.BadRequest("محصول پیدا نشد")
		}
		type fk struct{ Table, Col string }
		fks, err := db.All[struct {
			T string `db:"t"`
			C string `db:"c"`
		}](ctx, tx, `SELECT c.conrelid::regclass::text AS t, a.attname AS c FROM pg_constraint c
			JOIN pg_attribute a ON a.attrelid=c.conrelid AND a.attnum=c.conkey[1]
			WHERE c.contype='f' AND c.confrelid='products'::regclass AND array_length(c.conkey,1)=1`)
		if err != nil {
			return err
		}
		for _, f := range fks {
			if _, err := tx.Exec(ctx, `SAVEPOINT mp`); err != nil {
				return err
			}
			tag, err := tx.Exec(ctx, fmt.Sprintf(`UPDATE %s SET %s=$1 WHERE %s=$2`, f.T, f.C, f.C), in.Into, from)
			if db.IsUnique(err) { // both products were in the same wishlist / compat list: keep the target's row
				if _, e := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT mp`); e != nil {
					return e
				}
				tag, err = tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s WHERE %s=$1`, f.T, f.C), from)
			}
			if err != nil {
				return err
			}
			if tag.RowsAffected() > 0 {
				moved[f.T] += tag.RowsAffected()
			}
			if _, err := tx.Exec(ctx, `RELEASE SAVEPOINT mp`); err != nil {
				return err
			}
		}
		// the target keeps its own data; only blanks are filled from the source
		_, err = tx.Exec(ctx, `UPDATE products t SET
			category_id = COALESCE(t.category_id, s.category_id),
			summary     = CASE WHEN t.summary='' THEN s.summary ELSE t.summary END,
			description = CASE WHEN t.description='' THEN s.description ELSE t.description END,
			name_en     = CASE WHEN t.name_en='' THEN s.name_en ELSE t.name_en END,
			sepidar_code= CASE WHEN t.sepidar_code='' THEN s.sepidar_code ELSE t.sepidar_code END,
			images      = CASE WHEN t.images='[]'::jsonb THEN s.images ELSE t.images END,
			specs       = CASE WHEN t.specs='{}'::jsonb THEN s.specs ELSE t.specs END,
			code_format = CASE WHEN t.code_format='simple' AND s.code_format<>'simple' THEN s.code_format ELSE t.code_format END,
			code_prefix = CASE WHEN t.code_prefix='' THEN s.code_prefix ELSE t.code_prefix END,
			code_pattern= CASE WHEN t.code_pattern='' THEN s.code_pattern ELSE t.code_pattern END,
			code_formula= CASE WHEN t.code_formula='[]'::jsonb THEN s.code_formula ELSE t.code_formula END,
			updated_at=NOW()
			FROM products s WHERE t.id=$1 AND s.id=$2`, in.Into, from)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE products SET deleted_at=NOW(), is_active=FALSE, is_published=FALSE, sku=sku||'~merged'||id, slug=slug||'~merged'||id WHERE id=$1`, from)
		return err
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "product.merge", "product", from, "into "+strconv.FormatInt(in.Into, 10))
	httpx.OK(w, map[string]any{"merged": true, "moved": moved})
	return nil
}

// ───────── export ─────────

func (m *mod) export(w http.ResponseWriter, r *http.Request) error {
	args := []any{httpx.Q(r, "status"), httpx.Q(r, "q"), httpx.Q(r, "code_from"), httpx.Q(r, "code_to")}
	where := ` WHERE g.deleted_at IS NULL AND ($1='' OR g.status=$1)
		AND ($2='' OR g.code ILIKE '%'||$2||'%' OR c.full_name ILIKE '%'||$2||'%' OR c.phone LIKE '%'||$2||'%')
		AND ($3='' OR g.code >= $3) AND ($4='' OR g.code <= $4)`
	if v, ok := httpx.QInt(r, "product_id"); ok {
		args = append(args, v)
		where += " AND g.product_id=$" + itoa(len(args))
	}
	if v, ok := httpx.QInt(r, "expiring_in_days"); ok {
		args = append(args, v)
		where += " AND g.status IN ('approved','renewed') AND g.expiry_date BETWEEN CURRENT_DATE AND CURRENT_DATE + ($" + itoa(len(args)) + " * INTERVAL '1 day')"
	}
	items, err := db.All[G](r.Context(), m.DB, gSel+where+` ORDER BY g.code LIMIT 50000`, args...)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="guarantees.csv"`)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"کد گارانتی", "وضعیت", "محصول", "مشتری", "موبایل", "شهر", "تاریخ تولید", "تاریخ خرید", "پایان گارانتی", "پایان طلایی", "روز باقی‌مانده", "منبع", "یادداشت"})
	jd := func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return jalali.Format(*t)
	}
	for _, g := range items {
		exp := g.ExpiryDate
		_ = cw.Write([]string{g.Code, g.Status, g.ProductName, g.CustomerName, g.CustomerPhone, g.CustomerCity, jd(g.ManufactureDate),
			jalali.Format(g.PurchaseDate), jalali.Format(exp), jd(g.GoldenExpiryDate), strconv.Itoa(g.DaysRemaining), g.Source, g.Notes})
	}
	cw.Flush()
	return nil
}

// ───────── trash (soft-deleted records) ─────────

var trashKinds = map[string]struct{ table, label, title string }{
	"guarantees": {"guarantees", "code", "گارانتی"},
	"customers":  {"customers", "full_name", "مشتری"},
	"products":   {"products", "name", "محصول"},
	"repairs":    {"repairs", "number", "تعمیر"},
	"parts":      {"parts", "name", "قطعه"},
}

func (m *mod) trashSummary(w http.ResponseWriter, r *http.Request) error {
	out := map[string]int{}
	for k, v := range trashKinds {
		n, err := db.Scalar[int](r.Context(), m.DB, `SELECT COUNT(*) FROM `+v.table+` WHERE deleted_at IS NOT NULL`)
		if err != nil {
			return err
		}
		out[k] = n
	}
	httpx.OK(w, out)
	return nil
}

func (m *mod) trashList(w http.ResponseWriter, r *http.Request) error {
	k, ok := trashKinds[chi.URLParam(r, "kind")]
	if !ok {
		return httpx.ErrNotFound
	}
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
		SELECT id, `+k.label+`::text AS label, deleted_at FROM `+k.table+` WHERE deleted_at IS NOT NULL ORDER BY deleted_at DESC LIMIT 300) t`)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
	return nil
}

func (m *mod) trashRestore(w http.ResponseWriter, r *http.Request) error {
	k, ok := trashKinds[chi.URLParam(r, "kind")]
	if !ok {
		return httpx.ErrNotFound
	}
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	_, err = restore(r.Context(), m.DB, k.table, id)
	if db.IsUnique(err) {
		return httpx.Conflict("نمی‌توان بازیابی کرد: مورد مشابه فعالی وجود دارد (مثلاً همان کد گارانتی دوباره ثبت شده است)")
	}
	if err != nil {
		return err
	}
	m.Audit.Log(r, "trash.restore", k.table, id, "")
	httpx.NoContent(w)
	return nil
}

func restore(ctx context.Context, q db.Q, table string, id int64) (int64, error) {
	tag, err := q.Exec(ctx, `UPDATE `+table+` SET deleted_at=NULL WHERE id=$1 AND deleted_at IS NOT NULL`, id)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// BackfillManufacture fills manufacture_date for guarantees that have none (imported ones, mostly) by reading the
// code with the product's rules. It never touches the expiry date. Returns how many rows were filled.
func BackfillManufacture(ctx context.Context, q db.Q) (int, error) {
	type row struct {
		ID      int64           `db:"id"`
		Code    string          `db:"code"`
		Format  string          `db:"code_format"`
		Prefix  string          `db:"code_prefix"`
		Pattern string          `db:"code_pattern"`
		Formula json.RawMessage `db:"code_formula"`
	}
	rows, err := db.All[row](ctx, q, `SELECT g.id,g.code,p.code_format,p.code_prefix,p.code_pattern,p.code_formula FROM guarantees g
		JOIN products p ON p.id=g.product_id WHERE g.manufacture_date IS NULL AND g.deleted_at IS NULL`)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		mfg, approx := mfgFromCode(ProductRules{Format: r.Format, Prefix: r.Prefix, Pattern: r.Pattern, Formula: parseFormula(r.Formula)}, r.Code)
		if mfg == nil {
			continue
		}
		if _, err := q.Exec(ctx, `UPDATE guarantees SET manufacture_date=$2, manufacture_approx=$3 WHERE id=$1`, r.ID, mfg, approx); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ───────── global search (header box) ─────────

func (m *mod) search(w http.ResponseWriter, r *http.Request) error {
	q := strings.TrimSpace(httpx.Digits(httpx.Q(r, "q")))
	if len([]rune(q)) < 2 {
		httpx.OK(w, map[string]any{"guarantees": []any{}, "customers": []any{}, "products": []any{}, "repairs": []any{}, "technicians": []any{}})
		return nil
	}
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT json_build_object(
	  'guarantees', COALESCE((SELECT json_agg(t) FROM (SELECT g.id, g.code AS label, c.full_name AS sub FROM guarantees g JOIN customers c ON c.id=g.customer_id
	      WHERE g.deleted_at IS NULL AND (g.code ILIKE '%'||$1||'%' OR c.full_name ILIKE '%'||$1||'%' OR c.phone LIKE '%'||$1||'%') ORDER BY g.id DESC LIMIT 6) t),'[]'),
	  'customers', COALESCE((SELECT json_agg(t) FROM (SELECT id, full_name AS label, phone AS sub FROM customers
	      WHERE deleted_at IS NULL AND (full_name ILIKE '%'||$1||'%' OR phone LIKE '%'||$1||'%' OR national_id=$1) ORDER BY id DESC LIMIT 6) t),'[]'),
	  'products', COALESCE((SELECT json_agg(t) FROM (SELECT id, name AS label, sku AS sub FROM products
	      WHERE deleted_at IS NULL AND (name ILIKE '%'||$1||'%' OR sku ILIKE '%'||$1||'%' OR name_en ILIKE '%'||$1||'%') ORDER BY id LIMIT 6) t),'[]'),
	  'repairs', COALESCE((SELECT json_agg(t) FROM (SELECT id, number AS label, complaint AS sub FROM repairs
	      WHERE deleted_at IS NULL AND (number ILIKE '%'||$1||'%' OR complaint ILIKE '%'||$1||'%') ORDER BY id DESC LIMIT 6) t),'[]'),
	  'technicians', COALESCE((SELECT json_agg(t) FROM (SELECT u.id, u.full_name AS label, u.phone AS sub FROM technicians tc JOIN users u ON u.id=tc.user_id
	      WHERE u.deleted_at IS NULL AND (u.full_name ILIKE '%'||$1||'%' OR u.phone LIKE '%'||$1||'%') ORDER BY u.id LIMIT 6) t),'[]'))`, q)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
	return nil
}
