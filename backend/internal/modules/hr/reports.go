package hr

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
	"github.com/evinki/platform/internal/platform/jalali"
)

type pgxTx = pgx.Tx

type empRow struct {
	ID       int64  `db:"id"`
	Code     string `db:"personnel_code"`
	Name     string `db:"name"`
	Dept     string `db:"department"`
	ShiftID  *int64 `db:"shift_id"`
	Start    string `db:"start_time"`
	End      string `db:"end_time"`
	Cross    bool   `db:"crosses_midnight"`
	GraceIn  int    `db:"grace_in_minutes"`
	GraceOut int    `db:"grace_out_minutes"`
	WorkDays string `db:"work_days"`
	Break    int    `db:"break_minutes"`
}

type DayRow struct {
	Date        string `json:"date"`
	Jalali      string `json:"jalali"`
	Status      string `json:"status"` // present absent leave mission holiday weekend incomplete
	FirstIn     string `json:"first_in,omitempty"`
	LastOut     string `json:"last_out,omitempty"`
	WorkedMin   int    `json:"worked_minutes"`
	LateMin     int    `json:"late_minutes"`
	EarlyMin    int    `json:"early_leave_minutes"`
	OvertimeMin int    `json:"overtime_minutes"`
}

type EmpReport struct {
	EmployeeID    int64    `json:"employee_id"`
	Name          string   `json:"name"`
	PersonnelCode string   `json:"personnel_code"`
	Department    string   `json:"department"`
	Present       int      `json:"present_days"`
	Absent        int      `json:"absent_days"`
	Leave         int      `json:"leave_days"`
	Incomplete    int      `json:"incomplete_days"`
	WorkedMin     int      `json:"worked_minutes"`
	LateMin       int      `json:"late_minutes"`
	EarlyMin      int      `json:"early_leave_minutes"`
	OvertimeMin   int      `json:"overtime_minutes"`
	LateCount     int      `json:"late_count"`
	Rate          float64  `json:"attendance_rate"` // present / (present+absent+incomplete) * 100
	Days          []DayRow `json:"days,omitempty"`
}

func parseClock(s string) (h, m int) {
	parts := strings.Split(s, ":")
	if len(parts) >= 2 {
		h, _ = strconv.Atoi(parts[0])
		m, _ = strconv.Atoi(parts[1])
	}
	return
}

// build computes the attendance picture for every active employee over [from,to].
func (m *mod) build(r *http.Request, from, to time.Time, withDays bool, onlyEmp int64) ([]EmpReport, error) {
	ctx := r.Context()
	emps, err := db.All[empRow](ctx, m.DB, `SELECT e.id,e.personnel_code,e.first_name||' '||e.last_name AS name,COALESCE(d.name,'') AS department,e.shift_id,
		COALESCE(to_char(s.start_time,'HH24:MI'),'') AS start_time,COALESCE(to_char(s.end_time,'HH24:MI'),'') AS end_time,
		COALESCE(s.crosses_midnight,FALSE) AS crosses_midnight,COALESCE(s.grace_in_minutes,0) AS grace_in_minutes,COALESCE(s.grace_out_minutes,0) AS grace_out_minutes,
		COALESCE(s.work_days,'0,1,2,3,4,5') AS work_days,COALESCE(s.break_minutes,0) AS break_minutes
		FROM employees e LEFT JOIN shifts s ON s.id=e.shift_id LEFT JOIN departments d ON d.id=e.department_id
		WHERE e.is_active AND ($1=0 OR e.id=$1) ORDER BY e.last_name,e.first_name`, onlyEmp)
	if err != nil {
		return nil, err
	}
	type punch struct {
		Emp  int64     `db:"employee_id"`
		Day  time.Time `db:"work_date"`
		Kind string    `db:"kind"`
		At   time.Time `db:"happened_at"`
	}
	punches, err := db.All[punch](ctx, m.DB, `SELECT employee_id,work_date,kind,happened_at FROM attendance_records WHERE work_date BETWEEN $1 AND $2 ORDER BY happened_at`, from, to)
	if err != nil {
		return nil, err
	}
	type key struct {
		e int64
		d string
	}
	byDay := map[key][]punch{}
	for _, p := range punches {
		k := key{p.Emp, p.Day.Format("2006-01-02")}
		byDay[k] = append(byDay[k], p)
	}
	leaves, err := db.All[leaveRow](ctx, m.DB, `SELECT employee_id,leave_type,start_at,end_at FROM leave_requests WHERE status='approved' AND end_at >= $1 AND start_at < ($2::date + 1)`, from, to)
	if err != nil {
		return nil, err
	}
	hol, err := db.All[struct {
		Day time.Time `db:"day"`
	}](ctx, m.DB, `SELECT day FROM holidays WHERE day BETWEEN $1 AND $2`, from, to)
	if err != nil {
		return nil, err
	}
	holidays := map[string]bool{}
	for _, h := range hol {
		holidays[h.Day.Format("2006-01-02")] = true
	}

	today := jalali.StartOfDay(time.Now())
	out := make([]EmpReport, 0, len(emps))
	for _, e := range emps {
		rep := EmpReport{EmployeeID: e.ID, Name: e.Name, PersonnelCode: e.Code, Department: e.Dept}
		workDays := map[int]bool{}
		for _, p := range strings.Split(e.WorkDays, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
				workDays[n] = true
			}
		}
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			ds := d.Format("2006-01-02")
			local := time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, jalali.Tehran)
			row := DayRow{Date: ds, Jalali: jalali.Format(local), Status: ""}
			ps := byDay[key{e.ID, ds}]
			switch {
			case holidays[ds]:
				row.Status = "holiday"
			case !workDays[jalali.IranWeekday(local)]:
				row.Status = "weekend"
			}
			if len(ps) == 0 && row.Status == "" {
				row.Status = leaveStatus(leaves, e.ID, local)
				if row.Status == "" {
					if local.After(today.Add(24 * time.Hour)) {
						row.Status = "" // future
					} else if jalali.StartOfDay(local).Equal(today) {
						row.Status = "pending" // the day is not over yet
					} else {
						row.Status = "absent"
					}
				}
			} else if len(ps) > 0 {
				var first, last *time.Time
				for i := range ps {
					t := ps[i].At
					if ps[i].Kind == "in" && first == nil {
						first = &t
					}
					if ps[i].Kind == "out" {
						last = &t
					}
				}
				if first != nil {
					row.FirstIn = first.In(jalali.Tehran).Format("15:04")
				}
				if last != nil {
					row.LastOut = last.In(jalali.Tehran).Format("15:04")
				}
				if first == nil || last == nil || !last.After(*first) {
					row.Status = "incomplete"
				} else {
					row.Status = "present"
					row.WorkedMin = max(0, int(last.Sub(*first).Minutes())-e.Break)
					if e.ShiftID != nil {
						sh, sm := parseClock(e.Start)
						eh, em := parseClock(e.End)
						start := time.Date(local.Year(), local.Month(), local.Day(), sh, sm, 0, 0, jalali.Tehran)
						end := time.Date(local.Year(), local.Month(), local.Day(), eh, em, 0, 0, jalali.Tehran)
						if e.Cross || !end.After(start) {
							end = end.AddDate(0, 0, 1)
						}
						row.LateMin = max(0, int(first.Sub(start.Add(time.Duration(e.GraceIn)*time.Minute)).Minutes()))
						row.EarlyMin = max(0, int(end.Add(-time.Duration(e.GraceOut)*time.Minute).Sub(*last).Minutes()))
						expected := max(0, int(end.Sub(start).Minutes())-e.Break)
						row.OvertimeMin = max(0, row.WorkedMin-expected)
					}
				}
			}
			switch row.Status {
			case "present":
				rep.Present++
			case "absent":
				rep.Absent++
			case "leave", "mission":
				rep.Leave++
			case "incomplete":
				rep.Incomplete++
			}
			rep.WorkedMin += row.WorkedMin
			rep.LateMin += row.LateMin
			rep.EarlyMin += row.EarlyMin
			rep.OvertimeMin += row.OvertimeMin
			if row.LateMin > 0 {
				rep.LateCount++
			}
			if withDays && row.Status != "" {
				rep.Days = append(rep.Days, row)
			}
		}
		if due := rep.Present + rep.Absent + rep.Incomplete; due > 0 {
			rep.Rate = float64(int(float64(rep.Present)/float64(due)*1000+0.5)) / 10
		}
		out = append(out, rep)
	}
	return out, nil
}

type leaveRow struct {
	Emp   int64     `db:"employee_id"`
	Type  string    `db:"leave_type"`
	Start time.Time `db:"start_at"`
	End   time.Time `db:"end_at"`
}

func leaveStatus(ls []leaveRow, emp int64, day time.Time) string {
	d0 := jalali.StartOfDay(day)
	d1 := d0.Add(24 * time.Hour)
	for _, l := range ls {
		if l.Emp == emp && l.Start.Before(d1) && !l.End.Before(d0) {
			if l.Type == "mission" {
				return "mission"
			}
			return "leave"
		}
	}
	return ""
}

func (m *mod) rangeParams(r *http.Request) (time.Time, time.Time, error) {
	parse := func(k string, def time.Time) (time.Time, error) {
		s := httpx.Digits(httpx.Q(r, k))
		if s == "" {
			return def, nil
		}
		return time.Parse("2006-01-02", s)
	}
	now := jalali.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	from, e1 := parse("from", today.AddDate(0, 0, -29))
	to, e2 := parse("to", today)
	if e1 != nil || e2 != nil || to.Before(from) || to.Sub(from) > 366*24*time.Hour {
		return from, to, httpx.BadRequest("بازه تاریخ نامعتبر است (حداکثر یک سال)")
	}
	return from, to, nil
}

func (m *mod) rangeReport(w http.ResponseWriter, r *http.Request) error {
	from, to, err := m.rangeParams(r)
	if err != nil {
		return err
	}
	wantDays := httpx.Q(r, "days") == "1" || httpx.Q(r, "employee_id") != ""
	rep, err := m.build(r, from, to, true, qint(r, "employee_id"))
	if err != nil {
		return err
	}
	sort.SliceStable(rep, func(i, j int) bool { return rep[i].Name < rep[j].Name })
	series, arrivals := daySeries(rep)
	depts := deptSummary(rep)
	totals := totalsOf(rep)
	if !wantDays {
		for i := range rep {
			rep[i].Days = nil
		}
	}
	httpx.OK(w, map[string]any{"from": from.Format("2006-01-02"), "to": to.Format("2006-01-02"), "employees": rep,
		"series": series, "arrivals": arrivals, "departments": depts, "totals": totals})
	return nil
}

func (m *mod) rangeCSV(w http.ResponseWriter, r *http.Request) error {
	from, to, err := m.rangeParams(r)
	if err != nil {
		return err
	}
	detail := httpx.Q(r, "detail") == "1"
	rep, err := m.build(r, from, to, detail, 0)
	if err != nil {
		return err
	}
	if detail {
		return writeDetailCSV(w, rep)
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="attendance.csv"`)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"کد پرسنلی", "نام", "واحد", "روز حضور", "غیبت", "مرخصی/مأموریت", "ناقص", "ساعت کار", "تأخیر (دقیقه)", "تعجیل (دقیقه)", "اضافه‌کار (دقیقه)"})
	for _, e := range rep {
		_ = cw.Write([]string{e.PersonnelCode, e.Name, e.Department, strconv.Itoa(e.Present), strconv.Itoa(e.Absent), strconv.Itoa(e.Leave),
			strconv.Itoa(e.Incomplete), fmt.Sprintf("%.1f", float64(e.WorkedMin)/60), strconv.Itoa(e.LateMin), strconv.Itoa(e.EarlyMin), strconv.Itoa(e.OvertimeMin)})
	}
	cw.Flush()
	return nil
}

