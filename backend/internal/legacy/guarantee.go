package legacy

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// ImportGuarantee loads the guarantee-management-system database.
// Import the warehouse first when possible: repair components are then matched
// to warehouse parts by name instead of becoming duplicate catalog entries.
func (im *Importer) ImportGuarantee(src *sql.DB) error {
	im.Source = "guarantee"
	return im.Run(im.ctxOrBackground(), func() error {
		steps := []struct {
			name string
			fn   func(*sql.DB) error
		}{
			{"staff accounts", im.gStaff}, {"technicians", im.gTechnicians}, {"categories", im.gCategories},
			{"products", im.gProducts}, {"repair catalog", im.gCatalog}, {"customers", im.gCustomers},
			{"guarantees", im.gGuarantees}, {"repairs", im.gRepairs}, {"part requests", im.gRequests},
			{"shipments", im.gShipments}, {"surveys", im.gPolls}, {"sms templates", im.gSMS},
		}
		for _, s := range steps {
			im.logf("importing %s ...", s.name)
			if err := s.fn(src); err != nil {
				return fmt.Errorf("%s: %w", s.name, err)
			}
		}
		return nil
	})
}

func (im *Importer) gStaff(src *sql.DB) error {
	rows, err := query(src, `SELECT * FROM admins WHERE deleted_at IS NULL`)
	if err != nil {
		return err
	}
	for _, r := range rows {
		role := "admin"
		if S(r["role"]) == "technical" {
			role = "technical"
		}
		_, _, err := im.ensure("admin", r["id"], func() (int64, error) {
			return im.upsertUser(S(r["username"]), S(r["full_name"]), S(r["phone"]), S(r["email"]), S(r["password"]), B(r["is_active"]), role)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (im *Importer) gTechnicians(src *sql.DB) error {
	rows, err := query(src, `SELECT * FROM technicians WHERE deleted_at IS NULL`)
	if err != nil {
		return err
	}
	for _, r := range rows {
		_, _, err := im.ensure("technician", r["id"], func() (int64, error) {
			id, err := im.upsertUser(S(r["username"]), S(r["full_name"]), S(r["phone"]), "", S(r["password"]), B(r["is_active"]), "technician")
			if err != nil {
				return 0, err
			}
			status := strings.ToLower(S(r["status"]))
			if status == "" {
				status = "approved"
			}
			return id, im.exec(`INSERT INTO technicians(user_id,national_id,province,city,address,about,status,applied_at,reviewed_at,review_notes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
				id, S(r["national_id"]), S(r["province"]), S(r["city"]), S(r["address"]), S(r["about"]), status, T(r["applied_at"]), T(r["reviewed_at"]), S(r["review_notes"]))
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (im *Importer) gCategories(src *sql.DB) error {
	rows, err := query(src, `SELECT * FROM product_categories WHERE deleted_at IS NULL`)
	if err != nil {
		return err
	}
	for _, r := range rows {
		_, _, err := im.ensure("category", r["id"], func() (int64, error) {
			return im.one(`INSERT INTO categories(slug,name,description,is_active) VALUES($1,$2,$3,$4) RETURNING id`,
				fmt.Sprintf("legacy-cat-%v", I(r["id"])), S(r["name"]), S(r["description"]), B(r["is_active"]))
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (im *Importer) gProducts(src *sql.DB) error {
	rows, err := query(src, `SELECT * FROM products WHERE deleted_at IS NULL`)
	if err != nil {
		return err
	}
	for _, r := range rows {
		_, _, err := im.ensure("product", r["id"], func() (int64, error) {
			cat := ptrID(r["category_id"], func(i int64) (int64, bool) { return im.mapGet("category", i) })
			format := S(r["code_format"])
			switch format {
			case "simple", "jalali_encoded", "evinki_vacuum", "any":
			default:
				format = "simple"
			}
			gm, golden := int(I(r["default_guarantee_months"])), int(I(r["golden_guarantee_months"]))
			if gm == 0 {
				gm = 12
			}
			return im.one(`INSERT INTO products(category_id,sku,slug,name,description,is_active,is_published,price_on_request,guarantee_months,golden_months,code_format,code_prefix,code_pattern)
				VALUES($1,$2,$3,$4,$5,$6,FALSE,TRUE,$7,$8,$9,$10,$11) RETURNING id`,
				cat, fmt.Sprintf("LEG-%d", I(r["id"])), fmt.Sprintf("legacy-%d", I(r["id"])), S(r["name"]), S(r["description"]), B(r["is_active"]),
				gm, golden, format, S(r["code_prefix"]), S(r["code_pattern"]))
		})
		if err != nil {
			return err
		}
	}
	im.logf("   note: imported products are unpublished in the shop until you review them")
	return nil
}

// gCatalog maps repair components onto warehouse parts by name.
func (im *Importer) gCatalog(src *sql.DB) error {
	existing := map[string]int64{}
	rows, err := im.tx.Query(im.ctx, `SELECT id, name, variant FROM parts WHERE deleted_at IS NULL`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var name, variant string
		if err := rows.Scan(&id, &name, &variant); err != nil {
			return err
		}
		existing[NormName(name)] = id
		if variant != "" {
			existing[NormName(name+" "+variant)] = id
		}
	}
	rows.Close()

	comps, err := query(src, `SELECT * FROM repair_components WHERE deleted_at IS NULL`)
	if err != nil {
		return err
	}
	matched, created := 0, 0
	for _, r := range comps {
		_, fresh, err := im.ensure("component", r["id"], func() (int64, error) {
			if id, ok := existing[NormName(S(r["name"]))]; ok {
				matched++
				return id, nil
			}
			created++
			tok := fmt.Sprintf("LEG%05d", I(r["id"]))
			return im.one(`INSERT INTO parts(code,name,note,qr_token,is_active) VALUES($1,$2,$3,$4,$5) RETURNING id`,
				fmt.Sprintf("RC-%d", I(r["id"])), S(r["name"]), S(r["description"]), tok, B(r["is_active"]))
		})
		_ = fresh
		if err != nil {
			return err
		}
	}
	im.logf("   repair components: %d matched to warehouse parts by name, %d created as new parts", matched, created)

	svcs, err := query(src, `SELECT * FROM repair_services WHERE deleted_at IS NULL`)
	if err != nil {
		return err
	}
	for _, r := range svcs {
		if _, _, err := im.ensure("service", r["id"], func() (int64, error) {
			return im.one(`INSERT INTO service_items(name,description,is_active) VALUES($1,$2,$3) RETURNING id`, S(r["name"]), S(r["description"]), B(r["is_active"]))
		}); err != nil {
			return err
		}
	}
	return nil
}

func (im *Importer) gCustomers(src *sql.DB) error {
	rows, err := query(src, `SELECT * FROM customers WHERE deleted_at IS NULL`)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if _, _, err := im.ensure("customer", r["id"], func() (int64, error) {
			return im.one(`INSERT INTO customers(full_name,phone,national_id,province,city,address,created_at) VALUES($1,$2,$3,$4,$5,$6,COALESCE($7,NOW())) RETURNING id`,
				S(r["full_name"]), normPhone(S(r["phone"])), S(r["national_id"]), S(r["province"]), S(r["city"]), S(r["address"]), T(r["created_at"]))
		}); err != nil {
			return err
		}
	}
	return nil
}

func normPhone(p string) string {
	p = strings.NewReplacer(" ", "", "-", "", "+", "").Replace(p)
	switch {
	case strings.HasPrefix(p, "0098"):
		return "0" + p[4:]
	case strings.HasPrefix(p, "98") && len(p) == 12:
		return "0" + p[2:]
	case strings.HasPrefix(p, "9") && len(p) == 10:
		return "0" + p
	}
	return p
}

func (im *Importer) gGuarantees(src *sql.DB) error {
	rows, err := query(src, `SELECT * FROM guarantees WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return err
	}
	for _, r := range rows {
		cust, ok1 := im.mapGet("customer", r["customer_id"])
		prod, ok2 := im.mapGet("product", r["product_id"])
		if !ok1 || !ok2 {
			im.Stats["guarantees skipped (customer/product missing)"]++
			continue
		}
		status := strings.ToLower(S(r["status"]))
		if status == "" {
			status = "pending"
		}
		if _, _, err := im.ensure("guarantee", r["id"], func() (int64, error) {
			return im.one(`INSERT INTO guarantees(code,customer_id,product_id,purchase_date,expiry_date,golden_start_date,golden_expiry_date,status,source,invoice_file,card_file,notes,approved_at,created_at)
				VALUES($1,$2,$3,$4::date,$5::date,$6::date,$7::date,$8,'import',$9,$10,$11,$12,COALESCE($13,NOW())) RETURNING id`,
				S(r["code"]), cust, prod, dateS(r["purchase_date"]), dateS(r["expiry_date"]), dateS(r["golden_start_date"]), dateS(r["golden_expiry_date"]),
				status, S(r["invoice_image"]), S(r["guarantee_card_image"]), S(r["notes"]), T(r["approved_at"]), T(r["created_at"]))
		}); err != nil {
			return err
		}
	}
	return nil
}

func dateS(v any) *string {
	t := T(v)
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

var repairStatus = map[string]string{"pending": "submitted", "approved": "approved", "rejected": "rejected", "cancelled": "cancelled"}

func (im *Importer) gRepairs(src *sql.DB) error {
	rows, err := query(src, `SELECT * FROM repairs WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return err
	}
	for _, r := range rows {
		g, ok := im.mapGet("guarantee", r["guarantee_id"])
		if !ok {
			im.Stats["repairs skipped (guarantee missing)"]++
			continue
		}
		status := repairStatus[strings.ToLower(S(r["status"]))]
		if status == "" {
			status = "submitted"
		}
		tech := ptrID(r["technician_id"], func(i int64) (int64, bool) { return im.mapGet("technician", i) })
		if _, _, err := im.ensure("repair", r["id"], func() (int64, error) {
			return im.one(`INSERT INTO repairs(guarantee_id,customer_id,product_id,technician_id,status,source,report,guarantee_was_expired,reviewed_at,review_notes,started_at,submitted_at,completed_at,created_at)
				SELECT $1,g.customer_id,g.product_id,$2,$3,'technician',$4,$5,$6::timestamptz,$7,$8::timestamptz,COALESCE($8::timestamptz,$9::timestamptz),$10::timestamptz,COALESCE($9::timestamptz,NOW()) FROM guarantees g WHERE g.id=$1 RETURNING id`,
				g, tech, status, S(r["description"]), B(r["guarantee_was_expired"]), T(r["reviewed_at"]), S(r["review_notes"]), T(r["started_at"]), T(r["created_at"]), T(r["completed_at"]))
		}); err != nil {
			return err
		}
	}

	items, err := query(src, `SELECT * FROM repair_component_items ORDER BY id`)
	if err != nil {
		return err
	}
	for _, r := range items {
		rep, ok1 := im.mapGet("repair", r["repair_id"])
		part, ok2 := im.mapGet("component", r["repair_component_id"])
		if !ok1 || !ok2 {
			continue
		}
		if _, _, err := im.ensure("repair part", r["id"], func() (int64, error) {
			// shipments (below) refine return_state; until then an approved/submitted repair still owes the old part
			return im.one(`INSERT INTO repair_parts(repair_id,part_id,qty,report,source,return_required,return_state)
				SELECT $1,$2,1,$3,'stock',TRUE,CASE WHEN r.status IN ('submitted','approved') THEN 'pending' ELSE 'none' END FROM repairs r WHERE r.id=$1 RETURNING id`,
				rep, part, S(r["report"]))
		}); err != nil {
			return err
		}
	}
	svc, err := query(src, `SELECT * FROM repair_service_items ORDER BY id`)
	if err != nil {
		return err
	}
	for _, r := range svc {
		rep, ok1 := im.mapGet("repair", r["repair_id"])
		sid, ok2 := im.mapGet("service", r["repair_service_id"])
		if !ok1 || !ok2 {
			continue
		}
		if _, _, err := im.ensure("repair service", r["id"], func() (int64, error) {
			return im.one(`INSERT INTO repair_services(repair_id,service_id,qty,fee,report) VALUES($1,$2,1,0,$3) RETURNING id`, rep, sid, S(r["report"]))
		}); err != nil {
			return err
		}
	}
	return nil
}

var requestStatus = map[string]string{"pending": "pending", "approved": "approved", "notdelivered": "not_delivered", "delivered": "delivered", "cancelled": "cancelled"}

func (im *Importer) gRequests(src *sql.DB) error {
	rows, err := query(src, `SELECT * FROM component_requests WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return err
	}
	for _, r := range rows {
		tech, ok := im.mapGet("technician", r["technician_id"])
		if !ok {
			continue
		}
		status := requestStatus[strings.ToLower(S(r["status"]))]
		if status == "" {
			status = "pending"
		}
		repair := ptrID(r["repair_id"], func(i int64) (int64, bool) { return im.mapGet("repair", i) })
		if _, _, err := im.ensure("part request", r["id"], func() (int64, error) {
			return im.one(`INSERT INTO part_requests(technician_id,repair_id,status,notes,guarantee_was_expired,reviewed_at,review_notes,delivered_at,created_at)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,COALESCE($9,NOW())) RETURNING id`,
				tech, repair, status, S(r["notes"]), B(r["guarantee_was_expired"]), T(r["reviewed_at"]), S(r["review_notes"]), T(r["delivered_at"]), T(r["created_at"]))
		}); err != nil {
			return err
		}
	}
	items, err := query(src, `SELECT * FROM component_request_items ORDER BY id`)
	if err != nil {
		return err
	}
	for _, r := range items {
		req, ok := im.mapGet("part request", r["component_request_id"])
		if !ok {
			continue
		}
		kind := map[string]string{"component": "part", "service": "service", "custom": "custom"}[S(r["item_type"])]
		part := ptrID(r["repair_component_id"], func(i int64) (int64, bool) { return im.mapGet("component", i) })
		svc := ptrID(r["repair_service_id"], func(i int64) (int64, bool) { return im.mapGet("service", i) })
		if kind == "part" && part == nil || kind == "service" && svc == nil || kind == "" {
			continue
		}
		if kind == "custom" && S(r["custom_item_name"]) == "" {
			continue
		}
		if _, _, err := im.ensure("part request item", r["id"], func() (int64, error) {
			return im.one(`INSERT INTO part_request_items(request_id,kind,part_id,service_id,custom_name,qty) VALUES($1,$2,$3,$4,$5,GREATEST(1,LEAST($6,999))) RETURNING id`,
				req, kind, part, svc, S(r["custom_item_name"]), I(r["quantity"]))
		}); err != nil {
			return err
		}
	}
	return nil
}

func (im *Importer) gShipments(src *sql.DB) error {
	if !tableExists(src, "part_shipments") {
		return nil
	}
	rows, err := query(src, `SELECT * FROM part_shipments WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return err
	}
	for _, r := range rows {
		tech, ok := im.mapGet("technician", r["technician_id"])
		if !ok {
			continue
		}
		status := strings.ToLower(S(r["status"]))
		sid, fresh, err := im.ensure("shipment", r["id"], func() (int64, error) {
			return im.one(`INSERT INTO shipments(technician_id,status,method,tracking_code,sent_on,notes,received_at,receive_notes,invoiced_at,invoice_total,paid_at,payment_ref,payment_notes,review_notes,created_at)
				VALUES($1,$2,$3,$4,$5::date,$6,$7,$8,$9,$10,$11,$12,$13,$14,COALESCE($15,NOW())) RETURNING id`,
				tech, status, S(r["shipping_method"]), S(r["tracking_code"]), dateS(r["sent_on"]), S(r["notes"]), T(r["received_at"]), S(r["receive_notes"]),
				T(r["invoiced_at"]), I(r["invoice_total"]), T(r["paid_at"]), S(r["payment_reference"]), S(r["payment_notes"]), S(r["review_notes"]), T(r["created_at"]))
		})
		if err != nil {
			return err
		}
		if !fresh {
			continue
		}
		// the technician's account mirrors the invoice and payment, so balances survive the move
		total := I(r["invoice_total"])
		if total > 0 && (status == "invoiced" || status == "paid") {
			if err := im.exec(`INSERT INTO technician_ledger(technician_id,kind,amount,ref_type,ref_id,note,created_at) VALUES($1,'part_buyback',$2,'shipment',$3,'انتقال از سامانه قدیم',COALESCE($4,NOW()))`,
				tech, total, sid, T(r["invoiced_at"])); err != nil {
				return err
			}
			if status == "paid" {
				if err := im.exec(`INSERT INTO technician_ledger(technician_id,kind,amount,ref_type,ref_id,note,created_at) VALUES($1,'payment',$2,'shipment',$3,'انتقال از سامانه قدیم',COALESCE($4,NOW()))`,
					tech, -total, sid, T(r["paid_at"])); err != nil {
					return err
				}
			}
		}
	}
	items, err := query(src, `SELECT * FROM part_shipment_items ORDER BY id`)
	if err != nil {
		return err
	}
	for _, r := range items {
		sid, ok1 := im.mapGet("shipment", r["shipment_id"])
		rp, ok2 := im.mapGet("repair part", r["repair_component_item_id"])
		if !ok1 || !ok2 {
			continue
		}
		state := "in_shipment"
		var shipStatus string
		_ = im.tx.QueryRow(im.ctx, `SELECT status FROM shipments WHERE id=$1`, sid).Scan(&shipStatus)
		switch shipStatus {
		case "received", "invoiced", "paid":
			state = "received"
			if r["received"] != nil && !B(r["received"]) {
				state = "pending" // marked "did not arrive": owed again
			}
		case "rejected", "cancelled":
			state = "pending"
		}
		shipID := &sid
		if state == "pending" {
			shipID = nil
		}
		if err := im.exec(`UPDATE repair_parts SET return_state=$2, shipment_id=$3, buyback_price=$4, condition_note=$5 WHERE id=$1`,
			rp, state, shipID, I(r["unit_price"]), S(r["condition_note"])); err != nil {
			return err
		}
	}
	return nil
}

func (im *Importer) gPolls(src *sql.DB) error {
	if !tableExists(src, "polls") {
		return nil
	}
	polls, err := query(src, `SELECT * FROM polls WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return err
	}
	for _, r := range polls {
		status := strings.ToLower(S(r["status"]))
		switch status {
		case "draft", "closed":
		default:
			status = "active" // old "Sending"/"Sent" are simply live surveys now
		}
		filter, _ := json.Marshal(map[string]any{"city": S(r["filter_city"]), "province": S(r["filter_province"]),
			"product_id": im.mappedOrZero("product", r["filter_product_id"]),
			"purchased_before": dateS(r["filter_purchased_before"]), "expiry_before": dateS(r["filter_expiry_before"])})
		tpl := S(r["sms_template_key"])
		if tpl == "" {
			tpl = "survey_invite"
		}
		if _, _, err := im.ensure("survey", r["id"], func() (int64, error) {
			return im.one(`INSERT INTO surveys(title,description,status,sms_template,offer_template,link_mode,filter,created_at,closed_at) VALUES($1,$2,$3,$4,$5,$6,$7,COALESCE($8,NOW()),$9) RETURNING id`,
				S(r["title"]), S(r["description"]), status, tpl, S(r["offer_sms_template_key"]), nz(S(r["link_mode"]), "url"), filter, T(r["created_at"]), T(r["closed_at"]))
		}); err != nil {
			return err
		}
	}
	qs, err := query(src, `SELECT * FROM poll_questions ORDER BY poll_id, position, id`)
	if err != nil {
		return err
	}
	for _, r := range qs {
		sv, ok := im.mapGet("survey", r["poll_id"])
		if !ok {
			continue
		}
		var opts []string
		if o := S(r["options"]); o != "" {
			if json.Unmarshal([]byte(o), &opts) != nil {
				opts = strings.Split(o, "\n")
			}
		}
		ob, _ := json.Marshal(opts)
		if opts == nil {
			ob = []byte("[]")
		}
		if _, _, err := im.ensure("survey question", r["id"], func() (int64, error) {
			return im.one(`INSERT INTO survey_questions(survey_id,position,text,kind,options,required) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`,
				sv, I(r["position"]), S(r["text"]), nz(S(r["kind"]), "rating"), ob, B(r["required"]))
		}); err != nil {
			return err
		}
	}
	recs, err := query(src, `SELECT * FROM poll_recipients ORDER BY id`)
	if err != nil {
		return err
	}
	for _, r := range recs {
		sv, ok := im.mapGet("survey", r["poll_id"])
		if !ok {
			continue
		}
		cust := ptrID(r["customer_id"], func(i int64) (int64, bool) { return im.mapGet("customer", i) })
		gua := ptrID(r["guarantee_id"], func(i int64) (int64, bool) { return im.mapGet("guarantee", i) })
		smsStatus := strings.ToLower(S(r["sms_status"]))
		if smsStatus == "" {
			smsStatus = "pending"
		}
		if _, _, err := im.ensure("survey recipient", r["id"], func() (int64, error) {
			return im.one(`INSERT INTO survey_recipients(survey_id,customer_id,guarantee_id,token,customer_name,phone,city,product_name,sms_status,sms_error,sent_at,opened_at,responded_at,offer_sent_at)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING id`,
				sv, cust, gua, S(r["token"]), S(r["customer_name"]), normPhone(S(r["customer_phone"])), S(r["customer_city"]), S(r["product_name"]),
				smsStatus, S(r["sms_error"]), T(r["sent_at"]), T(r["opened_at"]), T(r["responded_at"]), T(r["offer_sent_at"]))
		}); err != nil {
			return err
		}
	}
	ans, err := query(src, `SELECT * FROM poll_answers ORDER BY id`)
	if err != nil {
		return err
	}
	for _, r := range ans {
		rc, ok1 := im.mapGet("survey recipient", r["recipient_id"])
		q, ok2 := im.mapGet("survey question", r["question_id"])
		if !ok1 || !ok2 {
			continue
		}
		var num *int64
		if r["answer_number"] != nil {
			n := I(r["answer_number"])
			num = &n
		}
		if _, _, err := im.ensure("survey answer", r["id"], func() (int64, error) {
			return im.one(`INSERT INTO survey_answers(recipient_id,question_id,answer_text,answer_number) VALUES($1,$2,$3,$4)
				ON CONFLICT (recipient_id,question_id) DO UPDATE SET answer_text=EXCLUDED.answer_text RETURNING id`, rc, q, S(r["answer_text"]), num)
		}); err != nil {
			return err
		}
	}
	return nil
}

func (im *Importer) gSMS(src *sql.DB) error {
	if !tableExists(src, "sms_templates") {
		return nil
	}
	rows, err := query(src, `SELECT * FROM sms_templates`)
	if err != nil {
		return err
	}
	for _, r := range rows {
		// body ids registered in the Melli Payamak panel keep working unchanged
		if err := im.exec(`INSERT INTO sms_templates(key,title,description,body_id,sample_text,is_active,is_builtin) VALUES($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (key) DO UPDATE SET body_id=GREATEST(sms_templates.body_id,EXCLUDED.body_id), sample_text=EXCLUDED.sample_text`,
			S(r["key"]), S(r["title"]), S(r["description"]), I(r["body_id"]), S(r["sample_text"]), B(r["is_active"]), B(r["is_builtin"])); err != nil {
			return err
		}
		im.count("sms template")
	}
	return nil
}

func (im *Importer) mappedOrZero(entity string, v any) int64 {
	if v == nil {
		return 0
	}
	id, _ := im.mapGet(entity, v)
	return id
}

func nz(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
