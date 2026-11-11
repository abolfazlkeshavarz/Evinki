package main

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

func doRaw(method, path, token string, body []byte, headers ...string) (int, []byte) {
	req, _ := http.NewRequest(method, *base+"/api/v1"+path, bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := client.Do(req)
	if err != nil {
		failed++
		fmt.Println("   request error:", err)
		return 0, nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

// guaranteeFormulaChecks covers: code formulas, warranty counted from the manufacture date, hand-edited expiry,
// code generation, export, trash, product merge, global search, technician import and the update endpoint.
func guaranteeFormulaChecks(admin, sfx string) {
	fmt.Println("\n[code formula & manufacture date]")
	formula := []M{{"type": "year", "digits": 4, "calendar": "jalali"}, {"type": "text", "value": "FZ"}, {"type": "month"}, {"type": "number", "value": "24"}, {"type": "serial", "digits": 5}}
	code, pv, raw := call("POST", "/admin/code-formula/preview", admin, M{"formula": formula, "date": "2026-11-11", "serial": 123})
	check("formula preview builds the code (year FZ month 24 serial)", code == 200 && str(pv, "example") == "1405FZ082400123", string(raw))
	_, bad, _ := call("POST", "/admin/code-formula/preview", admin, M{"formula": []M{{"type": "text", "value": "X"}}})
	check("a formula without a serial is refused", bad["valid"] == false, bad)

	code, prod, raw := call("POST", "/admin/products", admin, M{"sku": "FM-" + sfx, "slug": "fm-" + sfx, "name": "فرمول‌دار " + sfx, "guarantee_months": 18, "golden_months": 3,
		"code_formula": formula, "warranty_basis": "manufacture", "images": []string{}})
	check("product with formula created (format becomes 'formula')", code == 201 && str(prod, "code_format") == "formula", string(raw))
	pid := id(prod)
	code, _, raw = call("POST", "/admin/products", admin, M{"sku": "FB-" + sfx, "slug": "fb-" + sfx, "name": "فرمول بد", "code_formula": []M{{"type": "month"}, {"type": "serial", "digits": 3}}})
	check("invalid formula refused on save", code == 400, code, string(raw))

	gcode := "1405FZ062400045" // made in Shahrivar 1405 -> counted from the last day of that month (2026-09-22)
	code, v, raw := call("POST", "/public/guarantees/validate", "", M{"product_id": pid, "code": gcode, "purchase_date": "2026-11-11"})
	check("code validates and reveals the manufacture date", code == 200 && v["valid"] == true && str(v, "manufacture_date") == "2026-09-22" && v["date_approx"] == true, string(raw))
	check("expiry preview counted from manufacture, not purchase", str(v, "expiry_date") > "2028-01-01" && str(v, "expiry_date") < "2028-04-30", str(v, "expiry_date"))
	_, v2, _ := call("POST", "/public/guarantees/validate", "", M{"product_id": pid, "code": "1405FZ132400045"})
	check("month 13 rejected", v2["valid"] == false, v2)
	_, v3, _ := call("POST", "/public/guarantees/validate", "", M{"product_id": pid, "code": "1499FZ062400045"})
	check("a future manufacture date rejected", v3["valid"] == false, v3)

	code, g, raw := call("POST", "/admin/guarantees", admin, M{"code": gcode, "product_id": pid, "purchase_date": "2026-11-11", "full_name": "مشتری فرمول", "phone": "0912" + sfx[:7]})
	check("guarantee created with manufacture date and basis", code == 201 && str(g, "manufacture_date")[:10] == "2026-09-22" && str(g, "expiry_basis") == "manufacture", string(raw))
	gid := id(g)
	origExpiry := str(g, "expiry_date")[:10]

	code, e, raw := call("PATCH", fmt.Sprintf("/admin/guarantees/%d", gid), admin, M{"expiry_date": "2029-03-21", "expiry_note": "تمدید به درخواست مدیر"})
	check("expiry date edited by hand and stamped", code == 200 && str(e, "expiry_date")[:10] == "2029-03-21" && str(e, "expiry_edited_at") != "", string(raw))
	code, _, _ = call("PATCH", fmt.Sprintf("/admin/guarantees/%d", gid), admin, M{"expiry_date": "2020-01-01"})
	check("expiry before the purchase date refused", code == 400, code)
	code, r, raw := call("PATCH", fmt.Sprintf("/admin/guarantees/%d", gid), admin, M{"recalculate": true})
	check("recalculate restores the rule-based expiry", code == 200 && str(r, "expiry_date")[:10] == origExpiry && str(r, "expiry_edited_at") == "", string(raw))

	code, gen, raw := call("POST", fmt.Sprintf("/admin/products/%d/codes/generate", pid), admin, M{"date": "2026-09-01", "start": 1, "count": 3})
	codes, _ := gen["codes"].([]any)
	check("batch of codes generated from the formula", code == 200 && len(codes) == 3 && codes[0] == "1405FZ062400001" && codes[2] == "1405FZ062400003", string(raw))

	_, csv := doRaw("GET", "/admin/guarantees-export?code_from=1405FZ&code_to=1405FZ9", admin, nil)
	check("export CSV lists the guarantee with the Jalali manufacture date", strings.Contains(string(csv), gcode) && strings.Contains(string(csv), "1405/06/31"), string(csv))

	_, srch, _ := call("GET", "/admin/search?q="+gcode[:10], admin, nil)
	gl, _ := srch["guarantees"].([]any)
	check("global search finds the guarantee", len(gl) >= 1, srch)

	fmt.Println("\n[trash & merge]")
	call("DELETE", fmt.Sprintf("/admin/guarantees/%d", gid), admin, nil)
	_, tr, _ := call("GET", "/admin/trash", admin, nil)
	check("deleted guarantee is in the trash", num(tr, "guarantees") >= 1, tr)
	code, _, raw = call("POST", fmt.Sprintf("/admin/trash/guarantees/%d/restore", gid), admin, nil)
	check("guarantee restored from the trash", code == 204, code, string(raw))
	code, _, _ = call("GET", fmt.Sprintf("/admin/guarantees/%d", gid), admin, nil)
	check("restored guarantee is visible again", code == 200, code)

	_, a, _ := call("POST", "/admin/products", admin, M{"sku": "MA-" + sfx, "slug": "ma-" + sfx, "name": "FZ530 قدیمی " + sfx, "guarantee_months": 12, "code_prefix": "FZ", "code_format": "evinki_vacuum"})
	_, b, _ := call("POST", "/admin/products", admin, M{"sku": "MB-" + sfx, "slug": "mb-" + sfx, "name": "FZ530 فروشگاه " + sfx, "summary": "خلاصه فروشگاه"})
	_, ga, _ := call("POST", "/admin/guarantees", admin, M{"code": "MERGE" + sfx, "product_id": id(a), "purchase_date": "2026-10-01", "full_name": "مشتری ادغام", "phone": "0913" + sfx[:7]})
	code, mg, raw := call("POST", fmt.Sprintf("/admin/products/%d/merge", id(a)), admin, M{"into": id(b)})
	check("products merged", code == 200 && mg["merged"] == true, string(raw))
	_, gm, _ := call("GET", fmt.Sprintf("/admin/guarantees/%d", id(ga)), admin, nil)
	check("guarantee moved to the kept product", int64(num(gm, "guarantee", "product_id")) == id(b), gm)
	_, kept, _ := call("GET", fmt.Sprintf("/admin/products/%d", id(b)), admin, nil)
	check("kept product received the missing rules (code format)", str(kept, "code_format") == "evinki_vacuum" && str(kept, "code_prefix") == "FZ", kept)
	code, _, _ = call("GET", fmt.Sprintf("/admin/products/%d", id(a)), admin, nil)
	check("merged-away product is gone", code == 404, code)

	fmt.Println("\n[technician import & system update]")
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "t.csv")
	_, _ = fw.Write([]byte("\xEF\xBB\xBFنام و نام خانوادگی,نام کاربری,رمز عبور,موبایل,شهر\nتکنسین فایل,imp" + sfx + ",Passw0rd123,0914" + sfx[:7] + ",اصفهان\nبدون رمز,bad" + sfx + ",123,,\n"))
	mw.Close()
	code, imp := doRaw("POST", "/admin/technicians-import", admin, buf.Bytes(), "Content-Type", mw.FormDataContentType())
	check("technicians imported from a spreadsheet, bad row reported", code == 200 && strings.Contains(string(imp), `"created":1`) && strings.Contains(string(imp), `"row":3`), string(imp))

	code, st, _ := call("GET", "/admin/system/update", admin, nil)
	check("update status endpoint answers", code == 200 && st["updater_online"] != nil, st)
	code, _ = doRaw("PUT", "/admin/system/update", admin, []byte("not a gzip file"), "X-Filename", "x.tar.gz")
	check("a file that is not gzip is refused", code == 400, code)
	code, _ = doRaw("PUT", "/admin/system/update", admin, []byte("\x1f\x8b\x08"), "X-Filename", "evil.exe")
	check("wrong extension refused", code == 400, code)
	code, _, _ = call("GET", "/admin/system/update", "", nil)
	check("update screen needs sign-in", code == 401, code)
}
