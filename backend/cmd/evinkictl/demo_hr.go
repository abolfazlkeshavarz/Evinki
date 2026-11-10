package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/evinki/platform/internal/app"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/jalali"
)

// seedHRDemo fills the attendance module with 12 fictitious employees and 45 days of punches, so the
// dashboard and graphs can be shown. Demo people carry personnel codes D1001..; remove them with
// "evinkictl seed-hr-demo -clear" (not needed in production).
func seedHRDemo(ctx context.Context, d *app.Deps, clear bool) error {
	if clear {
		_, err := d.DB.Exec(ctx, `DELETE FROM employees WHERE personnel_code LIKE 'D1%' AND notes='demo'`)
		fmt.Println("demo employees removed")
		return err
	}
	if err := seedDemo(ctx, d); err != nil { // departments + shift
		return err
	}
	if n, _ := db.Scalar[int](ctx, d.DB, `SELECT COUNT(*) FROM employees WHERE notes='demo'`); n > 0 {
		fmt.Println("demo employees already exist")
		return nil
	}
	shift, _ := db.Scalar[int64](ctx, d.DB, `SELECT id FROM shifts ORDER BY id LIMIT 1`)
	deps, _ := db.All[struct {
		ID int64 `db:"id"`
	}](ctx, d.DB, `SELECT id FROM departments ORDER BY id`)
	names := [][2]string{{"علی", "رضایی"}, {"مریم", "احمدی"}, {"حسین", "کریمی"}, {"زهرا", "محمدی"}, {"رضا", "حسینی"}, {"فاطمه", "موسوی"},
		{"محمد", "جعفری"}, {"سارا", "نوری"}, {"امیر", "صادقی"}, {"نرگس", "قاسمی"}, {"مهدی", "کاظمی"}, {"لیلا", "یوسفی"}}
	rng := rand.New(rand.NewSource(7))
	today := jalali.StartOfDay(time.Now())
	for i, n := range names {
		dep := deps[i%len(deps)].ID
		id, err := db.Scalar[int64](ctx, d.DB, `INSERT INTO employees(personnel_code,first_name,last_name,department_id,shift_id,position,notes) VALUES($1,$2,$3,$4,$5,'کارمند','demo') RETURNING id`,
			fmt.Sprintf("D%d", 1001+i), n[0], n[1], dep, shift)
		if err != nil {
			return err
		}
		lateness := rng.Float64() * 0.35 // some people are habitually late
		absence := rng.Float64() * 0.12
		for back := 45; back >= 0; back-- {
			day := today.AddDate(0, 0, -back)
			wd := jalali.IranWeekday(day)
			if wd == 6 { // Friday is the weekly holiday (Thursday is a working day in the default shift)
				continue
			}
			if rng.Float64() < absence {
				continue
			}
			in := time.Date(day.Year(), day.Month(), day.Day(), 7, 50, 0, 0, jalali.Tehran).Add(time.Duration(rng.Intn(25)) * time.Minute)
			if rng.Float64() < lateness {
				in = in.Add(time.Duration(15+rng.Intn(50)) * time.Minute)
			}
			out := time.Date(day.Year(), day.Month(), day.Day(), 16, 30, 0, 0, jalali.Tehran).Add(time.Duration(rng.Intn(70)-10) * time.Minute)
			if back == 0 && time.Now().Before(out) {
				out = time.Time{} // still inside today
			}
			if back == 0 && time.Now().Before(in) {
				continue
			}
			wdate := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
			if _, err := d.DB.Exec(ctx, `INSERT INTO attendance_records(employee_id,kind,method,happened_at,work_date,confidence,client_uuid) VALUES($1,'in','face',$2,$3,0.8,$4)`, id, in, wdate, fmt.Sprintf("demo-%d-%d-i", id, back)); err != nil {
				return err
			}
			if !out.IsZero() {
				if _, err := d.DB.Exec(ctx, `INSERT INTO attendance_records(employee_id,kind,method,happened_at,work_date,confidence,client_uuid) VALUES($1,'out','face',$2,$3,0.8,$4)`, id, out, wdate, fmt.Sprintf("demo-%d-%d-o", id, back)); err != nil {
					return err
				}
			}
		}
	}
	_, _ = d.DB.Exec(ctx, `INSERT INTO leave_requests(employee_id,leave_type,start_at,end_at,status,reason) SELECT id,'daily',NOW()+INTERVAL '2 days',NOW()+INTERVAL '3 days','pending','کار شخصی' FROM employees WHERE personnel_code='D1003'`)
	fmt.Println("demo employees and 45 days of attendance created (personnel codes D1001..D1012)")
	return nil
}
