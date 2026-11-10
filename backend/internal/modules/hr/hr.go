// Package hr: employees, shifts, attendance (kiosk tablets + fingerprint
// readers, offline-safe), leaves, tasks and the attendance reports.
package hr

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
	"github.com/evinki/platform/internal/platform/crud"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
	"github.com/evinki/platform/internal/platform/jalali"
	"github.com/evinki/platform/internal/platform/settings"
)

type mod struct{ *app.Deps }

func Mount(r chi.Router, d *app.Deps) {
	m := &mod{d}
	read, manage, devices, del := auth.PHrRead, auth.PHrManage, auth.PHrDevices, auth.PHrDelete

	(&crud.Resource{Name: "department", Table: "departments", DB: d.DB, Audit: d.Audit,
		Cols:     map[string]crud.Kind{"name": crud.Text, "description": crud.Text, "is_active": crud.Bool},
		Required: []string{"name"}, Order: "name"}).Mount(r, "/hr/departments", crud.Perms{Read: read, Write: manage, Delete: del})

	(&crud.Resource{Name: "shift", Table: "shifts", DB: d.DB, Audit: d.Audit,
		Select: `id,name,to_char(start_time,'HH24:MI') AS start_time,to_char(end_time,'HH24:MI') AS end_time,crosses_midnight,grace_in_minutes,grace_out_minutes,work_days,break_minutes,is_active`,
		Cols: map[string]crud.Kind{"name": crud.Text, "start_time": crud.Text, "end_time": crud.Text, "crosses_midnight": crud.Bool,
			"grace_in_minutes": crud.Int, "grace_out_minutes": crud.Int, "work_days": crud.Text, "break_minutes": crud.Int, "is_active": crud.Bool},
		Required: []string{"name", "start_time", "end_time"}, Order: "name"}).Mount(r, "/hr/shifts", crud.Perms{Read: read, Write: manage, Delete: del})

	(&crud.Resource{Name: "holiday", Table: "holidays", DB: d.DB, Audit: d.Audit,
		Cols:     map[string]crud.Kind{"day": crud.Date, "title": crud.Text, "is_official": crud.Bool},
		Required: []string{"day", "title"}, Order: "day"}).Mount(r, "/hr/holidays", crud.Perms{Read: read, Write: manage, Delete: del})

	(&crud.Resource{Name: "employee", Table: "employees", DB: d.DB, Audit: d.Audit, TouchUpdated: true,
		Select: `employees.id,personnel_code,first_name,last_name,national_code,mobile,position,department_id,shift_id,hire_date,photo,notes,is_active,
			(pin_hash<>'') AS has_pin,
			(SELECT name FROM departments WHERE id=department_id) AS department,(SELECT name FROM shifts WHERE id=shift_id) AS shift,
			EXISTS(SELECT 1 FROM face_embeddings f WHERE f.employee_id=employees.id AND f.is_active) AS face_enrolled,
			EXISTS(SELECT 1 FROM fingerprint_templates f WHERE f.employee_id=employees.id) AS fingerprint_enrolled`,
		Cols: map[string]crud.Kind{"personnel_code": crud.Text, "first_name": crud.Text, "last_name": crud.Text, "national_code": crud.Text,
			"mobile": crud.Text, "position": crud.Text, "department_id": crud.Int, "shift_id": crud.Int, "hire_date": crud.Date,
			"notes": crud.Text, "is_active": crud.Bool, "photo": crud.Text},
		Required: []string{"personnel_code", "first_name", "last_name"}, Search: []string{"first_name", "last_name", "personnel_code", "mobile"},
		Filters: map[string]string{"department_id": "department_id", "active": "is_active"}, Order: "last_name, first_name",
		Check: func(_ *http.Request, _ bool, b crud.Body) error {
			if raw, ok := b["national_code"]; ok && strings.TrimSpace(string(raw)) == `""` {
				b["national_code"] = json.RawMessage("null") // UNIQUE column: empty must be NULL, not ''
			}
			return nil
		}}).Mount(r, "/hr/employees", crud.Perms{Read: read, Write: manage, Delete: del})

	(&crud.Resource{Name: "task", Table: "tasks", DB: d.DB, Audit: d.Audit, TouchUpdated: true,
		Select: `tasks.*, (SELECT first_name||' '||last_name FROM employees WHERE id=employee_id) AS employee_name`,
		Cols: map[string]crud.Kind{"title": crud.Text, "description": crud.Text, "employee_id": crud.Int, "department_id": crud.Int,
			"status": crud.Text, "priority": crud.Text, "recurrence": crud.Text, "due_date": crud.Date, "start_date": crud.Date,
			"end_date": crud.Date, "estimated_minutes": crud.Int, "progress": crud.Int, "is_active": crud.Bool},
		Required: []string{"title"}, Search: []string{"title"}, Filters: map[string]string{"employee_id": "employee_id", "status": "status"},
		Order: "due_date NULLS LAST, id DESC"}).Mount(r, "/hr/tasks", crud.Perms{Read: read, Write: manage, Delete: del})

	r.Group(func(r chi.Router) {
		r.Use(auth.Require(read))
		r.Get("/hr/attendance", httpx.H(m.attendance))
		r.Get("/hr/dashboard", httpx.H(m.dashboard))
		r.Get("/hr/today", httpx.H(m.today))
		r.Get("/hr/employees/{id}/faces", httpx.H(m.listFaces))
		r.Get("/hr/reports/range", httpx.H(m.rangeReport))
		r.Get("/hr/reports/range.csv", httpx.H(m.rangeCSV))
		r.Get("/hr/leaves", httpx.H(m.leaves))
		r.Get("/hr/tasks/{id}/logs", httpx.H(m.taskLogs))
		r.Get("/hr/devices", httpx.H(m.listDevices))
	})
	r.Group(func(r chi.Router) {
		r.Use(auth.Require(manage))
		r.Post("/hr/attendance", httpx.H(m.manualPunch))
		r.Post("/hr/leaves", httpx.H(m.adminLeave))
		r.Post("/hr/leaves/{id}/review", httpx.H(m.reviewLeave))
		r.Post("/hr/employees/{id}/pin", httpx.H(m.setPin))
		r.Post("/hr/employees/{id}/faces", httpx.H(m.addFace))
		r.Delete("/hr/employees/{id}/faces", httpx.H(m.clearFaces))
		r.Delete("/hr/employees/{id}/faces/{faceId}", httpx.H(m.deleteFace))
		r.Post("/hr/tasks/{id}/logs", httpx.H(m.addTaskLog))
	})
	r.With(auth.Require(del)).Delete("/hr/attendance/{id}", httpx.H(m.deletePunch))
	r.Group(func(r chi.Router) {
		r.Use(auth.Require(devices))
		r.Post("/hr/devices", httpx.H(m.createDevice))
		r.Patch("/hr/devices/{id}", httpx.H(m.patchDevice))
		r.Post("/hr/devices/{id}/rotate-key", httpx.H(m.rotateKey))
		r.Post("/hr/employees/{id}/fingerprint-jobs", httpx.H(m.fingerprintJob))
		r.Get("/hr/employees/{id}/fingerprint-jobs", httpx.H(m.fpJobsOf))
		r.Delete("/hr/fingerprint-jobs/{jobId}", httpx.H(m.cancelFpJob))
	})

	// employees ask for leave with their personnel code + PIN, no account needed
	r.Post("/public/leaves", httpx.H(m.publicLeave))

	m.mountKiosk(r)
}

// ───────── attendance ─────────

func (m *mod) attendance(w http.ResponseWriter, r *http.Request) error {
	pg := httpx.Paging(r, 500)
	args := []any{httpx.Q(r, "from"), httpx.Q(r, "to"), qint(r, "employee_id")}
	where := ` WHERE ($1='' OR a.work_date >= $1::date) AND ($2='' OR a.work_date <= $2::date) AND ($3=0 OR a.employee_id=$3)`
	total, err := db.Scalar[int](r.Context(), m.DB, `SELECT COUNT(*) FROM attendance_records a`+where, args...)
	if err != nil {
		return err
	}
	args = append(args, pg.Limit, pg.Offset)
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
	  SELECT a.id,a.employee_id,e.first_name||' '||e.last_name AS employee_name,e.personnel_code,a.kind,a.method,a.happened_at,a.work_date,a.confidence,a.note,
	    a.created_offline,(SELECT name FROM attendance_devices WHERE id=a.device_id) AS device
	  FROM attendance_records a JOIN employees e ON e.id=a.employee_id`+where+` ORDER BY a.happened_at DESC LIMIT $4 OFFSET $5) t`, args...)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"items":` + string(b) + `,"total":` + itoa(total) + `,"page":` + itoa(pg.Page) + `,"limit":` + itoa(pg.Limit) + `}`))
	return nil
}

// workDate decides which working day a punch belongs to. Night shifts that
// cross midnight count their early-morning exit towards the previous day.
func workDate(ctx context.Context, q db.Q, emp int64, kind string, at time.Time) time.Time {
	local := at.In(jalali.Tehran)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	cross, _ := db.Scalar[bool](ctx, q, `SELECT COALESCE((SELECT crosses_midnight FROM shifts s JOIN employees e ON e.shift_id=s.id WHERE e.id=$1),FALSE)`, emp)
	if cross && kind == "out" && local.Hour() < 12 {
		day = day.AddDate(0, 0, -1)
	}
	return day
}

func (m *mod) insertPunch(ctx context.Context, q db.Q, emp int64, device *int64, kind, method string, at time.Time, conf *float64, uuid *string, offline bool, note string, by *int64) (bool, error) {
	tag, err := q.Exec(ctx, `INSERT INTO attendance_records(employee_id,device_id,kind,method,happened_at,work_date,confidence,client_uuid,created_offline,synced_at,note,created_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW(),$10,$11) ON CONFLICT (client_uuid) DO NOTHING`,
		emp, device, kind, method, at, workDate(ctx, q, emp, kind, at), conf, uuid, offline, note, by)
	return err == nil && tag.RowsAffected() == 1, err
}

func (m *mod) manualPunch(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		EmployeeID int64  `json:"employee_id"`
		Kind       string `json:"kind"`
		At         string `json:"happened_at"`
		Note       string `json:"note"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	at, err := time.Parse(time.RFC3339, in.At)
	if err != nil || (in.Kind != "in" && in.Kind != "out") {
		return httpx.BadRequest("نوع تردد و زمان معتبر لازم است")
	}
	me := auth.From(r.Context()).UserID
	if _, err = m.insertPunch(r.Context(), m.DB, in.EmployeeID, nil, in.Kind, "manual", at, nil, nil, false, in.Note, &me); err != nil {
		return err
	}
	m.Audit.Log(r, "attendance.manual", "employee", in.EmployeeID, in.Kind)
	httpx.NoContent(w)
	return nil
}

func (m *mod) deletePunch(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	_, err = m.DB.Exec(r.Context(), `DELETE FROM attendance_records WHERE id=$1`, id)
	m.Audit.Log(r, "attendance.delete", "attendance", id, "")
	httpx.NoContent(w)
	return err
}

// ───────── employee credentials ─────────

func (m *mod) setPin(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		PIN string `json:"pin"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	pin := httpx.Digits(strings.TrimSpace(in.PIN))
	if len(pin) < 4 || len(pin) > 8 {
		return httpx.BadRequest("رمز باید ۴ تا ۸ رقم باشد")
	}
	h, err := auth.HashPassword(pin)
	if err != nil {
		return err
	}
	_, err = m.DB.Exec(r.Context(), `UPDATE employees SET pin_hash=$2, updated_at=NOW() WHERE id=$1`, id, h)
	m.Audit.Log(r, "employee.pin", "employee", id, "")
	httpx.NoContent(w)
	return err
}

func (m *mod) addFace(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Vector    []float64 `json:"vector"`
		ModelName string    `json:"model_name"`
		Quality   *float64  `json:"quality"`
		Image     string    `json:"image"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if len(in.Vector) < 64 || len(in.Vector) > 1024 {
		return httpx.BadRequest("بردار چهره نامعتبر است")
	}
	// stored as sent: the kiosk and the server must compare in the same vector space
	vec, _ := json.Marshal(in.Vector)
	if in.ModelName == "" {
		in.ModelName = "mobilefacenet"
	}
	_, err = m.DB.Exec(r.Context(), `INSERT INTO face_embeddings(employee_id,vector,dim,model_name,quality,image) VALUES($1,$2,$3,$4,$5,$6)`,
		id, string(vec), len(in.Vector), in.ModelName, in.Quality, in.Image)
	if err != nil {
		return err
	}
	m.Audit.Log(r, "employee.face", "employee", id, "")
	httpx.Created(w, map[string]any{"ok": true})
	return nil
}

func (m *mod) clearFaces(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	_, err = m.DB.Exec(r.Context(), `DELETE FROM face_embeddings WHERE employee_id=$1`, id)
	m.Audit.Log(r, "employee.face_clear", "employee", id, "")
	httpx.NoContent(w)
	return err
}

// ───────── tasks ─────────

func (m *mod) taskLogs(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t ORDER BY t.log_date DESC),'[]') FROM (SELECT id,log_date,status,spent_minutes,note FROM task_logs WHERE task_id=$1) t`, id)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
	return nil
}

func (m *mod) addTaskLog(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		LogDate      string `json:"log_date"`
		Status       string `json:"status"`
		SpentMinutes *int   `json:"spent_minutes"`
		Note         string `json:"note"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.LogDate == "" {
		in.LogDate = jalali.Now().Format("2006-01-02")
	}
	if in.Status == "" {
		in.Status = "done"
	}
	me := auth.From(r.Context()).UserID
	_, err = m.DB.Exec(r.Context(), `INSERT INTO task_logs(task_id,log_date,status,spent_minutes,note,recorded_by) VALUES($1,$2::date,$3,$4,$5,$6)
		ON CONFLICT (task_id,log_date) DO UPDATE SET status=EXCLUDED.status,spent_minutes=EXCLUDED.spent_minutes,note=EXCLUDED.note,recorded_by=EXCLUDED.recorded_by`,
		id, httpx.Digits(in.LogDate), in.Status, in.SpentMinutes, in.Note, me)
	httpx.NoContent(w)
	return err
}

// ───────── leaves ─────────

func (m *mod) leaves(w http.ResponseWriter, r *http.Request) error {
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
	  SELECT l.id,l.employee_id,e.first_name||' '||e.last_name AS employee_name,l.leave_type,l.start_at,l.end_at,l.status,l.reason,l.review_note,l.created_at
	  FROM leave_requests l JOIN employees e ON e.id=l.employee_id WHERE ($1='' OR l.status=$1) ORDER BY (l.status='pending') DESC, l.id DESC LIMIT 300) t`, httpx.Q(r, "status"))
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
	return nil
}

type leaveIn struct {
	EmployeeID    int64  `json:"employee_id"`
	PersonnelCode string `json:"personnel_code"`
	PIN           string `json:"pin"`
	LeaveType     string `json:"leave_type"`
	StartAt       string `json:"start_at"`
	EndAt         string `json:"end_at"`
	Reason        string `json:"reason"`
}

func (m *mod) saveLeave(ctx context.Context, in leaveIn, status string, by *int64) (int64, error) {
	s, e1 := time.Parse(time.RFC3339, in.StartAt)
	e, e2 := time.Parse(time.RFC3339, in.EndAt)
	if e1 != nil || e2 != nil || e.Before(s) {
		return 0, httpx.BadRequest("بازه زمانی نامعتبر است")
	}
	if in.LeaveType == "" {
		in.LeaveType = "daily"
	}
	return db.Scalar[int64](ctx, m.DB, `INSERT INTO leave_requests(employee_id,leave_type,start_at,end_at,status,reason,reviewed_by,reviewed_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,CASE WHEN $5<>'pending' THEN NOW() END) RETURNING id`, in.EmployeeID, in.LeaveType, s, e, status, in.Reason, by)
}

func (m *mod) publicLeave(w http.ResponseWriter, r *http.Request) error {
	var in leaveIn
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	emp, err := m.employeeByPIN(r.Context(), in.PersonnelCode, in.PIN)
	if err != nil {
		return err
	}
	in.EmployeeID = emp.ID
	id, err := m.saveLeave(r.Context(), in, "pending", nil)
	if err != nil {
		return err
	}
	httpx.Created(w, map[string]any{"id": id})
	return nil
}

func (m *mod) adminLeave(w http.ResponseWriter, r *http.Request) error {
	var in leaveIn
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	me := auth.From(r.Context()).UserID
	id, err := m.saveLeave(r.Context(), in, "approved", &me)
	if err != nil {
		return err
	}
	m.Audit.Log(r, "leave.create", "leave", id, "")
	httpx.Created(w, map[string]any{"id": id})
	return nil
}

func (m *mod) reviewLeave(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	to := map[string]string{"approve": "approved", "reject": "rejected"}[in.Decision]
	if to == "" {
		return httpx.BadRequest("تصمیم نامعتبر است")
	}
	tag, err := m.DB.Exec(r.Context(), `UPDATE leave_requests SET status=$2, review_note=$3, reviewed_by=$4, reviewed_at=NOW() WHERE id=$1 AND status='pending'`,
		id, to, in.Note, auth.From(r.Context()).UserID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.Conflict("این درخواست قبلاً بررسی شده است")
	}
	m.Audit.Log(r, "leave.review", "leave", id, to)
	httpx.NoContent(w)
	return nil
}

type empAuth struct {
	ID   int64  `db:"id"`
	Name string `db:"name"`
	Hash string `db:"pin_hash"`
}

// employeeByPIN throttles guesses per personnel code through a small DB counter on failed audit rows.
func (m *mod) employeeByPIN(ctx context.Context, code, pin string) (*empAuth, error) {
	bad := httpx.E(401, "invalid_credentials", "کد پرسنلی یا رمز نادرست است")
	code = strings.TrimSpace(httpx.Digits(code))
	fails, _ := db.Scalar[int](ctx, m.DB, `SELECT COUNT(*) FROM audit_log WHERE action='pin.failed' AND entity_id=$1 AND at > NOW()-INTERVAL '10 minutes'`, code)
	if fails >= 8 {
		return nil, httpx.E(429, "too_many_attempts", "تلاش‌های ناموفق زیاد بود؛ چند دقیقه بعد دوباره تلاش کنید")
	}
	e, err := db.One[empAuth](ctx, m.DB, `SELECT id, first_name||' '||last_name AS name, pin_hash FROM employees WHERE personnel_code=$1 AND is_active`, code)
	if err == nil && e.Hash != "" {
		pw := httpx.Digits(strings.TrimSpace(pin))
		var ok bool
		if auth.IsBcrypt(e.Hash) {
			ok, _ = auth.VerifyPassword(e.Hash, "", pw)
		} else {
			ok, _ = auth.VerifyPassword("", e.Hash, pw) // PIN imported from the old attendance system
		}
		if ok {
			return &e, nil
		}
	}
	_, _ = m.DB.Exec(ctx, `INSERT INTO audit_log(action,entity,entity_id,actor_name) VALUES('pin.failed','employee',$1,'kiosk')`, code)
	return nil, bad
}

func qint(r *http.Request, k string) int64 { n, _ := httpx.QInt(r, k); return n }

func itoa(n int) string { return strings_Itoa(n) }

var _ = pgx.ErrNoRows
var _ = settings.Defaults
