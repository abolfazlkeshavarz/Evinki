// Package warranty: customers and guarantees (registration, approval,
// renewal, golden periods) plus the public lookup page.
package warranty

import (
	"context"
	"encoding/json"
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
	"github.com/evinki/platform/internal/platform/sms"
)

type mod struct{ *app.Deps }

func Mount(r chi.Router, d *app.Deps) {
	m := &mod{d}

	// public: anyone with a code can look it up or register it
	r.Get("/public/guarantees/check", httpx.H(m.publicCheck))
	r.Post("/public/guarantees/validate", httpx.H(m.validate))
	r.Post("/public/guarantees/register", httpx.H(m.publicRegister))

	// customer portal
	r.With(auth.Require(auth.PCustomerPortal)).Get("/my/guarantees", httpx.H(m.mine))
	r.With(auth.Require(auth.PCustomerPortal)).Post("/my/guarantees", httpx.H(m.registerMine))

	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PWarrantyRead, auth.PServiceRead, auth.PTechPortal))
		r.Get("/admin/guarantees", httpx.H(m.list))
		r.Get("/admin/guarantees/{id}", httpx.H(m.get))
	})
	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PWarrantyWrite))
		r.Post("/admin/guarantees", httpx.H(m.create))
		r.Patch("/admin/guarantees/{id}", httpx.H(m.patch))
	})
	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PWarrantyApprove))
		r.Post("/admin/guarantees/{id}/approve", httpx.H(m.approve))
		r.Post("/admin/guarantees/{id}/reject", httpx.H(m.reject))
		r.Post("/admin/guarantees/{id}/renew", httpx.H(m.renew))
		r.Post("/admin/guarantees/{id}/cancel", httpx.H(m.cancel))
		r.Post("/admin/guarantees/{id}/golden", httpx.H(m.setGolden))
		r.Delete("/admin/guarantees/{id}/golden", httpx.H(m.removeGolden))
	})
	r.With(auth.Require(auth.PWarrantyDelete)).Delete("/admin/guarantees/{id}", httpx.H(m.remove))

	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PWarrantyRead, auth.PServiceRead))
		r.Get("/admin/customers", httpx.H(m.customers))
		r.Get("/admin/customers/{id}", httpx.H(m.customer))
	})
	r.With(auth.Require(auth.PWarrantyWrite, auth.PServiceManage)).Post("/admin/customers", httpx.H(m.saveCustomer))
	r.With(auth.Require(auth.PWarrantyWrite, auth.PServiceManage)).Patch("/admin/customers/{id}", httpx.H(m.saveCustomer))
}

// ───────── views ─────────

type G struct {
	ID               int64      `db:"id" json:"id"`
	Code             string     `db:"code" json:"code"`
	Status           string     `db:"status" json:"status"`
	Source           string     `db:"source" json:"source"`
	PurchaseDate     time.Time  `db:"purchase_date" json:"purchase_date"`
	ExpiryDate       time.Time  `db:"expiry_date" json:"expiry_date"`
	GoldenStartDate  *time.Time `db:"golden_start_date" json:"golden_start_date"`
	GoldenExpiryDate *time.Time `db:"golden_expiry_date" json:"golden_expiry_date"`
	Notes            string     `db:"notes" json:"notes"`
	InvoiceFile      string     `db:"invoice_file" json:"invoice_file"`
	CardFile         string     `db:"card_file" json:"card_file"`
	ApprovedAt       *time.Time `db:"approved_at" json:"approved_at"`
	CreatedAt        time.Time  `db:"created_at" json:"created_at"`
	CustomerID       int64      `db:"customer_id" json:"customer_id"`
	CustomerName     string     `db:"customer_name" json:"customer_name"`
	CustomerPhone    string     `db:"customer_phone" json:"customer_phone"`
	CustomerCity     string     `db:"customer_city" json:"customer_city"`
	ProductID        int64      `db:"product_id" json:"product_id"`
	ProductName      string     `db:"product_name" json:"product_name"`
	DaysRemaining    int        `db:"days_remaining" json:"days_remaining"`
	Tier             string     `db:"tier" json:"tier"`
}

const gSel = `SELECT g.id,g.code,g.status,g.source,g.purchase_date,g.expiry_date,g.golden_start_date,g.golden_expiry_date,
	g.notes,g.invoice_file,g.card_file,g.approved_at,g.created_at,
	c.id AS customer_id,c.full_name AS customer_name,c.phone AS customer_phone,c.city AS customer_city,
	p.id AS product_id,p.name AS product_name,
	(g.expiry_date - CURRENT_DATE)::int AS days_remaining,
	CASE WHEN g.status NOT IN ('approved','renewed') THEN ''
	     WHEN CURRENT_DATE > g.expiry_date THEN 'expired'
	     WHEN g.golden_expiry_date IS NOT NULL AND CURRENT_DATE <= g.golden_expiry_date
	          AND (g.golden_start_date IS NULL OR CURRENT_DATE >= g.golden_start_date) THEN 'golden'
	     ELSE 'normal' END AS tier
	FROM guarantees g JOIN customers c ON c.id=g.customer_id JOIN products p ON p.id=g.product_id`

func (m *mod) list(w http.ResponseWriter, r *http.Request) error {
	args := []any{httpx.Q(r, "status"), httpx.Q(r, "q")}
	where := ` WHERE g.deleted_at IS NULL AND ($1='' OR g.status=$1)
		AND ($2='' OR g.code ILIKE '%'||$2||'%' OR c.full_name ILIKE '%'||$2||'%' OR c.phone LIKE '%'||$2||'%' OR c.national_id=$2)`
	if v, ok := httpx.QInt(r, "product_id"); ok {
		args = append(args, v)
		where += " AND g.product_id=$3"
	}
	if v, ok := httpx.QInt(r, "expiring_in_days"); ok {
		args = append(args, v)
		where += " AND g.status IN ('approved','renewed') AND g.expiry_date BETWEEN CURRENT_DATE AND CURRENT_DATE + ($" + itoa(len(args)) + " * INTERVAL '1 day')"
	}
	total, err := db.Scalar[int](r.Context(), m.DB, `SELECT COUNT(*) FROM guarantees g JOIN customers c ON c.id=g.customer_id`+where, args...)
	if err != nil {
		return err
	}
	pg := httpx.Paging(r, 200)
	args = append(args, pg.Limit, pg.Offset)
	items, err := db.All[G](r.Context(), m.DB, gSel+where+` ORDER BY g.id DESC LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return err
	}
	httpx.OK(w, httpx.List[G]{Items: items, Total: total, Page: pg.Page, Limit: pg.Limit})
	return nil
}

func (m *mod) get(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	g, err := db.One[G](r.Context(), m.DB, gSel+` WHERE g.id=$1 AND g.deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	repairs, _ := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
		SELECT id,number,status,complaint,created_at,guarantee_was_expired FROM repairs WHERE guarantee_id=$1 AND deleted_at IS NULL ORDER BY id DESC) t`, id)
	httpx.OK(w, map[string]any{"guarantee": g, "repairs": repairs})
	return nil
}

// ───────── public ─────────

type rules struct {
	ID      int64  `db:"id"`
	Name    string `db:"name"`
	Months  int    `db:"guarantee_months"`
	Golden  int    `db:"golden_months"`
	Format  string `db:"code_format"`
	Prefix  string `db:"code_prefix"`
	Pattern string `db:"code_pattern"`
}

func productRules(ctx context.Context, q db.Q, id int64) (rules, error) {
	return db.One[rules](ctx, q, `SELECT id,name,guarantee_months,golden_months,code_format,code_prefix,code_pattern FROM products WHERE id=$1 AND deleted_at IS NULL AND is_active`, id)
}

func (m *mod) validate(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		ProductID int64  `json:"product_id"`
		Code      string `json:"code"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	p, err := productRules(r.Context(), m.DB, in.ProductID)
	if err != nil {
		return httpx.BadRequest("محصول پیدا نشد")
	}
	res := CheckCode(ProductRules{p.Format, p.Prefix, p.Pattern}, httpx.Digits(in.Code))
	if res.Valid {
		if n, _ := db.Scalar[int](r.Context(), m.DB, `SELECT COUNT(*) FROM guarantees WHERE code=$1 AND deleted_at IS NULL AND status IN ('pending','approved','renewed','expired')`, strings.TrimSpace(httpx.Digits(in.Code))); n > 0 {
			res.CanRegister, res.Message, res.MessageLevel = false, "این کد قبلاً ثبت شده است", "error"
		}
	}
	httpx.OK(w, res)
	return nil
}

// publicCheck shows status without exposing the owner's details.
func (m *mod) publicCheck(w http.ResponseWriter, r *http.Request) error {
	code := strings.TrimSpace(httpx.Digits(httpx.Q(r, "code")))
	if code == "" {
		return httpx.BadRequest("کد گارانتی را وارد کنید")
	}
	g, err := db.One[G](r.Context(), m.DB, gSel+` WHERE g.code=$1 AND g.deleted_at IS NULL AND g.status IN ('pending','approved','renewed','expired') ORDER BY g.id DESC LIMIT 1`, code)
	if err == db.ErrNotFound {
		httpx.OK(w, map[string]any{"found": false})
		return nil
	}
	if err != nil {
		return err
	}
	httpx.OK(w, map[string]any{"found": true, "code": g.Code, "status": g.Status, "tier": g.Tier,
		"product_name": g.ProductName, "purchase_date": g.PurchaseDate, "expiry_date": g.ExpiryDate,
		"golden_expiry_date": g.GoldenExpiryDate, "days_remaining": g.DaysRemaining,
		"owner": maskName(g.CustomerName)})
	return nil
}

func maskName(n string) string {
	parts := strings.Fields(n)
	for i, p := range parts {
		rs := []rune(p)
		if len(rs) > 1 {
			parts[i] = string(rs[0]) + strings.Repeat("•", len(rs)-1)
		}
	}
	return strings.Join(parts, " ")
}

type regIn struct {
	Code         string `json:"code"`
	ProductID    int64  `json:"product_id"`
	PurchaseDate string `json:"purchase_date"`
	FullName     string `json:"full_name"`
	Phone        string `json:"phone"`
	NationalID   string `json:"national_id"`
	Province     string `json:"province"`
	City         string `json:"city"`
	Address      string `json:"address"`
	InvoiceFile  string `json:"invoice_file"`
	CardFile     string `json:"card_file"`
}

func (m *mod) publicRegister(w http.ResponseWriter, r *http.Request) error {
	var in regIn
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	return m.register(w, r, in, nil, "web")
}

func (m *mod) registerMine(w http.ResponseWriter, r *http.Request) error {
	var in regIn
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	uid := auth.From(r.Context()).UserID
	return m.register(w, r, in, &uid, "app")
}

func (m *mod) register(w http.ResponseWriter, r *http.Request, in regIn, uid *int64, source string) error {
	ctx := r.Context()
	in.Code = strings.TrimSpace(httpx.Digits(in.Code))
	in.Phone = httpx.Phone(in.Phone)
	if strings.TrimSpace(in.FullName) == "" || !httpx.ValidMobile(in.Phone) {
		return httpx.BadRequest("نام و شماره موبایل معتبر لازم است")
	}
	// Both photos are mandatory when a customer registers: the office checks them before approving.
	if !strings.HasPrefix(in.InvoiceFile, "/api/v1/files/") {
		return httpx.BadRequest("بارگذاری عکس فاکتور خرید الزامی است")
	}
	if !strings.HasPrefix(in.CardFile, "/api/v1/files/") {
		return httpx.BadRequest("بارگذاری عکس کارت گارانتی الزامی است")
	}
	p, err := productRules(ctx, m.DB, in.ProductID)
	if err != nil {
		return httpx.BadRequest("محصول پیدا نشد")
	}
	chk := CheckCode(ProductRules{p.Format, p.Prefix, p.Pattern}, in.Code)
	if !chk.Valid || !chk.CanRegister {
		return httpx.BadRequest(chk.Message)
	}
	purchase, err := parseDate(in.PurchaseDate)
	if err != nil || purchase.After(time.Now().AddDate(0, 0, 1)) {
		return httpx.BadRequest("تاریخ خرید نامعتبر است")
	}
	var id int64
	err = m.DB.Tx(ctx, func(tx pgx.Tx) error {
		cid, e := upsertCustomer(ctx, tx, uid, in.FullName, in.Phone, in.NationalID, in.Province, in.City, in.Address)
		if e != nil {
			return e
		}
		g := computePeriods(purchase, p.Months, p.Golden)
		id, e = db.Scalar[int64](ctx, tx, `INSERT INTO guarantees(code,customer_id,product_id,purchase_date,expiry_date,golden_start_date,golden_expiry_date,status,source,invoice_file,card_file,created_by)
			VALUES($1,$2,$3,$4,$5,$6,$7,'pending',$8,$9,$10,$11) RETURNING id`,
			in.Code, cid, p.ID, g.purchase, g.expiry, g.goldenStart, g.goldenEnd, source, in.InvoiceFile, in.CardFile, uid)
		return e
	})
	if db.IsUnique(err) {
		return httpx.Conflict("این کد گارانتی قبلاً ثبت شده است")
	}
	if err != nil {
		return err
	}
	httpx.Created(w, map[string]any{"id": id, "status": "pending", "message": "گارانتی ثبت شد و پس از بررسی فعال می‌شود"})
	return nil
}

func (m *mod) mine(w http.ResponseWriter, r *http.Request) error {
	items, err := db.All[G](r.Context(), m.DB, gSel+` WHERE g.deleted_at IS NULL AND (c.user_id=$1 OR c.phone=(SELECT phone FROM users WHERE id=$1 AND phone<>'')) ORDER BY g.id DESC`, auth.From(r.Context()).UserID)
	if err != nil {
		return err
	}
	httpx.OK(w, items)
	return nil
}

// ───────── periods ─────────

type periods struct {
	purchase, expiry         time.Time
	goldenStart, goldenEnd *time.Time
}

// computePeriods: expiry = purchase + N months; the golden window runs from purchase.
func computePeriods(purchase time.Time, months, golden int) periods {
	p := periods{purchase: purchase, expiry: purchase.AddDate(0, months, 0)}
	if golden > 0 {
		s, e := purchase, purchase.AddDate(0, golden, 0)
		p.goldenStart, p.goldenEnd = &s, &e
	}
	return p
}

func parseDate(s string) (time.Time, error) {
	s = httpx.Digits(strings.TrimSpace(s))
	if len(s) > 10 {
		s = s[:10]
	}
	return time.ParseInLocation("2006-01-02", s, time.UTC)
}

func upsertCustomer(ctx context.Context, q db.Q, uid *int64, name, phone, nid, province, city, address string) (int64, error) {
	if uid != nil {
		if id, err := db.Scalar[int64](ctx, q, `SELECT id FROM customers WHERE user_id=$1 AND deleted_at IS NULL ORDER BY id LIMIT 1`, *uid); err == nil {
			return id, nil
		}
	}
	if phone != "" {
		if id, err := db.Scalar[int64](ctx, q, `UPDATE customers SET user_id=COALESCE(user_id,$2),
			national_id=COALESCE(NULLIF($3,''),national_id), city=COALESCE(NULLIF($4,''),city), province=COALESCE(NULLIF($5,''),province),
			address=COALESCE(NULLIF($6,''),address), updated_at=NOW()
			WHERE id=(SELECT id FROM customers WHERE phone=$1 AND deleted_at IS NULL ORDER BY id LIMIT 1) RETURNING id`,
			phone, uid, nid, city, province, address); err == nil {
			return id, nil
		}
	}
	return db.Scalar[int64](ctx, q, `INSERT INTO customers(user_id,full_name,phone,national_id,province,city,address) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		uid, strings.TrimSpace(name), phone, nid, province, city, address)
}

// EnsureCustomer is used by other modules (repairs for walk-in customers).
func EnsureCustomer(ctx context.Context, q db.Q, name, phone, nid, province, city, address string) (int64, error) {
	return upsertCustomer(ctx, q, nil, name, httpx.Phone(phone), nid, province, city, address)
}

// IssueForOrder creates approved guarantees for delivered order lines that
// carry a serial number, so a customer who bought online never has to register.
func IssueForOrder(ctx context.Context, q db.Q, orderID int64) error {
	type line struct {
		ItemID int64  `db:"id"`
		PID    int64  `db:"product_id"`
		Serial string `db:"serial_code"`
		Months int    `db:"guarantee_months"`
		Golden int    `db:"golden_months"`
		UID    int64  `db:"user_id"`
		Name   string `db:"full_name"`
		Phone  string `db:"phone"`
		Addr   string `db:"addr"`
		City   string `db:"city"`
		Prov   string `db:"province"`
	}
	lines, err := db.All[line](ctx, q, `SELECT oi.id,oi.product_id,oi.serial_code,p.guarantee_months,p.golden_months,o.user_id,
			COALESCE(NULLIF(o.address->>'recipient',''),u.full_name) AS full_name, COALESCE(NULLIF(u.phone,''),o.address->>'phone') AS phone,
			COALESCE(o.address->>'address','') AS addr, COALESCE(o.address->>'city','') AS city, COALESCE(o.address->>'province','') AS province
		FROM order_items oi JOIN orders o ON o.id=oi.order_id JOIN users u ON u.id=o.user_id JOIN products p ON p.id=oi.product_id
		WHERE oi.order_id=$1 AND oi.serial_code<>'' AND p.guarantee_months>0`, orderID)
	if err != nil {
		return err
	}
	today := time.Date(jalali.Now().Year(), jalali.Now().Month(), jalali.Now().Day(), 0, 0, 0, 0, time.UTC)
	for _, l := range lines {
		uid := l.UID
		cid, err := upsertCustomer(ctx, q, &uid, l.Name, httpx.Phone(l.Phone), "", l.Prov, l.City, l.Addr)
		if err != nil {
			return err
		}
		g := computePeriods(today, l.Months, l.Golden)
		if _, err = q.Exec(ctx, `INSERT INTO guarantees(code,customer_id,product_id,purchase_date,expiry_date,golden_start_date,golden_expiry_date,status,source,order_item_id,approved_at)
			SELECT $1,$2,$3,$4,$5,$6,$7,'approved','order',$8,NOW()
			WHERE NOT EXISTS (SELECT 1 FROM guarantees WHERE code=$1 AND deleted_at IS NULL AND status IN ('pending','approved','renewed','expired'))`,
			l.Serial, cid, l.PID, g.purchase, g.expiry, g.goldenStart, g.goldenEnd, l.ItemID); err != nil {
			return err
		}
	}
	return nil
}

// ───────── admin writes ─────────

func (m *mod) create(w http.ResponseWriter, r *http.Request) error {
	var in regIn
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	in.Code = strings.TrimSpace(httpx.Digits(in.Code))
	in.Phone = httpx.Phone(in.Phone)
	if strings.TrimSpace(in.FullName) == "" || in.Code == "" {
		return httpx.BadRequest("نام مشتری و کد گارانتی لازم است")
	}
	p, err := productRules(ctx, m.DB, in.ProductID)
	if err != nil {
		return httpx.BadRequest("محصول پیدا نشد")
	}
	purchase, err := parseDate(in.PurchaseDate)
	if err != nil {
		return httpx.BadRequest("تاریخ خرید نامعتبر است")
	}
	by := auth.From(ctx).UserID
	var id int64
	err = m.DB.Tx(ctx, func(tx pgx.Tx) error {
		cid, e := upsertCustomer(ctx, tx, nil, in.FullName, in.Phone, in.NationalID, in.Province, in.City, in.Address)
		if e != nil {
			return e
		}
		g := computePeriods(purchase, p.Months, p.Golden)
		id, e = db.Scalar[int64](ctx, tx, `INSERT INTO guarantees(code,customer_id,product_id,purchase_date,expiry_date,golden_start_date,golden_expiry_date,
			status,source,invoice_file,card_file,created_by,approved_by,approved_at) VALUES($1,$2,$3,$4,$5,$6,$7,'approved','admin',$8,$9,$10,$10,NOW()) RETURNING id`,
			in.Code, cid, p.ID, g.purchase, g.expiry, g.goldenStart, g.goldenEnd, in.InvoiceFile, in.CardFile, by)
		return e
	})
	if db.IsUnique(err) {
		return httpx.Conflict("این کد گارانتی قبلاً ثبت شده است")
	}
	if err != nil {
		return err
	}
	m.Audit.Log(r, "guarantee.create", "guarantee", id, in.Code)
	g, _ := db.One[G](ctx, m.DB, gSel+` WHERE g.id=$1`, id)
	httpx.Created(w, g)
	return nil
}

func (m *mod) patch(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Notes        *string `json:"notes"`
		PurchaseDate *string `json:"purchase_date"`
		ExpiryDate   *string `json:"expiry_date"`
		InvoiceFile  *string `json:"invoice_file"`
		CardFile     *string `json:"card_file"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	_, err = m.DB.Exec(r.Context(), `UPDATE guarantees SET notes=COALESCE($2,notes), invoice_file=COALESCE($3,invoice_file), card_file=COALESCE($6,card_file),
		purchase_date=COALESCE($4::date,purchase_date), expiry_date=COALESCE($5::date,expiry_date), updated_at=NOW()
		WHERE id=$1 AND deleted_at IS NULL`, id, in.Notes, in.InvoiceFile, digitsPtr(in.PurchaseDate), digitsPtr(in.ExpiryDate), in.CardFile)
	if err != nil {
		return err
	}
	m.Audit.Log(r, "guarantee.update", "guarantee", id, "")
	g, err := db.One[G](r.Context(), m.DB, gSel+` WHERE g.id=$1`, id)
	if err != nil {
		return err
	}
	httpx.OK(w, g)
	return nil
}

func digitsPtr(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	v := httpx.Digits(*s)
	return &v
}

func (m *mod) transition(w http.ResponseWriter, r *http.Request, from []string, to, action string, extra func(ctx context.Context, tx pgx.Tx, id int64) error) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	ctx := r.Context()
	by := auth.From(ctx).UserID
	err = m.DB.Tx(ctx, func(tx pgx.Tx) error {
		cur, e := db.Scalar[string](ctx, tx, `SELECT status FROM guarantees WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, id)
		if e != nil {
			return e
		}
		ok := false
		for _, f := range from {
			ok = ok || f == cur
		}
		if !ok {
			return httpx.Conflict("وضعیت فعلی گارانتی اجازه این کار را نمی‌دهد")
		}
		if _, e = tx.Exec(ctx, `UPDATE guarantees SET status=$2, approved_by=CASE WHEN $2='approved' THEN $3 ELSE approved_by END,
			approved_at=CASE WHEN $2='approved' THEN NOW() ELSE approved_at END, updated_at=NOW() WHERE id=$1`, id, to, by); e != nil {
			return e
		}
		if extra != nil {
			return extra(ctx, tx, id)
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "guarantee."+action, "guarantee", id, "")
	g, err := db.One[G](ctx, m.DB, gSel+` WHERE g.id=$1`, id)
	if err != nil {
		return err
	}
	httpx.OK(w, g)
	return nil
}

func (m *mod) notifyCustomer(ctx context.Context, tx pgx.Tx, id int64, tpl string) error {
	g, err := db.One[G](ctx, tx, gSel+` WHERE g.id=$1`, id)
	if err != nil {
		return err
	}
	return sms.Enqueue(ctx, tx, g.CustomerPhone, tpl, g.CustomerName, g.Code, jalali.Format(g.ExpiryDate))
}

func (m *mod) approve(w http.ResponseWriter, r *http.Request) error {
	return m.transition(w, r, []string{"pending"}, "approved", "approve", func(ctx context.Context, tx pgx.Tx, id int64) error {
		return m.notifyCustomer(ctx, tx, id, "guarantee_approved")
	})
}
func (m *mod) reject(w http.ResponseWriter, r *http.Request) error {
	return m.transition(w, r, []string{"pending"}, "rejected", "reject", nil)
}
func (m *mod) cancel(w http.ResponseWriter, r *http.Request) error {
	return m.transition(w, r, []string{"pending", "approved", "renewed"}, "cancelled", "cancel", nil)
}

func (m *mod) renew(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		NewExpiryDate string `json:"new_expiry_date"`
		Notes         string `json:"notes"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	exp, err := parseDate(in.NewExpiryDate)
	if err != nil {
		return httpx.BadRequest("تاریخ انقضای جدید نامعتبر است")
	}
	return m.transition(w, r, []string{"approved", "renewed", "expired"}, "renewed", "renew", func(ctx context.Context, tx pgx.Tx, id int64) error {
		tag, e := tx.Exec(ctx, `UPDATE guarantees SET expiry_date=$2, notes=CASE WHEN $3<>'' THEN $3 ELSE notes END WHERE id=$1 AND $2 > expiry_date`, id, exp, in.Notes)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			return httpx.BadRequest("تاریخ جدید باید بعد از تاریخ انقضای فعلی باشد")
		}
		return m.notifyCustomer(ctx, tx, id, "guarantee_renewed")
	})
}

func (m *mod) setGolden(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		StartDate    string `json:"start_date"`
		GoldenMonths int    `json:"golden_months"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	start, err := parseDate(in.StartDate)
	if err != nil || in.GoldenMonths <= 0 {
		return httpx.BadRequest("تاریخ شروع و تعداد ماه را درست وارد کنید")
	}
	end := start.AddDate(0, in.GoldenMonths, 0)
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	tag, err := m.DB.Exec(r.Context(), `UPDATE guarantees SET golden_start_date=$2, golden_expiry_date=$3, updated_at=NOW()
		WHERE id=$1 AND deleted_at IS NULL AND status IN ('approved','renewed') AND $3 <= expiry_date`, id, start, end)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.BadRequest("فقط گارانتی تاییدشده و پایان طلایی نباید از انقضای کل بیشتر باشد")
	}
	m.Audit.Log(r, "guarantee.golden", "guarantee", id, "")
	g, err := db.One[G](r.Context(), m.DB, gSel+` WHERE g.id=$1`, id)
	if err != nil {
		return err
	}
	httpx.OK(w, g)
	return nil
}

func (m *mod) removeGolden(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	_, err = m.DB.Exec(r.Context(), `UPDATE guarantees SET golden_start_date=NULL, golden_expiry_date=NULL, updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	m.Audit.Log(r, "guarantee.golden_remove", "guarantee", id, "")
	httpx.NoContent(w)
	return nil
}

func (m *mod) remove(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	tag, err := m.DB.Exec(r.Context(), `UPDATE guarantees SET deleted_at=NOW() WHERE id=$1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.ErrNotFound
	}
	m.Audit.Log(r, "guarantee.delete", "guarantee", id, "")
	httpx.NoContent(w)
	return nil
}

// ───────── customers ─────────

func (m *mod) customers(w http.ResponseWriter, r *http.Request) error {
	q := httpx.Q(r, "q")
	pg := httpx.Paging(r, 200)
	where := ` WHERE c.deleted_at IS NULL AND ($1='' OR c.full_name ILIKE '%'||$1||'%' OR c.phone LIKE '%'||$1||'%' OR c.national_id=$1)`
	total, err := db.Scalar[int](r.Context(), m.DB, `SELECT COUNT(*) FROM customers c`+where, q)
	if err != nil {
		return err
	}
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
		SELECT c.id,c.full_name,c.phone,c.national_id,c.province,c.city,c.address,c.created_at,
		  (SELECT COUNT(*) FROM guarantees g WHERE g.customer_id=c.id AND g.deleted_at IS NULL)::int AS guarantee_count
		FROM customers c`+where+` ORDER BY c.id DESC LIMIT $2 OFFSET $3) t`, q, pg.Limit, pg.Offset)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"items":` + string(raw) + `,"total":` + itoa(total) + `,"page":` + itoa(pg.Page) + `,"limit":` + itoa(pg.Limit) + `}`))
	return nil
}

func (m *mod) customer(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	return m.customerJSON(w, r, id)
}

func (m *mod) customerJSON(w http.ResponseWriter, r *http.Request, id int64) error {
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT row_to_json(t) FROM (
		SELECT c.id,c.full_name,c.phone,c.national_id,c.province,c.city,c.address,c.created_at,
		 COALESCE((SELECT json_agg(x) FROM (SELECT g.id,g.code,g.status,g.expiry_date,p.name AS product_name FROM guarantees g JOIN products p ON p.id=g.product_id
		   WHERE g.customer_id=c.id AND g.deleted_at IS NULL ORDER BY g.id DESC) x),'[]') AS guarantees
		FROM customers c WHERE c.id=$1 AND c.deleted_at IS NULL) t`, id)
	if err != nil || len(raw) == 0 {
		return httpx.ErrNotFound
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
	return nil
}

func (m *mod) saveCustomer(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		FullName   *string `json:"full_name"`
		Phone      *string `json:"phone"`
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
	var id int64
	var err error
	if chi.URLParam(r, "id") == "" {
		if in.FullName == nil || strings.TrimSpace(*in.FullName) == "" {
			return httpx.BadRequest("نام مشتری لازم است")
		}
		id, err = db.Scalar[int64](r.Context(), m.DB, `INSERT INTO customers(full_name,phone,national_id,province,city,address)
			VALUES($1,COALESCE($2,''),COALESCE($3,''),COALESCE($4,''),COALESCE($5,''),COALESCE($6,'')) RETURNING id`,
			*in.FullName, phone, in.NationalID, in.Province, in.City, in.Address)
	} else {
		if id, err = httpx.ID(r, "id"); err != nil {
			return err
		}
		_, err = m.DB.Exec(r.Context(), `UPDATE customers SET full_name=COALESCE($2,full_name),phone=COALESCE($3,phone),national_id=COALESCE($4,national_id),
			province=COALESCE($5,province),city=COALESCE($6,city),address=COALESCE($7,address),updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL`,
			id, in.FullName, phone, in.NationalID, in.Province, in.City, in.Address)
	}
	if err != nil {
		return err
	}
	m.Audit.Log(r, "customer.save", "customer", id, "")
	return m.customerJSON(w, r, id)
}
