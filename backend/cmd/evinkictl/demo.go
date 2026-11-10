package main

import (
	"context"
	"fmt"

	"github.com/evinki/platform/internal/app"
	"github.com/evinki/platform/internal/modules/catalog"
	"github.com/evinki/platform/internal/platform/db"
)

// seedDemo creates a small, realistic catalogue so a fresh install is not an
// empty page: it mirrors the products visible on evinki.com. Safe to re-run.
func seedDemo(ctx context.Context, d *app.Deps) error {
	cats := map[string]int64{}
	for _, c := range []struct{ slug, name, en string }{
		{"vacuum-cleaners", "جاروبرقی", "Vacuum cleaners"},
		{"accessories", "لوازم جانبی", "Accessories"},
	} {
		id, err := db.Scalar[int64](ctx, d.DB, `INSERT INTO categories(slug,name,name_en) VALUES($1,$2,$3)
			ON CONFLICT (slug) DO UPDATE SET name=EXCLUDED.name RETURNING id`, c.slug, c.name, c.en)
		if err != nil {
			return err
		}
		cats[c.slug] = id
	}
	_, err := d.DB.Exec(ctx, `INSERT INTO products(category_id,sku,slug,name,name_en,summary,specs,price,stock_qty,guarantee_months,golden_months,code_format,code_prefix,price_on_request)
		VALUES($1,'FZ530','fz530-performance','جاروبرقی FZ530 Performance','Evinki FZ530 Performance',
		'قدرتی که خانه را متحول می‌کند',
		'{"توان موتور":"۲۴۰۰ وات","حجم کیسه":"۴ لیتر","فیلتر":"HEPA13 قابل شستشو","سطح صدا":"۵۸ دسی‌بل","طول کابل":"۱۰ متر"}',
		0,20,18,3,'evinki_vacuum','FZ',TRUE) ON CONFLICT (sku) DO NOTHING`, cats["vacuum-cleaners"])
	if err != nil {
		return err
	}
	parts := []struct {
		code, name string
		sale, buy  int64
		sellable   bool
	}{
		{"FLT-HEPA", "فیلتر هپا جاروبرقی", 450000, 60000, true},
		{"HOSE-STD", "خرطومی ساده جاروبرقی", 380000, 50000, true},
		{"WHEEL", "چرخ جاروبرقی", 120000, 15000, true},
		{"CORD-REEL", "سیم جمع کن جاروبرقی", 210000, 30000, true},
		{"BAG-LOCK", "قفل کیسه", 60000, 8000, true},
		{"MOTOR-FELT", "نمد موتور جاروبرقی", 45000, 5000, false},
	}
	for _, p := range parts {
		if _, err := d.DB.Exec(ctx, `INSERT INTO parts(code,name,sale_price,buyback_price,is_sellable,qr_token) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT (code) DO NOTHING`,
			p.code, p.name, p.sale, p.buy, p.sellable, catalog.NewQRToken()); err != nil {
			return err
		}
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
	// the standard post-purchase survey, active from the start
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
	fmt.Println("demo catalogue created (FZ530 is 'price on request' like the public site)")
	return nil
}
