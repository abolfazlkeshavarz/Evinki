// Package system: settings, SMS patterns, the manager overview and health.
package system

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/evinki/platform/internal/app"
	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
	"github.com/evinki/platform/internal/platform/settings"
	"github.com/evinki/platform/internal/platform/sms"
)

type mod struct{ *app.Deps }

func Mount(r chi.Router, d *app.Deps) {
	m := &mod{d}
	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PSettings))
		r.Get("/admin/settings", httpx.H(m.getSettings))
		r.Put("/admin/settings", httpx.H(m.putSettings))
		r.Get("/admin/sms/templates", httpx.H(m.templates))
		r.Put("/admin/sms/templates/{key}", httpx.H(m.saveTemplate))
		r.Delete("/admin/sms/templates/{key}", httpx.H(m.deleteTemplate))
		r.Post("/admin/sms/test", httpx.H(m.testSMS))
		r.Get("/admin/sms/outbox", httpx.H(m.outbox))
		r.Get("/admin/system/update", httpx.H(m.updateStatus))
		r.Put("/admin/system/update", httpx.H(m.uploadUpdate))
	})
	// The management app's home screen: one call, everything that needs attention.
	r.With(auth.Require(auth.PReports)).Get("/manager/overview", httpx.H(m.overview))
}

func (m *mod) getSettings(w http.ResponseWriter, r *http.Request) error {
	s, err := settings.All(r.Context(), m.DB)
	if err != nil {
		return err
	}
	httpx.OK(w, s)
	return nil
}

func (m *mod) putSettings(w http.ResponseWriter, r *http.Request) error {
	var in map[string]json.RawMessage
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	for k, raw := range in {
		def, known := settings.Defaults[k]
		if !known {
			return httpx.BadRequest("تنظیم ناشناخته: " + k)
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return httpx.BadRequest("مقدار نامعتبر: " + k)
		}
		// keep types stable: a bool stays a bool, a number stays a number
		switch def.(type) {
		case bool:
			if _, ok := v.(bool); !ok {
				return httpx.BadRequest("مقدار بولی لازم است: " + k)
			}
		case int, float64:
			if _, ok := v.(float64); !ok {
				return httpx.BadRequest("مقدار عددی لازم است: " + k)
			}
		case string:
			if _, ok := v.(string); !ok {
				return httpx.BadRequest("مقدار متنی لازم است: " + k)
			}
		}
		if err := settings.Set(r.Context(), m.DB, k, v); err != nil {
			return err
		}
	}
	m.Audit.Log(r, "settings.update", "settings", "", "")
	return m.getSettings(w, r)
}

func (m *mod) templates(w http.ResponseWriter, r *http.Request) error {
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t ORDER BY t.is_builtin DESC, t.key),'[]') FROM sms_templates t`)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
	return nil
}

func (m *mod) saveTemplate(w http.ResponseWriter, r *http.Request) error {
	key := chi.URLParam(r, "key")
	var in struct {
		Title      string `json:"title"`
		Description string `json:"description"`
		BodyID     int    `json:"body_id"`
		SampleText string `json:"sample_text"`
		IsActive   bool   `json:"is_active"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.BodyID < 0 || strings.TrimSpace(in.Title) == "" {
		return httpx.BadRequest("عنوان و شناسه الگو را درست وارد کنید")
	}
	_, err := m.DB.Exec(r.Context(), `INSERT INTO sms_templates(key,title,description,body_id,sample_text,is_active) VALUES($1,$2,$3,$4,$5,$6)
		ON CONFLICT (key) DO UPDATE SET title=EXCLUDED.title,description=EXCLUDED.description,body_id=EXCLUDED.body_id,
		sample_text=EXCLUDED.sample_text,is_active=EXCLUDED.is_active,updated_at=NOW()`,
		key, in.Title, in.Description, in.BodyID, in.SampleText, in.IsActive)
	if err != nil {
		return err
	}
	m.Audit.Log(r, "sms_template.save", "sms_template", key, "")
	httpx.NoContent(w)
	return nil
}

func (m *mod) deleteTemplate(w http.ResponseWriter, r *http.Request) error {
	tag, err := m.DB.Exec(r.Context(), `DELETE FROM sms_templates WHERE key=$1 AND NOT is_builtin`, chi.URLParam(r, "key"))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.Conflict("الگوهای پیش‌فرض قابل حذف نیستند")
	}
	httpx.NoContent(w)
	return nil
}

func (m *mod) testSMS(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Phone string `json:"phone"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	phone := httpx.Phone(in.Phone)
	if !httpx.ValidMobile(phone) {
		return httpx.BadRequest("شماره موبایل نامعتبر است")
	}
	if err := sms.Enqueue(r.Context(), m.DB, phone, "test", "آزمایش"); err != nil {
		return err
	}
	httpx.OK(w, map[string]any{"queued": true})
	return nil
}

func (m *mod) outbox(w http.ResponseWriter, r *http.Request) error {
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
		SELECT id,phone,template_key,status,attempts,error,created_at,sent_at FROM sms_outbox ORDER BY id DESC LIMIT 100) t`)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
	return nil
}

// overview is deliberately one query: the management app opens on it.
func (m *mod) overview(w http.ResponseWriter, r *http.Request) error {
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT row_to_json(t) FROM (SELECT
	  json_build_object(
	    'orders_today',(SELECT COUNT(*) FROM orders WHERE created_at::date=CURRENT_DATE AND status<>'pending_payment'),
	    'revenue_today',(SELECT COALESCE(SUM(total),0) FROM orders WHERE paid_at::date=CURRENT_DATE AND status NOT IN ('cancelled','refunded')),
	    'revenue_month',(SELECT COALESCE(SUM(total),0) FROM orders WHERE paid_at >= date_trunc('month',NOW()) AND status NOT IN ('cancelled','refunded')),
	    'to_ship',(SELECT COUNT(*) FROM orders WHERE status IN ('paid','processing'))) AS shop,
	  json_build_object(
	    'pending_approval',(SELECT COUNT(*) FROM guarantees WHERE status='pending' AND deleted_at IS NULL),
	    'active',(SELECT COUNT(*) FROM guarantees WHERE status IN ('approved','renewed') AND expiry_date>=CURRENT_DATE AND deleted_at IS NULL),
	    'expiring_30d',(SELECT COUNT(*) FROM guarantees WHERE status IN ('approved','renewed') AND expiry_date BETWEEN CURRENT_DATE AND CURRENT_DATE+30 AND deleted_at IS NULL)) AS warranty,
	  json_build_object(
	    'awaiting_review',(SELECT COUNT(*) FROM repairs WHERE status='submitted' AND deleted_at IS NULL),
	    'unassigned',(SELECT COUNT(*) FROM repairs WHERE status='requested' AND deleted_at IS NULL),
	    'open',(SELECT COUNT(*) FROM repairs WHERE status IN ('assigned','in_progress','waiting_parts') AND deleted_at IS NULL),
	    'requests_pending',(SELECT COUNT(*) FROM part_requests WHERE status='pending' AND deleted_at IS NULL),
	    'shipments_to_receive',(SELECT COUNT(*) FROM shipments WHERE status='sent' AND deleted_at IS NULL),
	    'payable',(SELECT COALESCE(SUM(invoice_total),0) FROM shipments WHERE status='invoiced' AND deleted_at IS NULL),
	    'technicians_pending',(SELECT COUNT(*) FROM technicians WHERE status='pending')) AS service,
	  json_build_object(
	    'pending_documents',(SELECT COUNT(*) FROM stock_documents WHERE status='pending' AND deleted_at IS NULL),
	    'low_stock',(SELECT COUNT(*) FROM parts p WHERE p.deleted_at IS NULL AND p.is_active AND p.min_stock>0 AND
	       COALESCE((SELECT SUM(sl.qty) FROM stock_levels sl JOIN warehouses w ON w.id=sl.warehouse_id AND w.kind='central' WHERE sl.part_id=p.id),0) <= p.min_stock)) AS warehouse,
	  json_build_object(
	    'arrived_today',(SELECT COUNT(DISTINCT employee_id) FROM attendance_records WHERE work_date=CURRENT_DATE AND kind='in'),
	    'employees',(SELECT COUNT(*) FROM employees WHERE is_active),
	    'pending_leaves',(SELECT COUNT(*) FROM leave_requests WHERE status='pending')) AS hr,
	  json_build_object(
	    'responses_30d',(SELECT COUNT(*) FROM survey_recipients WHERE responded_at > NOW()-INTERVAL '30 days'),
	    'avg_rating_30d',(SELECT ROUND(AVG(a.answer_number)::numeric,2) FROM survey_answers a JOIN survey_questions q ON q.id=a.question_id AND q.kind='rating' WHERE a.created_at > NOW()-INTERVAL '30 days')) AS surveys
	) t`)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
	return nil
}
