// Package surveys replaces the old "polls". A survey is a set of questions
// plus a trigger: sent by hand to a filtered audience, or automatically after
// every approved repair / delivered order.
package surveys

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/evinki/platform/internal/app"
	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
	"github.com/evinki/platform/internal/platform/sms"
)

type mod struct{ *app.Deps }

var publicBase string // set at Mount; used by the Trigger* helpers that run without a request

func Mount(r chi.Router, d *app.Deps) {
	m := &mod{d}
	publicBase = d.Cfg.PublicURL

	r.Get("/public/surveys/{token}", httpx.H(m.publicGet))
	r.Post("/public/surveys/{token}/answers", httpx.H(m.publicAnswer))

	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PSurveyRead))
		r.Get("/admin/surveys", httpx.H(m.list))
		r.Get("/admin/surveys/{id}", httpx.H(m.get))
		r.Get("/admin/surveys/{id}/results", httpx.H(m.results))
		r.Get("/admin/surveys/{id}/recipients", httpx.H(m.recipients))
	})
	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PSurveyManage))
		r.Post("/admin/surveys", httpx.H(m.save))
		r.Put("/admin/surveys/{id}", httpx.H(m.save))
		r.Post("/admin/surveys/{id}/launch", httpx.H(m.launch))
		r.Post("/admin/surveys/{id}/close", httpx.H(m.close))
		r.Delete("/admin/surveys/{id}", httpx.H(m.remove))
	})
}

type question struct {
	ID       int64    `json:"id,omitempty"`
	Text     string   `json:"text"`
	Kind     string   `json:"kind"`
	Options  []string `json:"options"`
	Required bool     `json:"required"`
}

type surveyIn struct {
	Title         string          `json:"title"`
	Description   string          `json:"description"`
	Trigger       string          `json:"trigger"`
	SMSTemplate   string          `json:"sms_template"`
	OfferTemplate string          `json:"offer_template"`
	LinkMode      string          `json:"link_mode"`
	Filter        json.RawMessage `json:"filter"`
	Status        string          `json:"status"`
	Questions     []question      `json:"questions"`
}

func (m *mod) list(w http.ResponseWriter, r *http.Request) error {
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t ORDER BY t.id DESC),'[]') FROM (
		SELECT s.id,s.title,s.status,s.trigger,s.created_at,
		  (SELECT COUNT(*) FROM survey_recipients x WHERE x.survey_id=s.id)::int AS recipients,
		  (SELECT COUNT(*) FROM survey_recipients x WHERE x.survey_id=s.id AND x.responded_at IS NOT NULL)::int AS responses
		FROM surveys s WHERE s.deleted_at IS NULL) t`)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
	return nil
}

func (m *mod) get(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT row_to_json(t) FROM (
		SELECT s.id,s.title,s.description,s.status,s.trigger,s.sms_template,s.offer_template,s.link_mode,s.filter,s.created_at,
		 COALESCE((SELECT json_agg(q ORDER BY q.position) FROM (SELECT id,position,text,kind,options,required FROM survey_questions WHERE survey_id=s.id) q),'[]') AS questions
		FROM surveys s WHERE s.id=$1 AND s.deleted_at IS NULL) t`, id)
	if err != nil || len(raw) == 0 {
		return httpx.ErrNotFound
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
	return nil
}

func (m *mod) save(w http.ResponseWriter, r *http.Request) error {
	var in surveyIn
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Title) == "" || len(in.Questions) == 0 {
		return httpx.BadRequest("عنوان و حداقل یک سوال لازم است")
	}
	for _, q := range in.Questions {
		switch q.Kind {
		case "rating", "nps", "yes_no", "text":
		case "choice":
			if len(q.Options) < 2 {
				return httpx.BadRequest("سوال چندگزینه‌ای حداقل دو گزینه می‌خواهد")
			}
		default:
			return httpx.BadRequest("نوع سوال نامعتبر است")
		}
		if strings.TrimSpace(q.Text) == "" {
			return httpx.BadRequest("متن سوال خالی است")
		}
	}
	if in.Trigger == "" {
		in.Trigger = "manual"
	}
	if in.LinkMode == "" {
		in.LinkMode = "url"
	}
	if in.SMSTemplate == "" {
		in.SMSTemplate = "survey_invite"
	}
	if len(in.Filter) == 0 {
		in.Filter = json.RawMessage(`{}`)
	}
	ctx := r.Context()
	var id int64
	editing := chi.URLParam(r, "id") != ""
	err := m.DB.Tx(ctx, func(tx pgx.Tx) error {
		var e error
		if editing {
			if id, e = httpx.ID(r, "id"); e != nil {
				return e
			}
			// Questions with answers must not be rewritten under respondents' feet.
			if n, _ := db.Scalar[int](ctx, tx, `SELECT COUNT(*) FROM survey_answers a JOIN survey_questions q ON q.id=a.question_id WHERE q.survey_id=$1`, id); n > 0 {
				_, e = tx.Exec(ctx, `UPDATE surveys SET title=$2,description=$3,updated_at=NOW() WHERE id=$1`, id, in.Title, in.Description)
				return e
			}
			if _, e = tx.Exec(ctx, `UPDATE surveys SET title=$2,description=$3,trigger=$4,sms_template=$5,offer_template=$6,link_mode=$7,filter=$8,updated_at=NOW()
				WHERE id=$1 AND deleted_at IS NULL`, id, in.Title, in.Description, in.Trigger, in.SMSTemplate, in.OfferTemplate, in.LinkMode, in.Filter); e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `DELETE FROM survey_questions WHERE survey_id=$1`, id); e != nil {
				return e
			}
		} else {
			id, e = db.Scalar[int64](ctx, tx, `INSERT INTO surveys(title,description,trigger,sms_template,offer_template,link_mode,filter,created_by)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, in.Title, in.Description, in.Trigger, in.SMSTemplate, in.OfferTemplate, in.LinkMode, in.Filter, auth.From(ctx).UserID)
			if e != nil {
				return e
			}
		}
		for i, q := range in.Questions {
			opts, _ := json.Marshal(q.Options)
			if q.Options == nil {
				opts = []byte("[]")
			}
			if _, e = tx.Exec(ctx, `INSERT INTO survey_questions(survey_id,position,text,kind,options,required) VALUES($1,$2,$3,$4,$5,$6)`,
				id, i, q.Text, q.Kind, opts, q.Required); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Automatic surveys go live as soon as they exist; manual ones wait for "launch".
	if !editing && in.Trigger != "manual" {
		_, _ = m.DB.Exec(ctx, `UPDATE surveys SET status='active' WHERE id=$1`, id)
	}
	m.Audit.Log(r, "survey.save", "survey", id, in.Title)
	return m.getByID(w, r, id)
}

func (m *mod) getByID(w http.ResponseWriter, r *http.Request, id int64) error {
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT row_to_json(t) FROM (SELECT id,title,status,trigger FROM surveys WHERE id=$1) t`, id)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
	return nil
}

func (m *mod) close(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	_, err = m.DB.Exec(r.Context(), `UPDATE surveys SET status='closed', closed_at=NOW() WHERE id=$1`, id)
	httpx.NoContent(w)
	return err
}

func (m *mod) remove(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	_, err = m.DB.Exec(r.Context(), `UPDATE surveys SET deleted_at=NOW(), status='closed' WHERE id=$1`, id)
	httpx.NoContent(w)
	return err
}

// launch builds the audience from guarantees and queues one SMS per customer.
func (m *mod) launch(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	ctx := r.Context()
	type sv struct {
		Status   string          `db:"status"`
		Trigger  string          `db:"trigger"`
		Template string          `db:"sms_template"`
		LinkMode string          `db:"link_mode"`
		Filter   json.RawMessage `db:"filter"`
	}
	s, err := db.One[sv](ctx, m.DB, `SELECT status,trigger,sms_template,link_mode,filter FROM surveys WHERE id=$1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	if s.Status == "closed" {
		return httpx.Conflict("نظرسنجی بسته شده است")
	}
	var f struct {
		City            string `json:"city"`
		Province        string `json:"province"`
		ProductID       int64  `json:"product_id"`
		PurchasedBefore string `json:"purchased_before"`
		ExpiryBefore    string `json:"expiry_before"`
	}
	_ = json.Unmarshal(s.Filter, &f)

	type aud struct {
		CustomerID int64  `db:"customer_id"`
		GuaranteeID int64 `db:"guarantee_id"`
		Name       string `db:"full_name"`
		Phone      string `db:"phone"`
		City       string `db:"city"`
		Product    string `db:"product_name"`
	}
	people, err := db.All[aud](ctx, m.DB, `SELECT DISTINCT ON (c.id) c.id AS customer_id, g.id AS guarantee_id, c.full_name, c.phone, c.city, p.name AS product_name
		FROM guarantees g JOIN customers c ON c.id=g.customer_id JOIN products p ON p.id=g.product_id
		WHERE g.deleted_at IS NULL AND g.status IN ('approved','renewed') AND c.phone<>'' AND c.deleted_at IS NULL
		  AND ($1='' OR c.city=$1) AND ($2='' OR c.province=$2) AND ($3=0 OR g.product_id=$3)
		  AND ($4='' OR g.purchase_date < $4::date) AND ($5='' OR g.expiry_date < $5::date)
		  AND NOT EXISTS (SELECT 1 FROM survey_recipients x WHERE x.survey_id=$6 AND x.customer_id=c.id)
		ORDER BY c.id, g.id DESC`, f.City, f.Province, f.ProductID, f.PurchasedBefore, f.ExpiryBefore, id)
	if err != nil {
		return err
	}
	err = m.DB.Tx(ctx, func(tx pgx.Tx) error {
		for _, p := range people {
			tok := auth.RandToken(12)
			if _, e := tx.Exec(ctx, `INSERT INTO survey_recipients(survey_id,customer_id,guarantee_id,token,customer_name,phone,city,product_name)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, p.CustomerID, p.GuaranteeID, tok, p.Name, p.Phone, p.City, p.Product); e != nil {
				return e
			}
			if e := sms.Enqueue(ctx, tx, p.Phone, s.Template, linkFor(s.LinkMode, tok)); e != nil {
				return e
			}
		}
		_, e := tx.Exec(ctx, `UPDATE surveys SET status='active' WHERE id=$1 AND status='draft'`, id)
		return e
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "survey.launch", "survey", id, fmt.Sprint(len(people)))
	httpx.OK(w, map[string]any{"queued": len(people)})
	return nil
}

func linkFor(mode, token string) string {
	if mode == "token" {
		return token
	}
	return publicBase + "/s/" + token
}

// ───────── automatic triggers ─────────

type autoSurvey struct {
	ID       int64  `db:"id"`
	Template string `db:"sms_template"`
	LinkMode string `db:"link_mode"`
}

// TriggerOrder invites the buyer of a delivered order to every active order survey.
func TriggerOrder(ctx context.Context, q db.Q, orderID int64) error {
	type who struct {
		CustomerID *int64 `db:"customer_id"`
		Name       string `db:"name"`
		Phone      string `db:"phone"`
		City       string `db:"city"`
		Product    string `db:"product"`
	}
	w, err := db.One[who](ctx, q, `SELECT (SELECT id FROM customers WHERE user_id=o.user_id ORDER BY id LIMIT 1) AS customer_id,
		COALESCE(NULLIF(o.address->>'recipient',''),u.full_name) AS name, COALESCE(NULLIF(u.phone,''),o.address->>'phone') AS phone,
		COALESCE(o.address->>'city','') AS city, COALESCE((SELECT name FROM order_items WHERE order_id=o.id ORDER BY id LIMIT 1),'') AS product
		FROM orders o JOIN users u ON u.id=o.user_id WHERE o.id=$1`, orderID)
	if err != nil {
		return err
	}
	list, err := db.All[autoSurvey](ctx, q, `SELECT id,sms_template,link_mode FROM surveys WHERE status='active' AND trigger='order_delivered' AND deleted_at IS NULL`)
	if err != nil {
		return err
	}
	for _, s := range list {
		tok := auth.RandToken(12)
		tag, err := q.Exec(ctx, `INSERT INTO survey_recipients(survey_id,customer_id,order_id,token,customer_name,phone,city,product_name)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`, s.ID, w.CustomerID, orderID, tok, w.Name, w.Phone, w.City, w.Product)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			if err := sms.Enqueue(ctx, q, w.Phone, s.Template, linkFor(s.LinkMode, tok)); err != nil {
				return err
			}
		}
	}
	return nil
}

// TriggerRepair invites the customer of an approved repair; the answers are
// attributed to the technician who did the work.
func TriggerRepair(ctx context.Context, q db.Q, repairID int64) error {
	type who struct {
		CustomerID int64  `db:"customer_id"`
		Name       string `db:"name"`
		Phone      string `db:"phone"`
		City       string `db:"city"`
		Product    string `db:"product"`
		Tech       *int64 `db:"technician_id"`
		GuaranteeID *int64 `db:"guarantee_id"`
	}
	w, err := db.One[who](ctx, q, `SELECT r.customer_id, c.full_name AS name, c.phone, c.city, COALESCE(p.name,'') AS product, r.technician_id, r.guarantee_id
		FROM repairs r JOIN customers c ON c.id=r.customer_id LEFT JOIN products p ON p.id=r.product_id WHERE r.id=$1`, repairID)
	if err != nil || w.Phone == "" {
		return err
	}
	list, err := db.All[autoSurvey](ctx, q, `SELECT id,sms_template,link_mode FROM surveys WHERE status='active' AND trigger='repair_approved' AND deleted_at IS NULL`)
	if err != nil {
		return err
	}
	for _, s := range list {
		tok := auth.RandToken(12)
		tag, err := q.Exec(ctx, `INSERT INTO survey_recipients(survey_id,customer_id,guarantee_id,repair_id,technician_id,token,customer_name,phone,city,product_name)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT DO NOTHING`, s.ID, w.CustomerID, w.GuaranteeID, repairID, w.Tech, tok, w.Name, w.Phone, w.City, w.Product)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			if err := sms.Enqueue(ctx, q, w.Phone, s.Template, linkFor(s.LinkMode, tok)); err != nil {
				return err
			}
		}
	}
	return nil
}

// ───────── public answering ─────────

func (m *mod) publicGet(w http.ResponseWriter, r *http.Request) error {
	tok := chi.URLParam(r, "token")
	type rec struct {
		ID        int64      `db:"id"`
		SurveyID  int64      `db:"survey_id"`
		Responded *time.Time `db:"responded_at"`
		Status    string     `db:"status"`
		Title     string     `db:"title"`
		Desc      string     `db:"description"`
		Name      string     `db:"customer_name"`
	}
	v, err := db.One[rec](r.Context(), m.DB, `SELECT x.id,x.survey_id,x.responded_at,s.status,s.title,s.description,x.customer_name
		FROM survey_recipients x JOIN surveys s ON s.id=x.survey_id WHERE x.token=$1 AND s.deleted_at IS NULL`, tok)
	if err != nil {
		return httpx.ErrNotFound
	}
	_, _ = m.DB.Exec(r.Context(), `UPDATE survey_recipients SET opened_at=COALESCE(opened_at,NOW()) WHERE id=$1`, v.ID)
	qs, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(q ORDER BY q.position),'[]') FROM
		(SELECT id,position,text,kind,options,required FROM survey_questions WHERE survey_id=$1) q`, v.SurveyID)
	if err != nil {
		return err
	}
	httpx.OK(w, map[string]any{"title": v.Title, "description": v.Desc, "name": v.Name, "answered": v.Responded != nil,
		"closed": v.Status == "closed", "questions": qs})
	return nil
}

func (m *mod) publicAnswer(w http.ResponseWriter, r *http.Request) error {
	tok := chi.URLParam(r, "token")
	var in struct {
		Answers []struct {
			QuestionID int64  `json:"question_id"`
			Number     *int   `json:"number"`
			Text       string `json:"text"`
		} `json:"answers"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	ctx := r.Context()
	type rec struct {
		ID        int64      `db:"id"`
		SurveyID  int64      `db:"survey_id"`
		Responded *time.Time `db:"responded_at"`
		Status    string     `db:"status"`
		Offer     string     `db:"offer_template"`
		Phone     string     `db:"phone"`
	}
	v, err := db.One[rec](ctx, m.DB, `SELECT x.id,x.survey_id,x.responded_at,s.status,s.offer_template,x.phone FROM survey_recipients x
		JOIN surveys s ON s.id=x.survey_id WHERE x.token=$1 AND s.deleted_at IS NULL`, tok)
	if err != nil {
		return httpx.ErrNotFound
	}
	if v.Responded != nil {
		return httpx.Conflict("این نظرسنجی قبلاً تکمیل شده است")
	}
	if v.Status == "closed" {
		return httpx.Conflict("این نظرسنجی بسته شده است")
	}
	type qd struct {
		ID       int64    `db:"id"`
		Kind     string   `db:"kind"`
		Options  []byte   `db:"options"`
		Required bool     `db:"required"`
	}
	qs, err := db.All[qd](ctx, m.DB, `SELECT id,kind,options,required FROM survey_questions WHERE survey_id=$1`, v.SurveyID)
	if err != nil {
		return err
	}
	byQ := map[int64]qd{}
	for _, q := range qs {
		byQ[q.ID] = q
	}
	given := map[int64]bool{}
	err = m.DB.Tx(ctx, func(tx pgx.Tx) error {
		for _, a := range in.Answers {
			q, ok := byQ[a.QuestionID]
			if !ok {
				return httpx.BadRequest("سوال نامعتبر")
			}
			text := strings.TrimSpace(a.Text)
			switch q.Kind {
			case "rating":
				if a.Number == nil || *a.Number < 1 || *a.Number > 5 {
					continue
				}
			case "nps":
				if a.Number == nil || *a.Number < 0 || *a.Number > 10 {
					continue
				}
			case "yes_no":
				if a.Number == nil || (*a.Number != 0 && *a.Number != 1) {
					continue
				}
			case "choice":
				var opts []string
				_ = json.Unmarshal(q.Options, &opts)
				okOpt := false
				for _, o := range opts {
					okOpt = okOpt || o == text
				}
				if !okOpt {
					continue
				}
			case "text":
				if text == "" {
					continue
				}
				if len(text) > 2000 {
					text = text[:2000]
				}
			}
			given[q.ID] = true
			if _, e := tx.Exec(ctx, `INSERT INTO survey_answers(recipient_id,question_id,answer_text,answer_number) VALUES($1,$2,$3,$4)
				ON CONFLICT (recipient_id,question_id) DO NOTHING`, v.ID, q.ID, text, a.Number); e != nil {
				return e
			}
		}
		for _, q := range qs {
			if q.Required && !given[q.ID] {
				return httpx.BadRequest("به سوال‌های ضروری پاسخ دهید")
			}
		}
		if len(given) == 0 {
			return httpx.BadRequest("هیچ پاسخی ثبت نشد")
		}
		if _, e := tx.Exec(ctx, `UPDATE survey_recipients SET responded_at=NOW() WHERE id=$1`, v.ID); e != nil {
			return e
		}
		if v.Offer != "" {
			if e := sms.Enqueue(ctx, tx, v.Phone, v.Offer); e != nil {
				return e
			}
			_, e := tx.Exec(ctx, `UPDATE survey_recipients SET offer_sent_at=NOW() WHERE id=$1`, v.ID)
			return e
		}
		return nil
	})
	if err != nil {
		return err
	}
	httpx.OK(w, map[string]any{"ok": true})
	return nil
}

// ───────── results ─────────

func (m *mod) recipients(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	pg := httpx.Paging(r, 200)
	raw, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
		SELECT id,customer_name,phone,city,product_name,sms_status,sent_at,opened_at,responded_at FROM survey_recipients
		WHERE survey_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3) t`, id, pg.Limit, pg.Offset)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
	return nil
}

func (m *mod) results(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	ctx := r.Context()
	type counts struct {
		Total     int `db:"total"`
		Opened    int `db:"opened"`
		Responded int `db:"responded"`
	}
	c, err := db.One[counts](ctx, m.DB, `SELECT COUNT(*)::int AS total, COUNT(opened_at)::int AS opened, COUNT(responded_at)::int AS responded
		FROM survey_recipients WHERE survey_id=$1`, id)
	if err != nil {
		return err
	}
	qs, err := db.Scalar[json.RawMessage](ctx, m.DB, `SELECT COALESCE(json_agg(t ORDER BY t.position),'[]') FROM (
		SELECT q.id,q.position,q.text,q.kind,
		  COUNT(a.id)::int AS answers,
		  ROUND(AVG(a.answer_number)::numeric,2) AS average,
		  COALESCE((SELECT json_object_agg(n, cnt) FROM (SELECT answer_number::text AS n, COUNT(*) AS cnt FROM survey_answers WHERE question_id=q.id AND answer_number IS NOT NULL GROUP BY 1) d),'{}') AS distribution,
		  COALESCE((SELECT json_object_agg(txt, cnt) FROM (SELECT answer_text AS txt, COUNT(*) AS cnt FROM survey_answers WHERE question_id=q.id AND q.kind='choice' GROUP BY 1) d),'{}') AS choices,
		  CASE WHEN q.kind='nps' THEN
		    ROUND(100.0*(COUNT(a.id) FILTER (WHERE a.answer_number>=9) - COUNT(a.id) FILTER (WHERE a.answer_number<=6)) / NULLIF(COUNT(a.id),0),1) END AS nps,
		  CASE WHEN q.kind='text' THEN COALESCE((SELECT json_agg(x) FROM (SELECT answer_text FROM survey_answers WHERE question_id=q.id AND answer_text<>'' ORDER BY id DESC LIMIT 50) x),'[]') END AS texts
		FROM survey_questions q LEFT JOIN survey_answers a ON a.question_id=q.id
		WHERE q.survey_id=$1 GROUP BY q.id) t`, id)
	if err != nil {
		return err
	}
	// how each technician is rated by the customers they served
	techs, err := db.Scalar[json.RawMessage](ctx, m.DB, `SELECT COALESCE(json_agg(t ORDER BY t.average DESC),'[]') FROM (
		SELECT x.technician_id, u.full_name AS technician_name, COUNT(DISTINCT x.id)::int AS responses,
		  ROUND(AVG(a.answer_number)::numeric,2) AS average
		FROM survey_recipients x JOIN survey_answers a ON a.recipient_id=x.id
		JOIN survey_questions q ON q.id=a.question_id AND q.kind='rating'
		JOIN users u ON u.id=x.technician_id
		WHERE x.survey_id=$1 AND x.technician_id IS NOT NULL GROUP BY x.technician_id,u.full_name) t`, id)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	rate := 0.0
	if c.Total > 0 {
		rate = float64(c.Responded) * 100 / float64(c.Total)
	}
	fmt.Fprintf(w, `{"recipients":%d,"opened":%d,"responded":%d,"response_rate":%.1f,"questions":%s,"technicians":%s}`,
		c.Total, c.Opened, c.Responded, rate, qs, techs)
	return nil
}
