package hr

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
	"github.com/evinki/platform/internal/platform/jalali"
)

// SeriesPoint is one calendar day summed over every employee (for the trend graphs).
type SeriesPoint struct {
	Date       string `json:"date"`
	Jalali     string `json:"jalali"`
	Present    int    `json:"present"`
	Absent     int    `json:"absent"`
	Leave      int    `json:"leave"`
	Incomplete int    `json:"incomplete"`
	Late       int    `json:"late"`
	WorkedMin  int    `json:"worked_minutes"`
	Overtime   int    `json:"overtime_minutes"`
}

// daySeries sums the per-day rows of all employees and also counts first arrivals per hour of the day.
func daySeries(rep []EmpReport) ([]SeriesPoint, [24]int) {
	idx := map[string]*SeriesPoint{}
	var arrivals [24]int
	for _, e := range rep {
		for _, d := range e.Days {
			p := idx[d.Date]
			if p == nil {
				p = &SeriesPoint{Date: d.Date, Jalali: d.Jalali}
				idx[d.Date] = p
			}
			switch d.Status {
			case "present":
				p.Present++
			case "absent":
				p.Absent++
			case "leave", "mission":
				p.Leave++
			case "incomplete":
				p.Incomplete++
			}
			if d.LateMin > 0 {
				p.Late++
			}
			p.WorkedMin += d.WorkedMin
			p.Overtime += d.OvertimeMin
			if len(d.FirstIn) >= 2 {
				if h, err := strconv.Atoi(d.FirstIn[:2]); err == nil && h >= 0 && h < 24 {
					arrivals[h]++
				}
			}
		}
	}
	out := make([]SeriesPoint, 0, len(idx))
	for _, p := range idx {
		if p.Present+p.Absent+p.Leave+p.Incomplete > 0 { // weekends and holidays would only draw a misleading dip
			out = append(out, *p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, arrivals
}

type DeptSummary struct {
	Name      string  `json:"name"`
	Employees int     `json:"employees"`
	Present   int     `json:"present_days"`
	Absent    int     `json:"absent_days"`
	Leave     int     `json:"leave_days"`
	LateMin   int     `json:"late_minutes"`
	WorkedMin int     `json:"worked_minutes"`
	Overtime  int     `json:"overtime_minutes"`
	Rate      float64 `json:"attendance_rate"`
}

func deptSummary(rep []EmpReport) []DeptSummary {
	idx := map[string]*DeptSummary{}
	for _, e := range rep {
		name := e.Department
		if name == "" {
			name = "بدون واحد"
		}
		d := idx[name]
		if d == nil {
			d = &DeptSummary{Name: name}
			idx[name] = d
		}
		d.Employees++
		d.Present += e.Present
		d.Absent += e.Absent
		d.Leave += e.Leave
		d.LateMin += e.LateMin
		d.WorkedMin += e.WorkedMin
		d.Overtime += e.OvertimeMin
	}
	out := make([]DeptSummary, 0, len(idx))
	for _, d := range idx {
		if due := d.Present + d.Absent; due > 0 {
			d.Rate = float64(int(float64(d.Present)/float64(due)*1000+0.5)) / 10
		}
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func totalsOf(rep []EmpReport) map[string]any {
	var present, absent, leave, inc, worked, late, lateCnt, over, early int
	for _, e := range rep {
		present += e.Present
		absent += e.Absent
		leave += e.Leave
		inc += e.Incomplete
		worked += e.WorkedMin
		late += e.LateMin
		lateCnt += e.LateCount
		over += e.OvertimeMin
		early += e.EarlyMin
	}
	rate := 0.0
	if due := present + absent + inc; due > 0 {
		rate = float64(int(float64(present)/float64(due)*1000+0.5)) / 10
	}
	return map[string]any{"employees": len(rep), "present_days": present, "absent_days": absent, "leave_days": leave, "incomplete_days": inc,
		"worked_minutes": worked, "late_minutes": late, "late_count": lateCnt, "overtime_minutes": over, "early_leave_minutes": early, "attendance_rate": rate}
}

func writeDetailCSV(w http.ResponseWriter, rep []EmpReport) error {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="attendance-daily.csv"`)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"کد پرسنلی", "نام", "واحد", "تاریخ", "وضعیت", "ورود", "خروج", "کارکرد (دقیقه)", "تأخیر (دقیقه)", "تعجیل (دقیقه)", "اضافه‌کار (دقیقه)"})
	for _, e := range rep {
		for _, d := range e.Days {
			_ = cw.Write([]string{e.PersonnelCode, e.Name, e.Department, d.Jalali, d.Status, d.FirstIn, d.LastOut,
				strconv.Itoa(d.WorkedMin), strconv.Itoa(d.LateMin), strconv.Itoa(d.EarlyMin), strconv.Itoa(d.OvertimeMin)})
		}
	}
	cw.Flush()
	return nil
}

// ───────── today & dashboard ─────────

type todayRow struct {
	EmployeeID    int64  `json:"employee_id"`
	Name          string `json:"name"`
	PersonnelCode string `json:"personnel_code"`
	Department    string `json:"department"`
	Photo         string `json:"photo"`
	Status        string `json:"status"`
	FirstIn       string `json:"first_in"`
	LastOut       string `json:"last_out"`
	Inside        bool   `json:"inside"`
	WorkedMin     int    `json:"worked_minutes"`
	LateMin       int    `json:"late_minutes"`
}

// todayStatus is every active employee's situation right now.
func (m *mod) todayStatus(r *http.Request) ([]todayRow, error) {
	now := jalali.Now()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	rep, err := m.build(r, day, day, true, 0)
	if err != nil {
		return nil, err
	}
	type info struct {
		ID    int64  `db:"employee_id"`
		Photo string `db:"photo"`
		In    bool   `db:"inside"`
	}
	infos, err := db.All[info](r.Context(), m.DB, `SELECT e.id AS employee_id, COALESCE(e.photo,'') AS photo,
		COALESCE((SELECT kind='in' FROM attendance_records a WHERE a.employee_id=e.id AND a.work_date=$1 ORDER BY a.happened_at DESC LIMIT 1),FALSE) AS inside
		FROM employees e WHERE e.is_active`, day)
	if err != nil {
		return nil, err
	}
	byID := map[int64]info{}
	for _, i := range infos {
		byID[i.ID] = i
	}
	out := make([]todayRow, 0, len(rep))
	for _, e := range rep {
		row := todayRow{EmployeeID: e.EmployeeID, Name: e.Name, PersonnelCode: e.PersonnelCode, Department: e.Department,
			Photo: byID[e.EmployeeID].Photo, Inside: byID[e.EmployeeID].In, Status: "pending"}
		if len(e.Days) > 0 {
			d := e.Days[0]
			row.Status, row.FirstIn, row.LastOut, row.WorkedMin, row.LateMin = d.Status, d.FirstIn, d.LastOut, d.WorkedMin, d.LateMin
		}
		if row.Inside {
			row.Status = "present"
			if row.FirstIn != "" {
				if h, mi := parseClock(row.FirstIn); true {
					start := time.Date(now.Year(), now.Month(), now.Day(), h, mi, 0, 0, jalali.Tehran)
					row.WorkedMin = max(0, int(now.Sub(start).Minutes()))
				}
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func (m *mod) today(w http.ResponseWriter, r *http.Request) error {
	rows, err := m.todayStatus(r)
	if err != nil {
		return err
	}
	httpx.OK(w, rows)
	return nil
}

func (m *mod) dashboard(w http.ResponseWriter, r *http.Request) error {
	rows, err := m.todayStatus(r)
	if err != nil {
		return err
	}
	counters := map[string]int{}
	inside, late := 0, 0
	deptCount := map[string]int{}
	for _, t := range rows {
		counters[t.Status]++
		if t.Inside {
			inside++
		}
		if t.LateMin > 0 {
			late++
		}
		n := t.Department
		if n == "" {
			n = "بدون واحد"
		}
		deptCount[n]++
	}
	// last 7 days for the trend
	now := jalali.Now()
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	trendRep, err := m.build(r, to.AddDate(0, 0, -6), to, true, 0)
	if err != nil {
		return err
	}
	trend, arrivals := daySeries(trendRep)
	depts := make([]map[string]any, 0, len(deptCount))
	for k, v := range deptCount {
		depts = append(depts, map[string]any{"name": k, "count": v})
	}
	sort.Slice(depts, func(i, j int) bool { return depts[i]["name"].(string) < depts[j]["name"].(string) })

	extra, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT row_to_json(t) FROM (SELECT
	  (SELECT COUNT(*) FROM employees WHERE is_active)::int AS employees,
	  (SELECT COUNT(*) FROM leave_requests WHERE status='pending')::int AS pending_leaves,
	  (SELECT COUNT(*) FROM attendance_devices WHERE is_active AND (last_seen_at IS NULL OR last_seen_at < NOW()-INTERVAL '10 minutes'))::int AS devices_offline,
	  (SELECT COUNT(*) FROM attendance_devices WHERE is_active)::int AS devices,
	  (SELECT COUNT(*) FROM tasks WHERE is_active AND status IN ('todo','in_progress'))::int AS open_tasks,
	  (SELECT COUNT(*) FROM tasks WHERE is_active AND status IN ('todo','in_progress') AND due_date < CURRENT_DATE)::int AS overdue_tasks,
	  (SELECT COUNT(*) FROM employees e WHERE e.is_active AND NOT EXISTS (SELECT 1 FROM face_embeddings f WHERE f.employee_id=e.id AND f.is_active))::int AS face_not_enrolled) t`)
	if err != nil {
		return err
	}
	var x map[string]any
	_ = json.Unmarshal(extra, &x)
	x["date"] = jalali.Format(now)
	x["counters"] = counters
	x["inside_now"] = inside
	x["late_today"] = late
	x["arrived_today"] = counters["present"] + counters["incomplete"]
	x["trend"] = trend
	x["arrivals"] = arrivals
	x["departments"] = depts
	httpx.OK(w, x)
	return nil
}

// ───────── face samples ─────────

func (m *mod) listFaces(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t ORDER BY t.id),'[]') FROM (
	  SELECT id,dim,model_name,quality,image,created_at FROM face_embeddings WHERE employee_id=$1 AND is_active) t`, id)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
	return nil
}

func (m *mod) deleteFace(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	fid, err := httpx.ID(r, "faceId")
	if err != nil {
		return err
	}
	if _, err = m.DB.Exec(r.Context(), `DELETE FROM face_embeddings WHERE id=$1 AND employee_id=$2`, fid, id); err != nil {
		return err
	}
	m.Audit.Log(r, "employee.face_delete", "employee", id, "")
	httpx.NoContent(w)
	return nil
}

// fpJobsOf lists the latest fingerprint enrolment jobs of one employee (for the progress dialog).
func (m *mod) fpJobsOf(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
	  SELECT j.id,j.status,COALESCE(j.error,'') AS error,j.created_at,j.completed_at,(SELECT name FROM attendance_devices WHERE id=j.device_id) AS device
	  FROM fingerprint_enroll_jobs j WHERE j.employee_id=$1 ORDER BY j.id DESC LIMIT 5) t`, id)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
	return nil
}

func (m *mod) cancelFpJob(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "jobId")
	if err != nil {
		return err
	}
	_, err = m.DB.Exec(r.Context(), `UPDATE fingerprint_enroll_jobs SET status='cancelled',completed_at=NOW() WHERE id=$1 AND status='pending'`, id)
	httpx.NoContent(w)
	return err
}
