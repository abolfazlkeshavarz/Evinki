// Package shop: cart, checkout, online payment, orders, coupons, addresses.
package shop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/evinki/platform/internal/app"
	"github.com/evinki/platform/internal/modules/surveys"
	"github.com/evinki/platform/internal/modules/warranty"
	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/crud"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
	"github.com/evinki/platform/internal/platform/settings"
	"github.com/evinki/platform/internal/platform/sms"
)

type mod struct{ *app.Deps }

func Mount(r chi.Router, d *app.Deps) {
	m := &mod{d}

	// Cart works for guests (X-Cart-Token) and signed-in users.
	r.Get("/shop/cart", httpx.H(m.getCart))
	r.Put("/shop/cart/items", httpx.H(m.setItem))
	r.Delete("/shop/cart", httpx.H(m.clearCart))
	r.Post("/shop/cart/coupon", httpx.H(m.applyCoupon))
	r.Get("/shop/settings", httpx.H(m.publicSettings))
	// The gateway sends the customer's browser here; no bearer token is available.
	r.Get("/shop/payment/callback", httpx.H(m.callback))

	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth)
		r.Post("/shop/checkout", httpx.H(m.checkout))
		r.Get("/shop/orders", httpx.H(m.myOrders))
		r.Get("/shop/orders/{id}", httpx.H(m.myOrder))
		r.Post("/shop/orders/{id}/pay", httpx.H(m.repay))
		r.Post("/shop/orders/{id}/cancel", httpx.H(m.cancelMine))

		r.Get("/shop/addresses", httpx.H(m.listAddresses))
		r.Post("/shop/addresses", httpx.H(m.saveAddress))
		r.Put("/shop/addresses/{id}", httpx.H(m.saveAddress))
		r.Delete("/shop/addresses/{id}", httpx.H(m.deleteAddress))

		r.Get("/shop/wishlist", httpx.H(m.wishlist))
		r.Put("/shop/wishlist/{pid}", httpx.H(m.wishAdd))
		r.Delete("/shop/wishlist/{pid}", httpx.H(m.wishRemove))
	})

	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PShopRead, auth.PShopManage))
		r.Get("/admin/orders", httpx.H(m.adminOrders))
		r.Get("/admin/orders/{id}", httpx.H(m.adminOrder))
	})
	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PShopManage))
		r.Post("/admin/orders/{id}/status", httpx.H(m.setStatus))
		r.Put("/admin/orders/{id}/items/{item}/serial", httpx.H(m.setSerial))
	})

	coupons := &crud.Resource{Name: "coupon", Table: "coupons", DB: d.DB, Audit: d.Audit,
		Cols: map[string]crud.Kind{"code": crud.Text, "kind": crud.Text, "value": crud.Int, "max_discount": crud.Int,
			"min_total": crud.Int, "max_uses": crud.Int, "starts_at": crud.TS, "ends_at": crud.TS, "is_active": crud.Bool},
		Required: []string{"code", "kind", "value"}, Search: []string{"code"}, Order: "created_at DESC",
		Check: func(_ *http.Request, create bool, b crud.Body) error {
			if raw, ok := b["code"]; ok {
				var s string
				_ = json.Unmarshal(raw, &s)
				b["code"], _ = json.Marshal(strings.ToUpper(strings.TrimSpace(s)))
			}
			return nil
		}}
	// coupons are keyed by code, not id: manage them with the bespoke handlers below
	_ = coupons
	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PShopManage))
		r.Get("/admin/coupons", httpx.H(m.listCoupons))
		r.Post("/admin/coupons", httpx.H(m.saveCoupon))
		r.Delete("/admin/coupons/{code}", httpx.H(m.deleteCoupon))
	})
}

// ───────── cart ─────────

type cartLine struct {
	ID        int64  `db:"id" json:"id"`
	ProductID *int64 `db:"product_id" json:"product_id"`
	PartID    *int64 `db:"part_id" json:"part_id"`
	Name      string `db:"name" json:"name"`
	Slug      string `db:"slug" json:"slug"`
	Image     string `db:"image" json:"image"`
	UnitPrice int64  `db:"unit_price" json:"unit_price"`
	Qty       int    `db:"qty" json:"qty"`
	Total     int64  `db:"total" json:"total"`
	Available int    `db:"available" json:"available"` // 9999 when stock is not tracked
	Orderable bool   `db:"orderable" json:"orderable"`
}

type cartOut struct {
	Lines      []cartLine `json:"lines"`
	Subtotal   int64      `json:"subtotal"`
	Discount   int64      `json:"discount"`
	Shipping   int64      `json:"shipping_fee"`
	Total      int64      `json:"total"`
	Coupon     string     `json:"coupon"`
	CouponNote string     `json:"coupon_note,omitempty"`
	Count      int        `json:"count"`
}

// cartID finds (or lazily creates) the caller's cart and merges a guest cart
// into the user's right after sign-in.
func (m *mod) cartID(r *http.Request, create bool) (int64, error) {
	ctx := r.Context()
	guest := strings.TrimSpace(r.Header.Get("X-Cart-Token"))
	if p := auth.From(ctx); p != nil {
		id, err := db.Scalar[int64](ctx, m.DB, `SELECT id FROM carts WHERE user_id=$1`, p.UserID)
		if err == db.ErrNotFound {
			id, err = db.Scalar[int64](ctx, m.DB, `INSERT INTO carts(user_id) VALUES($1) ON CONFLICT (user_id) WHERE user_id IS NOT NULL DO UPDATE SET updated_at=NOW() RETURNING id`, p.UserID)
		}
		if err != nil {
			return 0, err
		}
		if guest != "" {
			if gid, gerr := db.Scalar[int64](ctx, m.DB, `SELECT id FROM carts WHERE guest_token=$1 AND user_id IS NULL`, guest); gerr == nil {
				_ = m.DB.Tx(ctx, func(tx pgx.Tx) error {
					_, e := tx.Exec(ctx, `INSERT INTO cart_items(cart_id,product_id,part_id,qty)
						SELECT $1,product_id,part_id,qty FROM cart_items WHERE cart_id=$2
						ON CONFLICT DO NOTHING`, id, gid)
					if e != nil {
						return e
					}
					_, e = tx.Exec(ctx, `DELETE FROM carts WHERE id=$1`, gid)
					return e
				})
			}
		}
		return id, nil
	}
	if guest == "" || len(guest) < 16 {
		return 0, httpx.BadRequest("شناسه سبد خرید (X-Cart-Token) لازم است")
	}
	id, err := db.Scalar[int64](ctx, m.DB, `SELECT id FROM carts WHERE guest_token=$1`, guest)
	if err == db.ErrNotFound && create {
		return db.Scalar[int64](ctx, m.DB, `INSERT INTO carts(guest_token) VALUES($1) RETURNING id`, guest)
	}
	return id, err
}

const lineSQL = `SELECT ci.id, ci.product_id, ci.part_id,
	COALESCE(p.name, pt.name) AS name, COALESCE(p.slug,'') AS slug,
	COALESCE((p.images->>0), pt.image, '') AS image,
	COALESCE(p.price, pt.sale_price) AS unit_price, ci.qty,
	COALESCE(p.price, pt.sale_price)*ci.qty AS total,
	CASE WHEN p.id IS NOT NULL AND p.track_stock THEN p.stock_qty ELSE 9999 END AS available,
	CASE WHEN p.id IS NOT NULL THEN (p.is_active AND p.is_published AND p.deleted_at IS NULL AND NOT p.price_on_request AND p.price>0)
	     ELSE (pt.is_active AND pt.is_sellable AND pt.deleted_at IS NULL AND pt.sale_price>0) END AS orderable
	FROM cart_items ci
	LEFT JOIN products p ON p.id=ci.product_id
	LEFT JOIN parts pt ON pt.id=ci.part_id
	WHERE ci.cart_id=$1 ORDER BY ci.id`

// price computes totals for a cart. Pure read; checkout re-runs it inside its transaction.
func (m *mod) price(ctx context.Context, q db.Q, cartID int64) (*cartOut, error) {
	lines, err := db.All[cartLine](ctx, q, lineSQL, cartID)
	if err != nil {
		return nil, err
	}
	out := &cartOut{Lines: lines}
	for _, l := range lines {
		out.Subtotal += l.Total
		out.Count += l.Qty
	}
	code, _ := db.Scalar[string](ctx, q, `SELECT coupon_code FROM carts WHERE id=$1`, cartID)
	if code != "" {
		d, note := couponDiscount(ctx, q, code, out.Subtotal)
		out.Coupon, out.Discount, out.CouponNote = code, d, note
		if note != "" {
			out.Coupon, out.Discount = "", 0
		}
	}
	if out.Subtotal > 0 {
		fee := settings.Int(ctx, q, "shipping_fee")
		free := settings.Int(ctx, q, "free_shipping_over")
		if free > 0 && out.Subtotal-out.Discount >= free {
			fee = 0
		}
		out.Shipping = fee
	}
	out.Total = out.Subtotal - out.Discount + out.Shipping
	return out, nil
}

func couponDiscount(ctx context.Context, q db.Q, code string, subtotal int64) (int64, string) {
	type c struct {
		Kind     string     `db:"kind"`
		Value    int64      `db:"value"`
		MaxDisc  int64      `db:"max_discount"`
		MinTotal int64      `db:"min_total"`
		MaxUses  int        `db:"max_uses"`
		Used     int        `db:"used_count"`
		Starts   *time.Time `db:"starts_at"`
		Ends     *time.Time `db:"ends_at"`
		Active   bool       `db:"is_active"`
	}
	v, err := db.One[c](ctx, q, `SELECT kind,value,max_discount,min_total,max_uses,used_count,starts_at,ends_at,is_active FROM coupons WHERE code=$1`, strings.ToUpper(code))
	now := time.Now()
	switch {
	case err != nil || !v.Active:
		return 0, "کد تخفیف معتبر نیست"
	case v.Starts != nil && now.Before(*v.Starts), v.Ends != nil && now.After(*v.Ends):
		return 0, "کد تخفیف منقضی شده یا هنوز فعال نشده است"
	case v.MaxUses > 0 && v.Used >= v.MaxUses:
		return 0, "ظرفیت این کد تخفیف تمام شده است"
	case subtotal < v.MinTotal:
		return 0, fmt.Sprintf("حداقل مبلغ خرید برای این کد %d است", v.MinTotal)
	}
	d := v.Value
	if v.Kind == "percent" {
		d = subtotal * v.Value / 100
		if v.MaxDisc > 0 && d > v.MaxDisc {
			d = v.MaxDisc
		}
	}
	if d > subtotal {
		d = subtotal
	}
	return d, ""
}

func (m *mod) getCart(w http.ResponseWriter, r *http.Request) error {
	id, err := m.cartID(r, false)
	if err == db.ErrNotFound {
		httpx.OK(w, &cartOut{Lines: []cartLine{}})
		return nil
	}
	if err != nil {
		return err
	}
	out, err := m.price(r.Context(), m.DB, id)
	if err != nil {
		return err
	}
	httpx.OK(w, out)
	return nil
}

func (m *mod) setItem(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		ProductID *int64 `json:"product_id"`
		PartID    *int64 `json:"part_id"`
		Qty       int    `json:"qty"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if (in.ProductID == nil) == (in.PartID == nil) {
		return httpx.BadRequest("یکی از product_id یا part_id لازم است")
	}
	if in.Qty < 0 || in.Qty > 99 {
		return httpx.BadRequest("تعداد نامعتبر است")
	}
	ctx := r.Context()
	cid, err := m.cartID(r, true)
	if err != nil {
		return err
	}
	if in.Qty == 0 {
		_, err = m.DB.Exec(ctx, `DELETE FROM cart_items WHERE cart_id=$1 AND product_id IS NOT DISTINCT FROM $2 AND part_id IS NOT DISTINCT FROM $3`, cid, in.ProductID, in.PartID)
	} else {
		// Refuse items that cannot be bought online (price on request, hidden, unsellable).
		var ok bool
		if in.ProductID != nil {
			ok, err = db.Scalar[bool](ctx, m.DB, `SELECT is_active AND is_published AND deleted_at IS NULL AND NOT price_on_request AND price>0 FROM products WHERE id=$1`, *in.ProductID)
		} else {
			ok, err = db.Scalar[bool](ctx, m.DB, `SELECT is_active AND is_sellable AND deleted_at IS NULL AND sale_price>0 FROM parts WHERE id=$1`, *in.PartID)
		}
		if err != nil || !ok {
			return httpx.BadRequest("این کالا قابل سفارش آنلاین نیست؛ برای قیمت تماس بگیرید")
		}
		cond := "product_id"
		id := in.ProductID
		if in.ProductID == nil {
			cond, id = "part_id", in.PartID
		}
		_, err = m.DB.Exec(ctx, fmt.Sprintf(`INSERT INTO cart_items(cart_id,%s,qty) VALUES($1,$2,$3)
			ON CONFLICT (cart_id,%s) WHERE %s IS NOT NULL DO UPDATE SET qty=EXCLUDED.qty`, cond, cond, cond), cid, *id, in.Qty)
	}
	if err != nil {
		return err
	}
	_, _ = m.DB.Exec(ctx, `UPDATE carts SET updated_at=NOW() WHERE id=$1`, cid)
	return m.getCart(w, r)
}

func (m *mod) clearCart(w http.ResponseWriter, r *http.Request) error {
	cid, err := m.cartID(r, false)
	if err == nil {
		_, err = m.DB.Exec(r.Context(), `DELETE FROM cart_items WHERE cart_id=$1`, cid)
		_, _ = m.DB.Exec(r.Context(), `UPDATE carts SET coupon_code='' WHERE id=$1`, cid)
	}
	if err != nil && err != db.ErrNotFound {
		return err
	}
	httpx.NoContent(w)
	return nil
}

func (m *mod) applyCoupon(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Code string `json:"code"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	cid, err := m.cartID(r, true)
	if err != nil {
		return err
	}
	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if code != "" {
		out, err := m.price(r.Context(), m.DB, cid)
		if err != nil {
			return err
		}
		if _, note := couponDiscount(r.Context(), m.DB, code, out.Subtotal); note != "" {
			return httpx.BadRequest(note)
		}
	}
	if _, err = m.DB.Exec(r.Context(), `UPDATE carts SET coupon_code=$2 WHERE id=$1`, cid, code); err != nil {
		return err
	}
	return m.getCart(w, r)
}

func (m *mod) publicSettings(w http.ResponseWriter, r *http.Request) error {
	s, err := settings.All(r.Context(), m.DB)
	if err != nil {
		return err
	}
	httpx.OK(w, map[string]any{
		"company_name": s["company_name"], "company_phone": s["company_phone"], "currency_label": s["currency_label"],
		"shipping_fee": s["shipping_fee"], "free_shipping_over": s["free_shipping_over"],
	})
	return nil
}

// ───────── addresses ─────────

type address struct {
	ID         int64  `db:"id" json:"id"`
	Title      string `db:"title" json:"title"`
	Recipient  string `db:"recipient" json:"recipient"`
	Phone      string `db:"phone" json:"phone"`
	Province   string `db:"province" json:"province"`
	City       string `db:"city" json:"city"`
	Address    string `db:"address" json:"address"`
	PostalCode string `db:"postal_code" json:"postal_code"`
	IsDefault  bool   `db:"is_default" json:"is_default"`
}

func (m *mod) listAddresses(w http.ResponseWriter, r *http.Request) error {
	a, err := db.All[address](r.Context(), m.DB, `SELECT id,title,recipient,phone,province,city,address,postal_code,is_default
		FROM addresses WHERE user_id=$1 ORDER BY is_default DESC, id DESC`, auth.From(r.Context()).UserID)
	if err != nil {
		return err
	}
	httpx.OK(w, a)
	return nil
}

func (m *mod) saveAddress(w http.ResponseWriter, r *http.Request) error {
	var in address
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	in.Phone = httpx.Phone(in.Phone)
	if strings.TrimSpace(in.Recipient) == "" || strings.TrimSpace(in.Address) == "" || !httpx.ValidMobile(in.Phone) {
		return httpx.BadRequest("نام گیرنده، موبایل معتبر و نشانی الزامی است")
	}
	uid := auth.From(r.Context()).UserID
	ctx := r.Context()
	err := m.DB.Tx(ctx, func(tx pgx.Tx) error {
		if in.IsDefault {
			if _, e := tx.Exec(ctx, `UPDATE addresses SET is_default=FALSE WHERE user_id=$1`, uid); e != nil {
				return e
			}
		}
		if idStr := chi.URLParam(r, "id"); idStr != "" {
			id, e := httpx.ID(r, "id")
			if e != nil {
				return e
			}
			tag, e := tx.Exec(ctx, `UPDATE addresses SET title=$3,recipient=$4,phone=$5,province=$6,city=$7,address=$8,postal_code=$9,is_default=$10
				WHERE id=$1 AND user_id=$2`, id, uid, in.Title, in.Recipient, in.Phone, in.Province, in.City, in.Address, in.PostalCode, in.IsDefault)
			if e == nil && tag.RowsAffected() == 0 {
				e = httpx.ErrNotFound
			}
			in.ID = id
			return e
		}
		var e error
		in.ID, e = db.Scalar[int64](ctx, tx, `INSERT INTO addresses(user_id,title,recipient,phone,province,city,address,postal_code,is_default)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`, uid, in.Title, in.Recipient, in.Phone, in.Province, in.City, in.Address, in.PostalCode, in.IsDefault)
		return e
	})
	if err != nil {
		return err
	}
	httpx.OK(w, in)
	return nil
}

func (m *mod) deleteAddress(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	_, err = m.DB.Exec(r.Context(), `DELETE FROM addresses WHERE id=$1 AND user_id=$2`, id, auth.From(r.Context()).UserID)
	httpx.NoContent(w)
	return err
}

// ───────── wishlist ─────────

func (m *mod) wishlist(w http.ResponseWriter, r *http.Request) error {
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
		SELECT p.id,p.slug,p.name,p.images,p.price,p.price_on_request FROM wishlist wl JOIN products p ON p.id=wl.product_id
		WHERE wl.user_id=$1 AND p.deleted_at IS NULL ORDER BY wl.created_at DESC) t`, auth.From(r.Context()).UserID)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
	return nil
}

func (m *mod) wishAdd(w http.ResponseWriter, r *http.Request) error {
	pid, err := httpx.ID(r, "pid")
	if err != nil {
		return err
	}
	_, err = m.DB.Exec(r.Context(), `INSERT INTO wishlist(user_id,product_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, auth.From(r.Context()).UserID, pid)
	httpx.NoContent(w)
	return err
}

func (m *mod) wishRemove(w http.ResponseWriter, r *http.Request) error {
	pid, err := httpx.ID(r, "pid")
	if err != nil {
		return err
	}
	_, err = m.DB.Exec(r.Context(), `DELETE FROM wishlist WHERE user_id=$1 AND product_id=$2`, auth.From(r.Context()).UserID, pid)
	httpx.NoContent(w)
	return err
}

// ───────── checkout & payment ─────────

func (m *mod) checkout(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		AddressID *int64   `json:"address_id"`
		Address   *address `json:"address"`
		Note      string   `json:"note"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	p := auth.From(ctx)
	cid, err := m.cartID(r, false)
	if err != nil {
		return httpx.BadRequest("سبد خرید خالی است")
	}

	var addr address
	switch {
	case in.AddressID != nil:
		addr, err = db.One[address](ctx, m.DB, `SELECT id,title,recipient,phone,province,city,address,postal_code,is_default FROM addresses WHERE id=$1 AND user_id=$2`, *in.AddressID, p.UserID)
		if err != nil {
			return httpx.BadRequest("نشانی انتخاب‌شده پیدا نشد")
		}
	case in.Address != nil:
		addr = *in.Address
		addr.Phone = httpx.Phone(addr.Phone)
		if strings.TrimSpace(addr.Recipient) == "" || strings.TrimSpace(addr.Address) == "" || !httpx.ValidMobile(addr.Phone) {
			return httpx.BadRequest("نام گیرنده، موبایل معتبر و نشانی الزامی است")
		}
	default:
		return httpx.BadRequest("نشانی تحویل را انتخاب کنید")
	}
	snap, _ := json.Marshal(addr)

	var orderID, total int64
	var number string
	err = m.DB.Tx(ctx, func(tx pgx.Tx) error {
		out, e := m.price(ctx, tx, cid)
		if e != nil {
			return e
		}
		if len(out.Lines) == 0 {
			return httpx.BadRequest("سبد خرید خالی است")
		}
		for _, l := range out.Lines {
			if !l.Orderable {
				return httpx.BadRequest(l.Name + " در حال حاضر قابل سفارش نیست")
			}
			if l.Qty > l.Available {
				return httpx.Conflict(fmt.Sprintf("موجودی «%s» کافی نیست (موجود: %d)", l.Name, l.Available))
			}
		}
		if out.Total <= 0 {
			return httpx.BadRequest("مبلغ سفارش نامعتبر است")
		}
		total = out.Total
		e = tx.QueryRow(ctx, `INSERT INTO orders(user_id,subtotal,discount,shipping_fee,total,coupon_code,address,note)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id, number`,
			p.UserID, out.Subtotal, out.Discount, out.Shipping, out.Total, out.Coupon, snap, strings.TrimSpace(in.Note)).Scan(&orderID, &number)
		if e != nil {
			return e
		}
		for _, l := range out.Lines {
			if _, e = tx.Exec(ctx, `INSERT INTO order_items(order_id,product_id,part_id,name,unit_price,qty,total) VALUES($1,$2,$3,$4,$5,$6,$7)`,
				orderID, l.ProductID, l.PartID, l.Name, l.UnitPrice, l.Qty, l.Total); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	url, err := m.startPayment(ctx, orderID, total, addr.Phone)
	if err != nil {
		return err
	}
	httpx.Created(w, map[string]any{"order_id": orderID, "number": number, "total": total, "payment_url": url})
	return nil
}

func (m *mod) startPayment(ctx context.Context, orderID, amount int64, phone string) (string, error) {
	cb := fmt.Sprintf("%s/api/v1/shop/payment/callback?order=%d", m.Cfg.PublicURL, orderID)
	res, err := m.Pay.Start(ctx, amount, cb, fmt.Sprintf("سفارش اوینکی #%d", orderID), phone)
	if err != nil {
		return "", httpx.E(502, "gateway_error", "درگاه پرداخت در دسترس نیست؛ کمی بعد دوباره تلاش کنید")
	}
	_, err = m.DB.Exec(ctx, `INSERT INTO payments(order_id,gateway,amount,authority) VALUES($1,$2,$3,$4)`, orderID, m.Pay.Name(), amount, res.Authority)
	return res.URL, err
}

func (m *mod) repay(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	type o struct {
		Total int64  `db:"total"`
		Phone string `db:"phone"`
	}
	v, err := db.One[o](r.Context(), m.DB, `SELECT total, address->>'phone' AS phone FROM orders WHERE id=$1 AND user_id=$2 AND status='pending_payment'`, id, auth.From(r.Context()).UserID)
	if err != nil {
		return err
	}
	url, err := m.startPayment(r.Context(), id, v.Total, v.Phone)
	if err != nil {
		return err
	}
	httpx.OK(w, map[string]any{"payment_url": url})
	return nil
}

// callback verifies with the gateway and redirects the browser to the site.
func (m *mod) callback(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	authority := r.URL.Query().Get("Authority")
	orderID, _ := httpx.QInt(r, "order")
	done := func(ok bool) error {
		http.Redirect(w, r, fmt.Sprintf("%s/account/orders/%d?paid=%t", m.Cfg.PublicURL, orderID, ok), http.StatusSeeOther)
		return nil
	}
	type pay struct {
		ID     int64  `db:"id"`
		Amount int64  `db:"amount"`
		Status string `db:"status"`
	}
	pm, err := db.One[pay](ctx, m.DB, `SELECT id,amount,status FROM payments WHERE gateway=$1 AND authority=$2 AND order_id=$3`, m.Pay.Name(), authority, orderID)
	if err != nil {
		return done(false)
	}
	if pm.Status == "paid" {
		return done(true)
	}
	if r.URL.Query().Get("Status") != "OK" {
		_, _ = m.DB.Exec(ctx, `UPDATE payments SET status='failed' WHERE id=$1`, pm.ID)
		return done(false)
	}
	vr, err := m.Pay.Verify(ctx, authority, pm.Amount)
	if err != nil || !vr.OK {
		_, _ = m.DB.Exec(ctx, `UPDATE payments SET status='failed' WHERE id=$1`, pm.ID)
		return done(false)
	}
	err = m.DB.Tx(ctx, func(tx pgx.Tx) error {
		// the status guard makes a repeated callback harmless
		tag, e := tx.Exec(ctx, `UPDATE payments SET status='paid', ref_id=$2, card_pan=$3, verified_at=NOW() WHERE id=$1 AND status<>'paid'`, pm.ID, vr.RefID, vr.CardPan)
		if e != nil || tag.RowsAffected() == 0 {
			return e
		}
		tag, e = tx.Exec(ctx, `UPDATE orders SET status='paid', paid_at=NOW(), updated_at=NOW() WHERE id=$1 AND status='pending_payment'`, orderID)
		if e != nil || tag.RowsAffected() == 0 {
			return e
		}
		// stock leaves when money arrives, never before
		if _, e = tx.Exec(ctx, `UPDATE products p SET stock_qty=GREATEST(0,p.stock_qty-oi.qty)
			FROM order_items oi WHERE oi.order_id=$1 AND oi.product_id=p.id AND p.track_stock`, orderID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE coupons SET used_count=used_count+1 WHERE code=(SELECT coupon_code FROM orders WHERE id=$1) AND $1>0`, orderID); e != nil {
			return e
		}
		// the cart has done its job
		if _, e = tx.Exec(ctx, `DELETE FROM cart_items WHERE cart_id IN (SELECT c.id FROM carts c JOIN orders o ON o.user_id=c.user_id WHERE o.id=$1)`, orderID); e != nil {
			return e
		}
		var phone, number string
		var uid int64
		if e = tx.QueryRow(ctx, `SELECT o.number, COALESCE(NULLIF(o.address->>'phone',''),u.phone), o.user_id FROM orders o JOIN users u ON u.id=o.user_id WHERE o.id=$1`, orderID).Scan(&number, &phone, &uid); e != nil {
			return e
		}
		_ = sms.Enqueue(ctx, tx, phone, "order_paid", number)
		_, e = tx.Exec(ctx, `INSERT INTO notifications(user_id,kind,title,body,link) VALUES($1,'order','پرداخت سفارش موفق بود',$2,$3)`,
			uid, "سفارش "+number+" ثبت شد.", fmt.Sprintf("/account/orders/%d", orderID))
		return e
	})
	if err != nil {
		return err
	}
	return done(true)
}

// ───────── orders ─────────

type orderRow struct {
	ID           int64           `db:"id" json:"id"`
	Number       string          `db:"number" json:"number"`
	UserID       int64           `db:"user_id" json:"user_id"`
	CustomerName string          `db:"customer_name" json:"customer_name"`
	Status       string          `db:"status" json:"status"`
	Subtotal     int64           `db:"subtotal" json:"subtotal"`
	Discount     int64           `db:"discount" json:"discount"`
	ShippingFee  int64           `db:"shipping_fee" json:"shipping_fee"`
	Total        int64           `db:"total" json:"total"`
	CouponCode   string          `db:"coupon_code" json:"coupon_code"`
	Address      json.RawMessage `db:"address" json:"address"`
	Note         string          `db:"note" json:"note"`
	TrackingCode string          `db:"tracking_code" json:"tracking_code"`
	PaidAt       *time.Time      `db:"paid_at" json:"paid_at"`
	CreatedAt    time.Time       `db:"created_at" json:"created_at"`
	ItemCount    int             `db:"item_count" json:"item_count"`
}

const orderSel = `SELECT o.id,o.number,o.user_id,COALESCE(NULLIF(u.full_name,''),u.username) AS customer_name,o.status,o.subtotal,o.discount,
	o.shipping_fee,o.total,o.coupon_code,o.address,o.note,o.tracking_code,o.paid_at,o.created_at,
	COALESCE((SELECT SUM(qty) FROM order_items WHERE order_id=o.id),0)::int AS item_count
	FROM orders o JOIN users u ON u.id=o.user_id`

type orderItem struct {
	ID         int64  `db:"id" json:"id"`
	ProductID  *int64 `db:"product_id" json:"product_id"`
	PartID     *int64 `db:"part_id" json:"part_id"`
	Name       string `db:"name" json:"name"`
	UnitPrice  int64  `db:"unit_price" json:"unit_price"`
	Qty        int    `db:"qty" json:"qty"`
	Total      int64  `db:"total" json:"total"`
	SerialCode string `db:"serial_code" json:"serial_code"`
}

func (m *mod) detail(ctx context.Context, where string, args ...any) (map[string]any, error) {
	o, err := db.One[orderRow](ctx, m.DB, orderSel+" WHERE "+where, args...)
	if err != nil {
		return nil, err
	}
	items, err := db.All[orderItem](ctx, m.DB, `SELECT id,product_id,part_id,name,unit_price,qty,total,serial_code FROM order_items WHERE order_id=$1 ORDER BY id`, o.ID)
	if err != nil {
		return nil, err
	}
	pays, err := db.Scalar[json.RawMessage](ctx, m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (SELECT gateway,amount,status,ref_id,created_at FROM payments WHERE order_id=$1 ORDER BY id) t`, o.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"order": o, "items": items, "payments": pays}, nil
}

func (m *mod) myOrders(w http.ResponseWriter, r *http.Request) error {
	o, err := db.All[orderRow](r.Context(), m.DB, orderSel+` WHERE o.user_id=$1 ORDER BY o.id DESC LIMIT 100`, auth.From(r.Context()).UserID)
	if err != nil {
		return err
	}
	httpx.OK(w, o)
	return nil
}

func (m *mod) myOrder(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	d, err := m.detail(r.Context(), `o.id=$1 AND o.user_id=$2`, id, auth.From(r.Context()).UserID)
	if err != nil {
		return err
	}
	httpx.OK(w, d)
	return nil
}

func (m *mod) cancelMine(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	tag, err := m.DB.Exec(r.Context(), `UPDATE orders SET status='cancelled', updated_at=NOW() WHERE id=$1 AND user_id=$2 AND status='pending_payment'`, id, auth.From(r.Context()).UserID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.Conflict("فقط سفارش پرداخت‌نشده قابل لغو است")
	}
	httpx.NoContent(w)
	return nil
}

func (m *mod) adminOrders(w http.ResponseWriter, r *http.Request) error {
	status, q := httpx.Q(r, "status"), httpx.Q(r, "q")
	pg := httpx.Paging(r, 100)
	where := ` WHERE ($1='' OR o.status=$1) AND ($2='' OR o.number ILIKE '%'||$2||'%' OR u.phone LIKE '%'||$2||'%' OR u.full_name ILIKE '%'||$2||'%')`
	total, err := db.Scalar[int](r.Context(), m.DB, `SELECT COUNT(*) FROM orders o JOIN users u ON u.id=o.user_id`+where, status, q)
	if err != nil {
		return err
	}
	items, err := db.All[orderRow](r.Context(), m.DB, orderSel+where+` ORDER BY o.id DESC LIMIT $3 OFFSET $4`, status, q, pg.Limit, pg.Offset)
	if err != nil {
		return err
	}
	httpx.OK(w, httpx.List[orderRow]{Items: items, Total: total, Page: pg.Page, Limit: pg.Limit})
	return nil
}

func (m *mod) adminOrder(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	d, err := m.detail(r.Context(), `o.id=$1`, id)
	if err != nil {
		return err
	}
	httpx.OK(w, d)
	return nil
}

var transitions = map[string][]string{
	"paid":       {"processing", "shipped", "cancelled", "refunded"},
	"processing": {"shipped", "cancelled", "refunded"},
	"shipped":    {"delivered", "refunded"},
	"delivered":  {"refunded"},
}

func (m *mod) setStatus(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Status       string `json:"status"`
		TrackingCode string `json:"tracking_code"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	err = m.DB.Tx(ctx, func(tx pgx.Tx) error {
		cur, e := db.Scalar[string](ctx, tx, `SELECT status FROM orders WHERE id=$1 FOR UPDATE`, id)
		if e != nil {
			return e
		}
		allowed := false
		for _, s := range transitions[cur] {
			allowed = allowed || s == in.Status
		}
		if !allowed {
			return httpx.Conflict(fmt.Sprintf("تغییر وضعیت از %s به %s ممکن نیست", cur, in.Status))
		}
		if in.Status == "shipped" && strings.TrimSpace(in.TrackingCode) == "" {
			return httpx.BadRequest("کد رهگیری را وارد کنید")
		}
		if _, e = tx.Exec(ctx, `UPDATE orders SET status=$2, tracking_code=COALESCE(NULLIF($3,''),tracking_code), updated_at=NOW(),
			shipped_at=CASE WHEN $2='shipped' THEN NOW() ELSE shipped_at END,
			delivered_at=CASE WHEN $2='delivered' THEN NOW() ELSE delivered_at END WHERE id=$1`, id, in.Status, strings.TrimSpace(in.TrackingCode)); e != nil {
			return e
		}
		switch in.Status {
		case "shipped":
			var phone, number string
			if e = tx.QueryRow(ctx, `SELECT COALESCE(NULLIF(address->>'phone',''),''), number FROM orders WHERE id=$1`, id).Scan(&phone, &number); e != nil {
				return e
			}
			return sms.Enqueue(ctx, tx, phone, "order_shipped", number, strings.TrimSpace(in.TrackingCode))
		case "delivered":
			if settings.Bool(ctx, tx, "guarantee_auto_approve_orders") {
				if e = warranty.IssueForOrder(ctx, tx, id); e != nil {
					return e
				}
			}
			return surveys.TriggerOrder(ctx, tx, id)
		case "cancelled", "refunded":
			// give stock back if it had been taken
			_, e = tx.Exec(ctx, `UPDATE products p SET stock_qty=p.stock_qty+oi.qty FROM order_items oi
				WHERE oi.order_id=$1 AND oi.product_id=p.id AND p.track_stock AND $2 IN ('cancelled','refunded')`, id, in.Status)
			return e
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "order.status", "order", id, in.Status)
	httpx.NoContent(w)
	return nil
}

func (m *mod) setSerial(w http.ResponseWriter, r *http.Request) error {
	oid, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	item, err := httpx.ID(r, "item")
	if err != nil {
		return err
	}
	var in struct {
		SerialCode string `json:"serial_code"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	tag, err := m.DB.Exec(r.Context(), `UPDATE order_items SET serial_code=$3 WHERE id=$2 AND order_id=$1`, oid, item, strings.TrimSpace(httpx.Digits(in.SerialCode)))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.ErrNotFound
	}
	httpx.NoContent(w)
	return nil
}

// ───────── coupons ─────────

func (m *mod) listCoupons(w http.ResponseWriter, r *http.Request) error {
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(c ORDER BY c.created_at DESC),'[]') FROM coupons c`)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
	return nil
}

func (m *mod) saveCoupon(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Code        string     `json:"code"`
		Kind        string     `json:"kind"`
		Value       int64      `json:"value"`
		MaxDiscount int64      `json:"max_discount"`
		MinTotal    int64      `json:"min_total"`
		MaxUses     int        `json:"max_uses"`
		StartsAt    *time.Time `json:"starts_at"`
		EndsAt      *time.Time `json:"ends_at"`
		IsActive    bool       `json:"is_active"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	if in.Code == "" || (in.Kind != "percent" && in.Kind != "fixed") || in.Value <= 0 || (in.Kind == "percent" && in.Value > 100) {
		return httpx.BadRequest("کد، نوع و مقدار تخفیف را درست وارد کنید")
	}
	_, err := m.DB.Exec(r.Context(), `INSERT INTO coupons(code,kind,value,max_discount,min_total,max_uses,starts_at,ends_at,is_active)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (code) DO UPDATE SET kind=EXCLUDED.kind,value=EXCLUDED.value,
		max_discount=EXCLUDED.max_discount,min_total=EXCLUDED.min_total,max_uses=EXCLUDED.max_uses,
		starts_at=EXCLUDED.starts_at,ends_at=EXCLUDED.ends_at,is_active=EXCLUDED.is_active`,
		in.Code, in.Kind, in.Value, in.MaxDiscount, in.MinTotal, in.MaxUses, in.StartsAt, in.EndsAt, in.IsActive)
	if err != nil {
		return err
	}
	m.Audit.Log(r, "coupon.save", "coupon", in.Code, "")
	httpx.NoContent(w)
	return nil
}

func (m *mod) deleteCoupon(w http.ResponseWriter, r *http.Request) error {
	_, err := m.DB.Exec(r.Context(), `DELETE FROM coupons WHERE code=$1`, strings.ToUpper(chi.URLParam(r, "code")))
	httpx.NoContent(w)
	return err
}
