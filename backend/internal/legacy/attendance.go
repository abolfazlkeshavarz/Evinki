package legacy

import (
	"database/sql"
	"strings"
)

// ImportAttendance loads the attendance-system database (SQLite file or Postgres).
// Kiosk API keys from the old system stay valid when the server is started with
// LEGACY_DEVICE_KEY_SECRET set to the old SECRET_KEY, so tablets and readers
// need no re-registration.
func (im *Importer) ImportAttendance(src *sql.DB) error {
	im.Source = "attendance"
	return im.Run(im.ctxOrBackground(), func() error {
		im.logf("importing users ...")
		users, err := query(src, `SELECT * FROM users`)
		if err != nil {
			return err
		}
		for _, r := range users {
			role := map[string]string{"admin": "admin", "manager": "hr_manager", "viewer": "viewer"}[S(r["role"])]
			if role == "" {
				role = "viewer"
			}
			if _, _, err := im.ensure("user", r["id"], func() (int64, error) {
				return im.upsertUser(S(r["username"]), S(r["full_name"]), "", "", S(r["hashed_password"]), B(r["is_active"]), role)
			}); err != nil {
				return err
			}
		}
		for _, d := range mustRows(query(src, `SELECT * FROM departments`)) {
			if _, _, err := im.ensure("department", d["id"], func() (int64, error) {
				return im.one(`INSERT INTO departments(name,description,is_active) VALUES($1,$2,$3) ON CONFLICT (name) DO UPDATE SET description=EXCLUDED.description RETURNING id`, S(d["name"]), S(d["description"]), B(d["is_active"]))
			}); err != nil {
				return err
			}
		}
		for _, s := range mustRows(query(src, `SELECT * FROM shifts`)) {
			if _, _, err := im.ensure("shift", s["id"], func() (int64, error) {
				return im.one(`INSERT INTO shifts(name,start_time,end_time,crosses_midnight,grace_in_minutes,grace_out_minutes,work_days,break_minutes,is_active)
					VALUES($1,$2::time,$3::time,$4,$5,$6,$7,$8,$9) ON CONFLICT (name) DO UPDATE SET name=EXCLUDED.name RETURNING id`,
					S(s["name"]), clock(S(s["start_time"])), clock(S(s["end_time"])), B(s["crosses_midnight"]), I(s["grace_in_minutes"]), I(s["grace_out_minutes"]),
					nz(S(s["work_days"]), "0,1,2,3,4,5"), I(s["break_minutes"]), B(s["is_active"]))
			}); err != nil {
				return err
			}
		}
		for _, h := range mustRows(query(src, `SELECT * FROM holidays`)) {
			if err := im.exec(`INSERT INTO holidays(day,title,is_official) VALUES($1::date,$2,$3) ON CONFLICT (day) DO NOTHING`, dateS(h["day"]), S(h["title"]), B(h["is_official"])); err != nil {
				return err
			}
		}

		im.logf("importing employees ...")
		emps, err := query(src, `SELECT * FROM employees`)
		if err != nil {
			return err
		}
		for _, e := range emps {
			dept := ptrID(e["department_id"], func(i int64) (int64, bool) { return im.mapGet("department", i) })
			shift := ptrID(e["shift_id"], func(i int64) (int64, bool) { return im.mapGet("shift", i) })
			nat := strings.TrimSpace(S(e["national_code"]))
			var natp *string
			if nat != "" {
				natp = &nat
			}
			if _, _, err := im.ensure("employee", e["id"], func() (int64, error) {
				// the legacy PIN hash (pbkdf2) is verified as it is; nobody has to re-enter a PIN
				return im.one(`INSERT INTO employees(personnel_code,first_name,last_name,national_code,mobile,position,department_id,shift_id,hire_date,pin_hash,notes,is_active)
					VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::date,$10,$11,$12) RETURNING id`,
					S(e["personnel_code"]), S(e["first_name"]), S(e["last_name"]), natp, normPhone(S(e["mobile"])), S(e["position"]), dept, shift,
					dateS(e["hire_date"]), S(e["pin_hash"]), S(e["notes"]), B(e["is_active"]))
			}); err != nil {
				return err
			}
		}
		for _, f := range mustRows(query(src, `SELECT * FROM face_embeddings`)) {
			emp, ok := im.mapGet("employee", f["employee_id"])
			if !ok {
				continue
			}
			if _, _, err := im.ensure("face", f["id"], func() (int64, error) {
				return im.one(`INSERT INTO face_embeddings(employee_id,vector,dim,model_name,quality,is_active) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`,
					emp, S(f["vector"]), I(f["dim"]), S(f["model_name"]), F(f["quality"]), B(f["is_active"]))
			}); err != nil {
				return err
			}
		}
		for _, d := range mustRows(query(src, `SELECT * FROM devices`)) {
			if _, _, err := im.ensure("device", d["id"], func() (int64, error) {
				return im.one(`INSERT INTO attendance_devices(name,kind,device_uid,api_key_hash,location,last_seen_at,app_version,is_active) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
					S(d["name"]), nz(S(d["kind"]), "tablet"), S(d["device_uid"]), S(d["api_key_hash"]), S(d["location"]), T(d["last_seen_at"]), S(d["app_version"]), B(d["is_active"]))
			}); err != nil {
				return err
			}
		}

		im.logf("importing attendance records ...")
		recs, err := query(src, `SELECT * FROM attendance_records ORDER BY id`)
		if err != nil {
			return err
		}
		for _, a := range recs {
			emp, ok := im.mapGet("employee", a["employee_id"])
			at := T(a["happened_at"])
			if !ok || at == nil {
				continue
			}
			dev := ptrID(a["device_id"], func(i int64) (int64, bool) { return im.mapGet("device", i) })
			method := S(a["method"])
			switch method {
			case "face", "fingerprint", "pin", "manual", "admin_fix":
			default:
				method = "manual"
			}
			if _, _, err := im.ensure("punch", a["id"], func() (int64, error) {
				return im.one(`INSERT INTO attendance_records(employee_id,device_id,kind,method,happened_at,work_date,confidence,client_uuid,created_offline,note)
					VALUES($1,$2,$3,$4,$5,$6::date,$7,$8,$9,$10) ON CONFLICT (client_uuid) DO UPDATE SET note=attendance_records.note RETURNING id`,
					emp, dev, S(a["kind"]), method, at, dateS(a["work_date"]), F(a["confidence"]), NullS(a["client_uuid"]), B(a["created_offline"]), S(a["note"]))
			}); err != nil {
				return err
			}
		}
		for _, l := range mustRows(query(src, `SELECT * FROM leave_requests`)) {
			emp, ok := im.mapGet("employee", l["employee_id"])
			s, e := T(l["start_at"]), T(l["end_at"])
			if !ok || s == nil || e == nil {
				continue
			}
			if _, _, err := im.ensure("leave", l["id"], func() (int64, error) {
				return im.one(`INSERT INTO leave_requests(employee_id,leave_type,start_at,end_at,status,reason,review_note,reviewed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
					emp, S(l["leave_type"]), s, e, S(l["status"]), S(l["reason"]), S(l["review_note"]), T(l["reviewed_at"]))
			}); err != nil {
				return err
			}
		}
		for _, t := range mustRows(query(src, `SELECT * FROM tasks`)) {
			emp := ptrID(t["employee_id"], func(i int64) (int64, bool) { return im.mapGet("employee", i) })
			dept := ptrID(t["department_id"], func(i int64) (int64, bool) { return im.mapGet("department", i) })
			if _, _, err := im.ensure("task", t["id"], func() (int64, error) {
				return im.one(`INSERT INTO tasks(title,description,employee_id,department_id,status,priority,recurrence,due_date,start_date,end_date,estimated_minutes,progress,completed_at,is_active)
					VALUES($1,$2,$3,$4,$5,$6,$7,$8::date,$9::date,$10::date,$11,$12,$13,$14) RETURNING id`,
					S(t["title"]), S(t["description"]), emp, dept, S(t["status"]), S(t["priority"]), S(t["recurrence"]), dateS(t["due_date"]), dateS(t["start_date"]),
					dateS(t["end_date"]), nullInt(t["estimated_minutes"]), I(t["progress"]), T(t["completed_at"]), B(t["is_active"]))
			}); err != nil {
				return err
			}
		}
		for _, l := range mustRows(query(src, `SELECT * FROM task_logs`)) {
			task, ok := im.mapGet("task", l["task_id"])
			if !ok {
				continue
			}
			if _, _, err := im.ensure("task log", l["id"], func() (int64, error) {
				return im.one(`INSERT INTO task_logs(task_id,log_date,status,spent_minutes,note) VALUES($1,$2::date,$3,$4,$5)
					ON CONFLICT (task_id,log_date) DO UPDATE SET note=EXCLUDED.note RETURNING id`, task, dateS(l["log_date"]), S(l["status"]), nullInt(l["spent_minutes"]), S(l["note"]))
			}); err != nil {
				return err
			}
		}
		for _, f := range mustRows(query(src, `SELECT * FROM fingerprint_templates`)) {
			emp, ok := im.mapGet("employee", f["employee_id"])
			if !ok {
				continue
			}
			b, _ := f["template_data"].([]byte)
			if err := im.exec(`INSERT INTO fingerprint_templates(employee_id,template_data,model_name) VALUES($1,$2,$3) ON CONFLICT (employee_id) DO NOTHING`, emp, b, S(f["model_name"])); err != nil {
				return err
			}
		}
		for _, f := range mustRows(query(src, `SELECT * FROM fingerprint_slots`)) {
			emp, ok1 := im.mapGet("employee", f["employee_id"])
			dev, ok2 := im.mapGet("device", f["device_id"])
			if !ok1 || !ok2 {
				continue
			}
			if err := im.exec(`INSERT INTO fingerprint_slots(device_id,employee_id,slot_id,synced_at) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, dev, emp, I(f["slot_id"]), T(f["synced_at"])); err != nil {
				return err
			}
		}
		return nil
	})
}

// clock normalises "08:00:00", "08:00" or a full timestamp to HH:MM:SS.
func clock(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " T"); i >= 0 {
		s = s[i+1:]
	}
	if len(s) >= 8 {
		return s[:8]
	}
	if len(s) == 5 {
		return s + ":00"
	}
	return s
}
