package main

import (
	"context"
	"fmt"

	"github.com/evinki/platform/internal/app"
	"github.com/evinki/platform/internal/platform/db"
)

// seedDemo prepares a fresh install so it is not an empty page: the real catalogue of evinki.com
// (products + photos), service items, a shift, departments and the standard survey. Safe to re-run.
func seedDemo(ctx context.Context, d *app.Deps) error {
	if err := seedCatalog(ctx, d); err != nil {
		return err
	}
	for _, s := range []struct {
		name string
		fee  int64
	}{{"تعویض فیلتر", 80000}, {"تعویض موتور", 400000}, {"بازدید و عیب‌یابی", 120000}} {
		if _, err := d.DB.Exec(ctx, `INSERT INTO service_items(name,fee) SELECT $1,$2 WHERE NOT EXISTS (SELECT 1 FROM service_items WHERE name=$1)`, s.name, s.fee); err != nil {
			return err
		}
	}
	if _, err := d.DB.Exec(ctx, `INSERT INTO shifts(name,start_time,end_time) VALUES('عادی','08:00','16:30') ON CONFLICT (name) DO NOTHING`); err != nil {
		return err
	}
	for _, dep := range []string{"تولید", "انبار", "اداری", "فروش"} {
		if _, err := d.DB.Exec(ctx, `INSERT INTO departments(name) VALUES($1) ON CONFLICT DO NOTHING`, dep); err != nil {
			return err
		}
	}
	// the standard post-service survey, active from the start
	var n int
	_ = d.DB.QueryRow(ctx, `SELECT COUNT(*) FROM surveys WHERE trigger='repair_approved'`).Scan(&n)
	if n == 0 {
		id, err := db.Scalar[int64](ctx, d.DB, `INSERT INTO surveys(title,description,status,trigger) VALUES('رضایت از خدمات پس از فروش','لطفاً تجربه خود را از سرویس اوینکی بفرمایید','active','repair_approved') RETURNING id`)
		if err != nil {
			return err
		}
		for i, q := range []struct{ text, kind string }{
			{"از رفتار و تخصص تکنسین چقدر راضی بودید؟", "rating"},
			{"سرعت رفع مشکل چطور بود؟", "rating"},
			{"با چه احتمالی اوینکی را به دوستانتان پیشنهاد می‌کنید؟", "nps"},
			{"نظر یا پیشنهاد شما", "text"},
		} {
			if _, err := d.DB.Exec(ctx, `INSERT INTO survey_questions(survey_id,position,text,kind) VALUES($1,$2,$3,$4)`, id, i, q.text, q.kind); err != nil {
				return err
			}
		}
	}
	fmt.Println("demo data ready")
	return nil
}
