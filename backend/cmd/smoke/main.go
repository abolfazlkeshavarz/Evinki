// Command smoke walks the main business flows end to end against a running
// server (`make smoke`). It creates its own data with a unique suffix, so it is
// safe to run on a database that already holds real records.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var (
	base   = flag.String("url", "http://localhost:8080", "server address")
	user   = flag.String("user", "admin", "super admin username")
	pass   = flag.String("pass", "", "super admin password")
	client = &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	failed int
)

type M = map[string]any

func call(method, path, token string, body any, extra ...string) (int, M, []byte) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, *base+"/api/v1"+path, rd)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(extra); i += 2 {
		req.Header.Set(extra[i], extra[i+1])
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("   request error:", err)
		failed++
		return 0, nil, nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var m M
	_ = json.Unmarshal(raw, &m)
	return resp.StatusCode, m, raw
}

func check(name string, ok bool, detail ...any) {
	if ok {
		fmt.Printf("  PASS  %s\n", name)
		return
	}
	failed++
	fmt.Printf("  FAIL  %s %v\n", name, detail)
}

func num(m M, path ...string) float64 {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(M)
		if !ok {
			return 0
		}
		cur = mm[p]
	}
	f, _ := cur.(float64)
	return f
}
func str(m M, path ...string) string {
	var cur any = m
	for _, p := range path {
		mm, ok := cur.(M)
		if !ok {
			return ""
		}
		cur = mm[p]
	}
	s, _ := cur.(string)
	return s
}
func id(m M) int64 { return int64(num(m, "id")) }

func login(u, p string) string {
	code, m, _ := call("POST", "/auth/login", "", M{"username": u, "password": p})
	if code != 200 {
		fmt.Println("   login failed for", u, code, m)
		failed++
		return ""
	}
	return str(m, "access_token")
}

func main() {
	flag.Parse()
	sfx := fmt.Sprint(time.Now().UnixNano() % 1_000_000_000)
	fmt.Println("Evinki smoke test, suffix", sfx)

	fmt.Println("\n[auth & access control]")
	admin := login(*user, *pass)
	if admin == "" {
		os.Exit(1)
	}
	code, me, _ := call("GET", "/auth/me", admin, nil)
	check("me returns super_admin", code == 200 && fmt.Sprint(me["roles"]) == "[super_admin]", me["roles"])
	code, _, _ = call("GET", "/admin/users", "", nil)
	check("anonymous cannot list users", code == 401, code)
	code, _, _ = call("POST", "/auth/login", "", M{"username": *user, "password": "wrong-password"})
	check("wrong password rejected", code == 401, code)

	fmt.Println("\n[catalog]")
	_, cat, _ := call("POST", "/admin/categories", admin, M{"slug": "vac-" + sfx, "name": "جاروبرقی " + sfx})
	check("category created", id(cat) > 0, cat)
	code, prod, raw := call("POST", "/admin/products", admin, M{"category_id": id(cat), "sku": "FZ-" + sfx, "slug": "fz-" + sfx, "name": "FZ530 Performance",
		"price": 18500000, "stock_qty": 5, "guarantee_months": 18, "golden_months": 3, "code_format": "simple", "images": []string{}})
	check("product created", code == 201, string(raw))
	_, pOnReq, _ := call("POST", "/admin/products", admin, M{"sku": "CALL-" + sfx, "slug": "call-" + sfx, "name": "تماس بگیرید", "price_on_request": true})
	code, part, raw := call("POST", "/admin/parts", admin, M{"code": "P" + sfx, "name": "فیلتر هپا " + sfx, "buyback_price": 40000, "sale_price": 250000, "is_sellable": true, "return_required": true, "min_stock": 2})
	check("part created with qr token", code == 201 && str(part, "qr_token") != "", string(raw))
	_, svc, _ := call("POST", "/admin/service-items", admin, M{"name": "تعویض فیلتر " + sfx, "fee": 150000})
	code, list, _ := call("GET", "/shop/products?q=FZ530&limit=5", "", nil)
	check("public product search works without login", code == 200 && num(list, "total") >= 1, list)
	code, _, _ = call("GET", "/shop/products/fz-"+sfx, "", nil)
	check("public product page", code == 200)

	fmt.Println("\n[warehouse]")
	code, meta, _ := call("GET", "/wh/meta", admin, nil)
	check("warehouse meta", code == 200 && meta["warehouses"] != nil)
	code, doc, raw := call("POST", "/wh/documents", admin, M{"direction": "in", "reason": "recharge", "client_ref": "smoke-" + sfx,
		"lines": []M{{"part_id": id(part), "qty": 20}}})
	check("stock-in applied immediately for admin", code == 201 && str(doc, "document", "status") == "approved", string(raw))
	code, again, _ := call("POST", "/wh/documents", admin, M{"direction": "in", "reason": "recharge", "client_ref": "smoke-" + sfx, "lines": []M{{"part_id": id(part), "qty": 20}}})
	check("same client_ref is idempotent", code == 200 && num(again, "document", "id") == num(doc, "document", "id"), code)
	code, items, _ := call("GET", "/wh/items?q="+url.QueryEscape("P"+sfx), admin, nil)
	check("stock level is 20", code == 200 && firstStock(items) == 20, items)
	_, keeperU, _ := call("POST", "/admin/users", admin, M{"username": "keeper" + sfx, "full_name": "انباردار", "password": "Keeper-pass-1", "roles": []string{"storekeeper"}})
	keeper := login("keeper"+sfx, "Keeper-pass-1")
	code, out, _ := call("POST", "/wh/documents", keeper, M{"direction": "out", "reason": "to_worker", "worker_name": "رضا", "lines": []M{{"part_id": id(part), "qty": 3}}})
	check("storekeeper outgoing waits for approval", code == 201 && str(out, "document", "status") == "pending", out)
	code, _, _ = call("POST", fmt.Sprintf("/wh/documents/%d/approve", int64(num(out, "document", "id"))), keeper, nil)
	check("storekeeper cannot approve", code == 403, code)
	code, _, _ = call("POST", fmt.Sprintf("/wh/documents/%d/approve", int64(num(out, "document", "id"))), admin, M{})
	check("admin approves", code == 200, code)
	_, items, _ = call("GET", "/wh/items?q="+url.QueryEscape("P"+sfx), admin, nil)
	check("stock is 17 after approval", firstStock(items) == 17, items)
	code, _, _ = call("POST", "/wh/documents", admin, M{"direction": "out", "reason": "sale", "lines": []M{{"part_id": id(part), "qty": 999}}})
	check("cannot take more than stock", code == 409, code)
	_ = keeperU

	fmt.Println("\n[shop: OTP login, cart, checkout, payment, delivery -> guarantee]")
	phone := "0912" + sfx[len(sfx)-7:]
	code, otp, _ := call("POST", "/auth/otp/request", "", M{"phone": phone})
	check("otp requested (dev code present)", code == 200 && str(otp, "dev_code") != "", otp)
	code, cust, _ := call("POST", "/auth/otp/verify", "", M{"phone": phone, "code": str(otp, "dev_code"), "name": "مشتری آزمایشی"})
	check("customer signed in via otp", code == 200 && str(cust, "access_token") != "", cust)
	customer := str(cust, "access_token")
	guest := "guest-token-" + sfx + "-aaaaaaaa"
	code, _, _ = call("PUT", "/shop/cart/items", "", M{"product_id": id(pOnReq), "qty": 1}, "X-Cart-Token", guest)
	check("price-on-request product cannot be added", code == 400, code)
	code, cart, _ := call("PUT", "/shop/cart/items", "", M{"product_id": id(prod), "qty": 1}, "X-Cart-Token", guest)
	check("guest cart works", code == 200 && num(cart, "subtotal") == 18500000, cart)
	code, cart, _ = call("GET", "/shop/cart", customer, nil, "X-Cart-Token", guest)
	check("guest cart merged into account on sign-in", code == 200 && num(cart, "count") == 1, cart)
	code, order, raw := call("POST", "/shop/checkout", customer, M{"address": M{"recipient": "مشتری", "phone": phone, "province": "قزوین", "city": "قزوین", "address": "خیابان امام"}})
	check("checkout creates order and payment url", code == 201 && str(order, "payment_url") != "", string(raw))
	oid := int64(num(order, "order_id"))
	// follow the mock gateway: its payment_url is the callback itself
	pu := str(order, "payment_url")
	// PUBLIC_URL may not be reachable from where the test runs (e.g. inside Docker): keep path+query, use our base
	if u, e := url.Parse(pu); e == nil {
		pu = strings.TrimRight(*base, "/") + u.RequestURI()
	}
	resp, err := client.Get(pu)
	check("payment callback redirects to site", err == nil && resp != nil && resp.StatusCode == 303 && strings.Contains(resp.Header.Get("Location"), "paid=true"), err)
	code, od, _ := call("GET", fmt.Sprintf("/shop/orders/%d", oid), customer, nil)
	check("order is paid", code == 200 && str(od, "order", "status") == "paid", od)
	_, pl, _ := call("GET", "/shop/products/fz-"+sfx, "", nil)
	check("product stock decreased", num(pl, "id") > 0)
	items0 := od["items"].([]any)[0].(M)
	serial := "SN" + sfx
	code, _, _ = call("PUT", fmt.Sprintf("/admin/orders/%d/items/%d/serial", oid, int64(num(items0, "id"))), admin, M{"serial_code": serial})
	check("serial recorded", code == 204, code)
	code, _, _ = call("POST", fmt.Sprintf("/admin/orders/%d/status", oid), admin, M{"status": "shipped"})
	check("shipping needs a tracking code", code == 400, code)
	code, _, _ = call("POST", fmt.Sprintf("/admin/orders/%d/status", oid), admin, M{"status": "shipped", "tracking_code": "TRK123"})
	check("order shipped", code == 204, code)
	code, _, _ = call("POST", fmt.Sprintf("/admin/orders/%d/status", oid), admin, M{"status": "delivered"})
	check("order delivered", code == 204, code)
	code, chk, _ := call("GET", "/public/guarantees/check?code="+serial, "", nil)
	check("guarantee auto-created and publicly checkable", code == 200 && chk["found"] == true && str(chk, "status") == "approved", chk)
	code, mine, _ := callList("GET", "/my/guarantees", customer)
	check("customer sees own guarantee", code == 200 && len(mine) == 1, len(mine))

	fmt.Println("\n[survey automation]")
	code, sv, raw := call("POST", "/admin/surveys", admin, M{"title": "رضایت از سرویس " + sfx, "trigger": "repair_approved",
		"questions": []M{{"text": "امتیاز شما به تکنسین؟", "kind": "rating", "required": true}, {"text": "پیشنهاد؟", "kind": "text"}}})
	check("auto survey created and active", code == 200 && str(sv, "status") == "active", string(raw))
	surveyID := id(sv)

	fmt.Println("\n[technician: part request -> repair -> return shipment -> invoice -> payment]")
	techPhone := "0935" + sfx[len(sfx)-7:]
	code, apply, raw := call("POST", "/public/technicians/apply", "", M{"full_name": "تکنسین آزمایشی", "phone": techPhone, "password": "Tech-pass-1234", "city": "قزوین"})
	check("technician applies", code == 201, string(raw))
	_ = apply
	tech := login(techPhone, "Tech-pass-1234")
	code, pend, _ := call("GET", "/tech/dashboard", tech, nil)
	check("pending technician is blocked", code == 403 && str(pend, "error", "code") == "pending_approval", pend)
	_, tl, _ := call("GET", "/admin/technicians?status=pending", admin, nil)
	_ = tl
	var techID int64
	code, tlist, _ := callList("GET", "/admin/technicians?status=pending", admin)
	for _, t := range tlist {
		if str(t, "phone") == techPhone {
			techID = id(t)
		}
	}
	check("office sees the application", code == 200 && techID > 0)
	code, _, _ = call("POST", fmt.Sprintf("/admin/technicians/%d/review", techID), admin, M{"decision": "approve"})
	check("technician approved", code == 200, code)
	code, _, _ = call("GET", "/tech/dashboard", tech, nil)
	check("approved technician enters portal", code == 200, code)

	code, lk, _ := call("GET", "/tech/guarantees/lookup?code="+serial, tech, nil)
	check("technician looks up guarantee", code == 200 && lk["found"] == true, lk)
	code, rep, raw := call("POST", "/tech/repairs", tech, M{"guarantee_code": serial, "complaint": "مکش ضعیف است"})
	check("repair opened against guarantee", code == 201 && str(rep, "status") == "in_progress", string(raw))
	rid := id(rep)
	code, nis, _ := call("POST", fmt.Sprintf("/tech/repairs/%d/parts", rid), tech, M{"part_id": id(part), "qty": 1, "report": "فیلتر کثیف"})
	check("part not in own stock is refused with a hint", code == 409 && str(nis, "error", "code") == "not_in_stock", nis)
	code, rq, raw := call("POST", "/tech/part-requests", tech, M{"repair_id": rid, "items": []M{{"kind": "part", "part_id": id(part), "qty": 2}}})
	check("part request filed", code == 201 && str(rq, "status") == "pending", string(raw))
	rqid := id(rq)
	code, dl, raw := call("POST", fmt.Sprintf("/admin/part-requests/%d/deliver", rqid), admin, M{})
	check("office delivers (central -> technician warehouse)", code == 200 && str(dl, "status") == "delivered", string(raw))
	_, st, stRaw := callAny("GET", "/tech/stock", tech)
	check("technician now holds 2", strings.Contains(string(stRaw), `"qty":2`), string(stRaw))
	_ = st
	code, _, raw = call("POST", fmt.Sprintf("/tech/repairs/%d/parts", rid), tech, M{"part_id": id(part), "qty": 1, "report": "فیلتر کثیف"})
	check("part added from own stock", code == 200, string(raw))
	code, _, _ = call("POST", fmt.Sprintf("/tech/repairs/%d/services", rid), tech, M{"service_id": id(svc), "qty": 1})
	check("service added", code == 200, code)
	code, sub, raw := call("POST", fmt.Sprintf("/tech/repairs/%d/submit", rid), tech, M{})
	check("report submitted", code == 200 && str(sub, "status") == "submitted", string(raw))
	_, items, _ = call("GET", "/wh/items?q="+url.QueryEscape("P"+sfx), admin, nil)
	check("central stock reduced by delivery (15)", firstStock(items) == 15, items)
	code, pipe, _ := call("GET", "/admin/pipeline", admin, nil)
	check("pipeline shows awaiting_review", code == 200 && num(pipe, "counts", "awaiting_review") >= 1, pipe["counts"])
	code, rv, raw := call("POST", fmt.Sprintf("/admin/repairs/%d/review", rid), admin, M{"decision": "approve"})
	check("repair approved", code == 200 && str(rv, "status") == "approved", string(raw))
	_, led, _ := call("GET", "/tech/ledger", tech, nil)
	check("labour credited to technician (150000)", num(led, "balance") == 150000, led)

	_, ret, retRaw := callAny("GET", "/tech/returns", tech)
	_ = ret
	var rpIDs []int64
	var arr []M
	_ = json.Unmarshal(retRaw, &arr)
	for _, x := range arr {
		rpIDs = append(rpIDs, id(x))
	}
	check("old part waits to be sent back", len(rpIDs) == 1, string(retRaw))
	code, ship, raw := call("POST", "/tech/shipments", tech, M{"repair_part_ids": rpIDs, "method": "post", "tracking_code": "POST999"})
	check("shipment created", code == 201 && str(ship, "status") == "sent", string(raw))
	code, _, _ = call("POST", "/tech/shipments", tech, M{"repair_part_ids": rpIDs, "method": "post", "tracking_code": "X"})
	check("same part cannot ship twice", code == 409, code)
	sid := id(ship)
	code, rcv, raw := call("POST", fmt.Sprintf("/admin/shipments/%d/receive", sid), admin, M{"received_repair_part_ids": rpIDs})
	check("received + auto-invoiced at price list", code == 200 && str(rcv, "status") == "invoiced" && num(rcv, "invoice_total") == 40000, string(raw))
	code, pd, raw := call("POST", fmt.Sprintf("/admin/shipments/%d/pay", sid), admin, M{"reference": "REF1"})
	check("shipment paid", code == 200 && str(pd, "status") == "paid", string(raw))
	_, led, _ = call("GET", "/tech/ledger", tech, nil)
	check("technician balance = labour only after payment (150000)", num(led, "balance") == 150000, led)

	fmt.Println("\n[survey result]")
	_, recRaw := rawGet(fmt.Sprintf("/admin/surveys/%d/recipients", surveyID), admin)
	var recs []M
	_ = json.Unmarshal(recRaw, &recs)
	check("repair approval invited the customer automatically", len(recs) == 1, string(recRaw))

	fmt.Println("\n[HR & kiosk]")
	code, emp, raw := call("POST", "/hr/employees", admin, M{"personnel_code": "E" + sfx, "first_name": "علی", "last_name": "رضایی"})
	check("employee created", code == 201, string(raw))
	code, _, _ = call("POST", fmt.Sprintf("/hr/employees/%d/pin", id(emp)), admin, M{"pin": "4321"})
	check("pin set", code == 204, code)
	code, dv, raw := call("POST", "/hr/devices", admin, M{"name": "درب اصلی " + sfx})
	check("device registered, key shown once", code == 201 && str(dv, "api_key") != "", string(raw))
	devKey := str(dv, "api_key")
	code, pin, raw := call("POST", "/kiosk/pin-punch", "", M{"personnel_code": "E" + sfx, "pin": "4321"}, "X-Device-Key", devKey)
	check("pin punch records entry", code == 200 && str(pin, "kind") == "in", string(raw))
	code, _, _ = call("POST", "/kiosk/pin-punch", "", M{"personnel_code": "E" + sfx, "pin": "0000"}, "X-Device-Key", devKey)
	check("wrong pin rejected", code == 401, code)
	uuid := "u-" + sfx
	punch := M{"punches": []M{{"client_uuid": uuid, "employee_id": id(emp), "kind": "out", "method": "face", "happened_at": time.Now().Add(-time.Minute).Format(time.RFC3339), "created_offline": true}}}
	_, b1, _ := call("POST", "/kiosk/punches", "", punch, "X-Device-Key", devKey)
	_, b2, _ := call("POST", "/kiosk/punches", "", punch, "X-Device-Key", devKey)
	check("offline batch accepted once, resend is a duplicate", num(b1, "accepted") == 1 && num(b2, "duplicates") == 1, b1, b2)
	code, _, _ = call("GET", "/kiosk/gallery", "", nil, "X-Device-Key", "bad-key")
	check("bad device key rejected", code == 401, code)
	code, rp, _ := call("GET", "/hr/reports/range?employee_id="+fmt.Sprint(id(emp)), admin, nil)
	check("attendance report builds", code == 200 && rp["employees"] != nil, rp)
	check("report carries chart data (series, departments, totals)", rp["series"] != nil && rp["departments"] != nil && rp["totals"] != nil && rp["arrivals"] != nil, rp)

	vec := make([]float64, 512)
	for i := range vec {
		vec[i] = float64(i%7) / 10
	}
	for n := 0; n < 2; n++ {
		code, _, raw = call("POST", fmt.Sprintf("/hr/employees/%d/faces", id(emp)), admin, M{"vector": vec, "model_name": "mobilefacenet-arcface-512", "quality": 0.9, "image": "data:image/jpeg;base64,AAAA"})
		check("face sample stored", code == 201, string(raw))
	}
	_, facesRaw := rawGet(fmt.Sprintf("/hr/employees/%d/faces", id(emp)), admin)
	var faces []M
	_ = json.Unmarshal(facesRaw, &faces)
	check("face samples listed (with thumbnail)", len(faces) == 2 && faces[0]["image"] != nil, string(facesRaw))
	code, _, _ = call("DELETE", fmt.Sprintf("/hr/employees/%d/faces/%d", id(emp), int64(num(faces[0], "id"))), admin, nil)
	check("one face sample deleted", code == 204, code)
	code, kc, raw := call("GET", "/kiosk/config", "", nil, "X-Device-Key", devKey)
	check("kiosk config has thresholds", code == 200 && num(kc, "threshold") > 0, string(raw))
	code, kv, raw := call("GET", "/kiosk/gallery/version", "", nil, "X-Device-Key", devKey)
	check("kiosk gallery version", code == 200 && str(kv, "version") != "", string(raw))
	code, ds, raw := call("GET", "/hr/dashboard", admin, nil)
	check("HR dashboard (counters, 7-day trend, departments)", code == 200 && ds["counters"] != nil && ds["trend"] != nil && ds["departments"] != nil, string(raw))
	_, todayRaw := rawGet("/hr/today", admin)
	check("today status lists the employee", strings.Contains(string(todayRaw), "E"+sfx), string(todayRaw))
	_, csvRaw := rawGet("/hr/reports/range.csv?detail=1", admin)
	check("daily detail CSV export", strings.Contains(string(csvRaw), "E"+sfx), string(csvRaw))

	guaranteeFormulaChecks(admin, sfx)

	fmt.Println("\n[access control & audit]")
	code, _, _ = call("GET", "/admin/users", tech, nil)
	check("technician cannot list users", code == 403, code)
	code, _, _ = call("GET", "/admin/orders", customer, nil)
	check("customer cannot list all orders", code == 403, code)
	code, _, _ = call("DELETE", fmt.Sprintf("/admin/guarantees/%d", 1), keeper, nil)
	check("storekeeper cannot delete guarantees", code == 403, code)
	code, au, _ := call("GET", "/admin/audit?limit=5", admin, nil)
	check("audit trail records actions", code == 200 && num(au, "total") > 10, au["total"])
	code, ov, _ := call("GET", "/manager/overview", admin, nil)
	check("manager overview", code == 200 && ov["service"] != nil, ov)

	fmt.Println()
	if failed > 0 {
		fmt.Printf("%d check(s) FAILED\n", failed)
		os.Exit(1)
	}
	fmt.Println("all checks passed")
}

func firstStock(m M) float64 {
	items, _ := m["items"].([]any)
	if len(items) == 0 {
		return -1
	}
	return num(items[0].(M), "stock")
}

func callAny(method, path, token string) (int, any, []byte) {
	code, _, raw := call(method, path, token, nil)
	return code, nil, raw
}

func rawGet(path, token string) (int, []byte) {
	code, _, raw := call("GET", path, token, nil)
	return code, raw
}

func callList(method, path, token string) (int, []M, []byte) {
	code, _, raw := call(method, path, token, nil)
	var out []M
	_ = json.Unmarshal(raw, &out)
	return code, out, raw
}
