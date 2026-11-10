// Package sms queues messages in the database and delivers them from a
// background worker. Handlers only enqueue: a provider outage never fails or
// slows the request that triggered the SMS.
package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/evinki/platform/internal/platform/db"
)

// Builtin templates seeded on first boot. body_id stays 0 until an admin pastes
// the pattern id from the Melli Payamak panel - 0 means "do not send".
var Builtin = []struct{ Key, Title, Desc string }{
	{"otp", "کد ورود", "کد یکبار مصرف ورود مشتری. یک مقدار: کد"},
	{"guarantee_approved", "تایید گارانتی", "پس از تایید گارانتی برای مشتری"},
	{"guarantee_renewed", "تمدید گارانتی", "پس از تمدید گارانتی برای مشتری"},
	{"order_paid", "پرداخت سفارش", "پس از پرداخت موفق سفارش. مقادیر: شماره سفارش"},
	{"order_shipped", "ارسال سفارش", "مقادیر: شماره سفارش، کد رهگیری"},
	{"part_request", "درخواست قطعه", "برای کارکنان؛ وقتی تکنسین قطعه درخواست می‌کند"},
	{"repair_report", "گزارش تعمیر", "برای کارکنان؛ وقتی تکنسین گزارش تعمیر ثبت می‌کند"},
	{"survey_invite", "دعوت به نظرسنجی", "مقدار: لینک نظرسنجی"},
	{"survey_offer", "پیشنهاد پس از نظرسنجی", "پس از ثبت نظر"},
	{"test", "پیام آزمایشی", "فقط برای دکمه ارسال آزمایشی"},
}

func SeedTemplates(ctx context.Context, d db.Q) error {
	for _, t := range Builtin {
		if _, err := d.Exec(ctx, `INSERT INTO sms_templates(key,title,description,is_builtin) VALUES($1,$2,$3,TRUE)
			ON CONFLICT DO NOTHING`, t.Key, t.Title, t.Desc); err != nil {
			return err
		}
	}
	return nil
}

// Enqueue schedules a message. Safe to call inside a transaction.
func Enqueue(ctx context.Context, q db.Q, phone, template string, args ...string) error {
	if phone == "" {
		return nil
	}
	b, _ := json.Marshal(args)
	_, err := q.Exec(ctx, `INSERT INTO sms_outbox(phone,template_key,args) VALUES($1,$2,$3)`, phone, template, b)
	return err
}

type Sender interface {
	Send(ctx context.Context, phone string, bodyID int, args []string) error
}

// MeliPayamak uses the "base number" web service: it fills a pre-approved
// pattern, so only the values are ours to supply.
type MeliPayamak struct{ Username, Password string }

func (m MeliPayamak) Send(ctx context.Context, phone string, bodyID int, args []string) error {
	form := url.Values{
		"username": {m.Username}, "password": {m.Password},
		"to": {phone}, "bodyId": {fmt.Sprint(bodyID)}, "text": {strings.Join(args, ";")},
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://rest.payamak-panel.com/api/SendSMS/BaseServiceNumber", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	var out struct {
		RetStatus int    `json:"RetStatus"`
		StrRet    string `json:"StrRet"`
	}
	if json.Unmarshal(body, &out) != nil || out.RetStatus != 1 {
		return fmt.Errorf("provider rejected the message: %s", bytes.TrimSpace(body))
	}
	return nil
}

// Log only prints; used in development so nothing leaves the machine.
type Log struct{}

func (Log) Send(_ context.Context, phone string, bodyID int, args []string) error {
	slog.Info("SMS (dev, not sent)", "to", phone, "body_id", bodyID, "args", args)
	return nil
}

// Worker delivers pending rows with exponential back-off, up to 6 attempts.
func Worker(ctx context.Context, d *db.DB, s Sender) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			deliver(ctx, d, s)
		}
	}
}

func deliver(ctx context.Context, d *db.DB, s Sender) {
	type job struct {
		ID       int64           `db:"id"`
		Phone    string          `db:"phone"`
		Key      string          `db:"template_key"`
		Args     json.RawMessage `db:"args"`
		Attempts int             `db:"attempts"`
		BodyID   int             `db:"body_id"`
		Active   bool            `db:"is_active"`
	}
	jobs, err := db.All[job](ctx, d, `SELECT o.id,o.phone,o.template_key,o.args,o.attempts,
			COALESCE(t.body_id,0) AS body_id, COALESCE(t.is_active,FALSE) AS is_active
		FROM sms_outbox o LEFT JOIN sms_templates t ON t.key=o.template_key
		WHERE o.status='pending' AND o.next_try_at<=NOW() ORDER BY o.id LIMIT 20 FOR UPDATE OF o SKIP LOCKED`)
	if err != nil {
		slog.Error("sms outbox read", "err", err)
		return
	}
	for _, j := range jobs {
		if j.BodyID == 0 || !j.Active {
			_, _ = d.Exec(ctx, `UPDATE sms_outbox SET status='skipped', error='الگوی پیامک تنظیم نشده است' WHERE id=$1`, j.ID)
			continue
		}
		var args []string
		_ = json.Unmarshal(j.Args, &args)
		if err := s.Send(ctx, j.Phone, j.BodyID, args); err != nil {
			if j.Attempts+1 >= 6 {
				_, _ = d.Exec(ctx, `UPDATE sms_outbox SET status='failed', attempts=attempts+1, error=$2 WHERE id=$1`, j.ID, err.Error())
			} else {
				backoff := time.Duration(1<<j.Attempts) * time.Minute
				_, _ = d.Exec(ctx, `UPDATE sms_outbox SET attempts=attempts+1, error=$2, next_try_at=NOW()+$3::interval WHERE id=$1`,
					j.ID, err.Error(), fmt.Sprintf("%d seconds", int(backoff.Seconds())))
			}
			continue
		}
		_, _ = d.Exec(ctx, `UPDATE sms_outbox SET status='sent', sent_at=NOW(), attempts=attempts+1, error='' WHERE id=$1`, j.ID)
	}
}
