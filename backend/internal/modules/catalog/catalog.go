// Package catalog: categories, products, spare parts and service items.
// Public read endpoints feed the storefront; admin endpoints are mostly
// declarative CRUD.
package catalog

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/evinki/platform/internal/app"
	"github.com/evinki/platform/internal/modules/warranty"
	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/crud"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
)

const qrAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// NewQRToken: upper-case + digits without look-alikes, keeps QR codes compact.
func NewQRToken() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = qrAlphabet[int(b[i])%len(qrAlphabet)]
	}
	return string(b)
}

func Mount(r chi.Router, d *app.Deps) {
	m := &mod{d}

	// ───── public storefront reads ─────
	r.Get("/shop/categories", httpx.H(m.publicCategories))
	r.Get("/shop/products", httpx.H(m.publicProducts))
	r.Get("/shop/products/{slug}", httpx.H(m.publicProduct))
	r.Get("/shop/parts", httpx.H(m.publicParts))

	// ───── admin ─────
	cats := &crud.Resource{Name: "category", Table: "categories", DB: d.DB, Audit: d.Audit, TouchUpdated: true,
		Select: `categories.*, (SELECT COUNT(*) FROM products p WHERE p.category_id=categories.id AND p.deleted_at IS NULL)::int AS product_count`,
		Cols: map[string]crud.Kind{"parent_id": crud.Int, "slug": crud.Text, "name": crud.Text, "name_en": crud.Text,
			"description": crud.Text, "sort_order": crud.Int, "is_active": crud.Bool},
		Required: []string{"slug", "name"}, Search: []string{"name", "slug"}, Order: "sort_order, id"}
	cats.Mount(r, "/admin/categories", crud.Perms{Read: auth.PCatalogManage, Write: auth.PCatalogManage, Delete: auth.PCatalogDelete})

	products := &crud.Resource{Name: "product", Table: "products", DB: d.DB, Audit: d.Audit, SoftDelete: true, TouchUpdated: true,
		Select: `products.*, (SELECT name FROM categories c WHERE c.id=products.category_id) AS category_name`,
		Cols: map[string]crud.Kind{"category_id": crud.Int, "sku": crud.Text, "slug": crud.Text, "name": crud.Text, "name_en": crud.Text,
			"summary": crud.Text, "description": crud.Text, "specs": crud.JSON, "images": crud.JSON, "price": crud.Int,
			"compare_price": crud.Int, "price_on_request": crud.Bool, "stock_qty": crud.Int, "track_stock": crud.Bool,
			"is_published": crud.Bool, "is_active": crud.Bool, "guarantee_months": crud.Int, "golden_months": crud.Int,
			"code_format": crud.Text, "code_prefix": crud.Text, "code_pattern": crud.Text, "sepidar_code": crud.Text,
			"code_formula": crud.JSON, "warranty_basis": crud.Text},
		Required: []string{"sku", "slug", "name"}, Search: []string{"name", "sku", "name_en"},
		Check: func(_ *http.Request, _ bool, b crud.Body) error {
			if raw, ok := b["code_formula"]; ok {
				var f warranty.Formula
				if json.Unmarshal(raw, &f) != nil {
					return httpx.BadRequest("فرمول کد نامعتبر است")
				}
				if len(f) > 0 {
					if err := f.Validate(); err != nil {
						return httpx.BadRequest(err.Error())
					}
					b["code_format"] = json.RawMessage(`"formula"`) // a formula is the product's code format
				}
			}
			return nil
		},
		Filters: map[string]string{"category_id": "category_id", "published": "is_published"}}
	products.Mount(r, "/admin/products", crud.Perms{Read: auth.PCatalogManage, Write: auth.PCatalogManage, Delete: auth.PCatalogDelete})

	parts := &crud.Resource{Name: "part", Table: "parts", DB: d.DB, Audit: d.Audit, SoftDelete: true, TouchUpdated: true,
		Select: `parts.*, COALESCE((SELECT SUM(sl.qty) FROM stock_levels sl JOIN warehouses w ON w.id=sl.warehouse_id AND w.kind='central'
			WHERE sl.part_id=parts.id),0)::int AS stock`,
		Cols: map[string]crud.Kind{"code": crud.Text, "sepidar_code": crud.Text, "part_no": crud.Text, "name": crud.Text,
			"variant": crud.Text, "material": crud.Text, "weight_g": crud.Float, "unit": crud.Text, "location": crud.Text,
			"min_stock": crud.Int, "image": crud.Text, "note": crud.Text, "sale_price": crud.Int, "buyback_price": crud.Int,
			"is_sellable": crud.Bool, "return_required": crud.Bool, "is_active": crud.Bool},
		Required: []string{"code", "name"}, Search: []string{"name", "code", "part_no", "sepidar_code"},
		Filters: map[string]string{"sellable": "is_sellable"},
	}
	parts.Mount(r, "/admin/parts", crud.Perms{Read: auth.PCatalogManage, Write: auth.PCatalogManage, Delete: auth.PCatalogDelete})
	// Registered after Mount so it replaces the generic POST: qr_token is server-generated.
	r.With(auth.Require(auth.PCatalogManage, auth.PWhManage)).Post("/admin/parts", httpx.H(m.createPart))

	r.With(auth.Require(auth.PCatalogManage)).Put("/admin/parts/{id}/compat", httpx.H(m.setCompat))
	r.With(auth.Require(auth.PCatalogManage, auth.PServiceRead, auth.PWhRead, auth.PTechPortal)).Get("/admin/parts/{id}/compat", httpx.H(m.getCompat))

	svc := &crud.Resource{Name: "service_item", Table: "service_items", DB: d.DB, Audit: d.Audit, SoftDelete: true, TouchUpdated: true,
		Cols:     map[string]crud.Kind{"name": crud.Text, "description": crud.Text, "fee": crud.Int, "is_active": crud.Bool},
		Required: []string{"name"}, Search: []string{"name"}, Order: "name"}
	svc.Mount(r, "/admin/service-items", crud.Perms{Read: auth.PServiceRead, Write: auth.PCatalogManage, Delete: auth.PCatalogDelete})

	// Technicians need to read the part and service lists to build a report.
	r.With(auth.Require(auth.PTechPortal, auth.PServiceRead, auth.PWhRead)).Get("/lookup/parts", httpx.H(m.lookupParts))
	r.With(auth.Require(auth.PTechPortal, auth.PServiceRead)).Get("/lookup/service-items", httpx.H(m.lookupServices))
	r.With(auth.Require(auth.PTechPortal, auth.PServiceRead, auth.PWarrantyRead, auth.PCustomerPortal)).Get("/lookup/products", httpx.H(m.lookupProducts))
}

type mod struct{ *app.Deps }

func (m *mod) createPart(w http.ResponseWriter, r *http.Request) error {
	var b map[string]json.RawMessage
	if err := httpx.Decode(r, &b); err != nil {
		return err
	}
	str := func(k string) string {
		var s string
		_ = json.Unmarshal(b[k], &s)
		return strings.TrimSpace(s)
	}
	num := func(k string) int64 {
		var n json.Number
		_ = json.Unmarshal(b[k], &n)
		v, _ := n.Int64()
		return v
	}
	flag := func(k string, def bool) bool {
		if raw, ok := b[k]; ok {
			var v bool
			_ = json.Unmarshal(raw, &v)
			return v
		}
		return def
	}
	if str("code") == "" || str("name") == "" {
		return httpx.BadRequest("کد و نام قطعه الزامی است")
	}
	var id int64
	var err error
	for i := 0; i < 5; i++ { // token collisions are astronomically rare; retry anyway
		id, err = db.Scalar[int64](r.Context(), m.DB, `INSERT INTO parts(code,sepidar_code,part_no,name,variant,material,unit,location,
			min_stock,image,note,sale_price,buyback_price,is_sellable,return_required,qr_token)
			VALUES($1,$2,$3,$4,$5,$6,COALESCE(NULLIF($7,''),'عدد'),$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id`,
			str("code"), str("sepidar_code"), str("part_no"), str("name"), str("variant"), str("material"), str("unit"),
			str("location"), num("min_stock"), str("image"), str("note"), num("sale_price"), num("buyback_price"),
			flag("is_sellable", false), flag("return_required", true), NewQRToken())
		if err == nil || !db.IsUnique(err) || db.ConstraintName(err) != "parts_qr_token_key" {
			break
		}
	}
	if err != nil {
		return err
	}
	m.Audit.Log(r, "part.create", "part", id, str("code"))
	p, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT row_to_json(p) FROM parts p WHERE id=$1`, id)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(p)
	return nil
}

func (m *mod) setCompat(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		ProductIDs []int64 `json:"product_ids"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	err = m.DB.Tx(r.Context(), func(tx pgxTx) error {
		if _, e := tx.Exec(r.Context(), `DELETE FROM part_compat WHERE part_id=$1`, id); e != nil {
			return e
		}
		for _, pid := range in.ProductIDs {
			if _, e := tx.Exec(r.Context(), `INSERT INTO part_compat(part_id,product_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, pid); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

func (m *mod) getCompat(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(product_id),'[]') FROM part_compat WHERE part_id=$1`, id)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"product_ids":` + string(raw) + `}`))
	return nil
}

// ───── public ─────

func (m *mod) publicCategories(w http.ResponseWriter, r *http.Request) error {
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
		SELECT c.id,c.parent_id,c.slug,c.name,c.name_en,c.description,
		  (SELECT COUNT(*) FROM products p WHERE p.category_id=c.id AND p.deleted_at IS NULL AND p.is_published AND p.is_active)::int AS product_count
		FROM categories c WHERE c.is_active ORDER BY c.sort_order, c.id) t`)
	if err != nil {
		return err
	}
	writeRaw(w, raw)
	return nil
}

const publicProductCols = `p.id,p.slug,p.sku,p.name,p.name_en,p.summary,p.images,p.price,p.compare_price,p.price_on_request,
	(NOT p.track_stock OR p.stock_qty>0) AS in_stock,p.guarantee_months,
	(SELECT slug FROM categories c WHERE c.id=p.category_id) AS category_slug,
	(SELECT name FROM categories c WHERE c.id=p.category_id) AS category_name`

func (m *mod) publicProducts(w http.ResponseWriter, r *http.Request) error {
	args := []any{httpx.Q(r, "q"), httpx.Q(r, "category")}
	where := ` WHERE p.deleted_at IS NULL AND p.is_published AND p.is_active
		AND ($1='' OR p.name ILIKE '%'||$1||'%' OR p.name_en ILIKE '%'||$1||'%' OR p.sku ILIKE '%'||$1||'%')
		AND ($2='' OR p.category_id IN (SELECT id FROM categories WHERE slug=$2 OR parent_id=(SELECT id FROM categories WHERE slug=$2)))`
	if v, ok := httpx.QInt(r, "min"); ok {
		args = append(args, v)
		where += " AND p.price >= $" + itoa(len(args))
	}
	if v, ok := httpx.QInt(r, "max"); ok {
		args = append(args, v)
		where += " AND p.price <= $" + itoa(len(args))
	}
	if httpx.Q(r, "stock") == "1" {
		where += " AND (NOT p.track_stock OR p.stock_qty>0)"
	}
	if httpx.Q(r, "priced") == "1" {
		where += " AND p.price>0 AND NOT p.price_on_request"
	}
	order := "p.id DESC"
	switch httpx.Q(r, "sort") {
	case "price_asc":
		order = "p.price ASC, p.id"
	case "price_desc":
		order = "p.price DESC, p.id"
	}
	total, err := db.Scalar[int](r.Context(), m.DB, `SELECT COUNT(*) FROM products p`+where, args...)
	if err != nil {
		return err
	}
	pg := httpx.Paging(r, 60)
	args = append(args, pg.Limit, pg.Offset)
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (SELECT `+publicProductCols+
		` FROM products p`+where+` ORDER BY `+order+` LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args))+`) t`, args...)
	if err != nil {
		return err
	}
	writeRaw(w, []byte(`{"items":`+string(raw)+`,"total":`+itoa(total)+`,"page":`+itoa(pg.Page)+`,"limit":`+itoa(pg.Limit)+`}`))
	return nil
}

func (m *mod) publicProduct(w http.ResponseWriter, r *http.Request) error {
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT row_to_json(t) FROM (
		SELECT `+publicProductCols+`,p.description,p.specs,p.golden_months,
		COALESCE((SELECT json_agg(a) FROM (SELECT pt.id,pt.name,pt.image,pt.sale_price AS price FROM part_compat pc
			JOIN parts pt ON pt.id=pc.part_id AND pt.deleted_at IS NULL AND pt.is_sellable AND pt.is_active
			WHERE pc.product_id=p.id ORDER BY pt.name) a),'[]') AS accessories
		,COALESCE((SELECT json_agg(rp) FROM (SELECT r.id,r.slug,r.name,r.images,r.price,r.price_on_request,r.compare_price,r.summary,(NOT r.track_stock OR r.stock_qty>0) AS in_stock
			FROM products r WHERE r.id<>p.id AND r.deleted_at IS NULL AND r.is_published AND r.is_active AND r.category_id IS NOT DISTINCT FROM p.category_id ORDER BY r.id DESC LIMIT 8) rp),'[]') AS related
		FROM products p WHERE p.slug=$1 AND p.deleted_at IS NULL AND p.is_published AND p.is_active) t`, chi.URLParam(r, "slug"))
	if err != nil || len(raw) == 0 {
		return httpx.ErrNotFound
	}
	writeRaw(w, raw)
	return nil
}

func (m *mod) publicParts(w http.ResponseWriter, r *http.Request) error {
	pg := httpx.Paging(r, 60)
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
		SELECT id,name,variant,image,sale_price AS price FROM parts
		WHERE deleted_at IS NULL AND is_sellable AND is_active AND sale_price>0
		  AND ($1='' OR name ILIKE '%'||$1||'%')
		  AND ($2='' OR id IN (SELECT part_id FROM part_compat WHERE product_id=(SELECT id FROM products WHERE slug=$2)))
		ORDER BY name LIMIT $3 OFFSET $4) t`, httpx.Q(r, "q"), httpx.Q(r, "product"), pg.Limit, pg.Offset)
	if err != nil {
		return err
	}
	writeRaw(w, raw)
	return nil
}

// ───── lookups for the staff/technician apps (small, unpaged) ─────

func (m *mod) lookupParts(w http.ResponseWriter, r *http.Request) error {
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
		SELECT id,code,name,variant,image,unit,return_required,buyback_price FROM parts
		WHERE deleted_at IS NULL AND is_active AND ($1='' OR name ILIKE '%'||$1||'%' OR code ILIKE '%'||$1||'%' OR part_no ILIKE '%'||$1||'%')
		  AND ($2=0 OR id IN (SELECT part_id FROM part_compat WHERE product_id=$2))
		ORDER BY name LIMIT 100) t`, httpx.Q(r, "q"), qint(r, "product_id"))
	if err != nil {
		return err
	}
	writeRaw(w, raw)
	return nil
}

func (m *mod) lookupServices(w http.ResponseWriter, r *http.Request) error {
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
		SELECT id,name,fee FROM service_items WHERE deleted_at IS NULL AND is_active ORDER BY name) t`)
	if err != nil {
		return err
	}
	writeRaw(w, raw)
	return nil
}

func (m *mod) lookupProducts(w http.ResponseWriter, r *http.Request) error {
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
		SELECT id,name,sku,guarantee_months,golden_months,code_format,code_prefix,warranty_basis FROM products
		WHERE deleted_at IS NULL AND is_active ORDER BY name) t`)
	if err != nil {
		return err
	}
	writeRaw(w, raw)
	return nil
}

func writeRaw(w http.ResponseWriter, raw []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(raw)
}

func qint(r *http.Request, k string) int64 {
	n, _ := httpx.QInt(r, k)
	return n
}

func itoa(n int) string {
	return strings.TrimSpace(strings.Join([]string{intStr(n)}, ""))
}
