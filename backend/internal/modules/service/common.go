// Package service is the after-sales workflow: repairs, part requests, old-part
// shipments and the technician's account. See migrations/0004 for the design
// note on why one repair_parts row follows a part from request to payment.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/evinki/platform/internal/app"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
	"github.com/evinki/platform/internal/platform/sms"
)

type mod struct{ *app.Deps }

// editable statuses: the technician may still change the report.
func editable(status string) bool {
	switch status {
	case "assigned", "in_progress", "waiting_parts":
		return true
	}
	return false
}

func raw(w http.ResponseWriter, status int, b []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func rawJSON(w http.ResponseWriter, b json.RawMessage) { raw(w, 200, b) }

// repairDetail assembles everything about one repair as JSON.
func repairDetail(ctx context.Context, q db.Q, id int64) (json.RawMessage, error) {
	b, err := db.Scalar[json.RawMessage](ctx, q, `SELECT row_to_json(t) FROM (
	  SELECT r.id,r.number,r.status,r.source,r.complaint,r.diagnosis,r.report,r.guarantee_was_expired,r.customer_charge,r.customer_paid,
	    r.visit_at,r.started_at,r.submitted_at,r.completed_at,r.reviewed_at,r.review_notes,r.created_at,r.updated_at,
	    r.guarantee_id,(SELECT code FROM guarantees WHERE id=r.guarantee_id) AS guarantee_code,
	    (SELECT expiry_date FROM guarantees WHERE id=r.guarantee_id) AS guarantee_expiry,
	    r.customer_id,c.full_name AS customer_name,c.phone AS customer_phone,c.city AS customer_city,c.address AS customer_address,
	    r.product_id,p.name AS product_name,
	    r.technician_id,(SELECT COALESCE(NULLIF(full_name,''),username) FROM users WHERE id=r.technician_id) AS technician_name,
	    COALESCE((SELECT json_agg(x ORDER BY x.id) FROM (
	      SELECT rp.id,rp.part_id,COALESCE(pt.name,rp.custom_name) AS name,pt.code,rp.qty,rp.report,rp.source,rp.return_required,rp.return_state,
	             rp.shipment_id,rp.buyback_price FROM repair_parts rp LEFT JOIN parts pt ON pt.id=rp.part_id WHERE rp.repair_id=r.id) x),'[]') AS parts,
	    COALESCE((SELECT json_agg(x ORDER BY x.id) FROM (
	      SELECT rs.id,rs.service_id,si.name,rs.qty,rs.fee,rs.report FROM repair_services rs JOIN service_items si ON si.id=rs.service_id WHERE rs.repair_id=r.id) x),'[]') AS services,
	    COALESCE((SELECT json_agg(x ORDER BY x.id) FROM (
	      SELECT id,file,caption FROM repair_photos WHERE repair_id=r.id) x),'[]') AS photos,
	    COALESCE((SELECT json_agg(x ORDER BY x.id) FROM (
	      SELECT pr.id,pr.number,pr.status,pr.notes,pr.created_at,pr.review_notes,
	        COALESCE((SELECT json_agg(i) FROM (SELECT pri.id,pri.kind,pri.qty,COALESCE(pt.name,si.name,pri.custom_name) AS name
	          FROM part_request_items pri LEFT JOIN parts pt ON pt.id=pri.part_id LEFT JOIN service_items si ON si.id=pri.service_id
	          WHERE pri.request_id=pr.id ORDER BY pri.id) i),'[]') AS items
	      FROM part_requests pr WHERE pr.repair_id=r.id AND pr.deleted_at IS NULL) x),'[]') AS requests
	  FROM repairs r JOIN customers c ON c.id=r.customer_id LEFT JOIN products p ON p.id=r.product_id
	  WHERE r.id=$1 AND r.deleted_at IS NULL) t`, id)
	if err != nil || len(b) == 0 {
		return nil, httpx.ErrNotFound
	}
	return b, nil
}

// staffUsers returns users whose roles grant the permission (or everything).
type staffUser struct {
	ID    int64  `db:"id"`
	Phone string `db:"phone"`
}

func staffUsers(ctx context.Context, q db.Q, perm string) ([]staffUser, error) {
	return db.All[staffUser](ctx, q, `SELECT DISTINCT u.id,u.phone FROM users u
		JOIN user_roles ur ON ur.user_id=u.id JOIN role_permissions rp ON rp.role_code=ur.role_code
		WHERE u.is_active AND u.deleted_at IS NULL AND (rp.permission=$1 OR rp.permission='*')`, perm)
}

// notifyStaff tells everyone who can act on something: an in-app notification
// for all, an SMS (when a pattern is configured) for those with a phone.
func notifyStaff(ctx context.Context, q db.Q, perm, template, title, body, link string, smsArgs ...string) {
	users, err := staffUsers(ctx, q, perm)
	if err != nil {
		return
	}
	for _, u := range users {
		_, _ = q.Exec(ctx, `INSERT INTO notifications(user_id,kind,title,body,link) VALUES($1,'service',$2,$3,$4)`, u.ID, title, body, link)
		if template != "" && u.Phone != "" {
			_ = sms.Enqueue(ctx, q, u.Phone, template, smsArgs...)
		}
	}
}

func notifyUser(ctx context.Context, q db.Q, uid int64, title, body, link string) {
	_, _ = q.Exec(ctx, `INSERT INTO notifications(user_id,kind,title,body,link) VALUES($1,'service',$2,$3,$4)`, uid, title, body, link)
}

// touchRepair bumps updated_at so the pipeline measures "time in stage" from the last real event.
func touchRepair(ctx context.Context, q db.Q, repairID int64) {
	_, _ = q.Exec(ctx, `UPDATE repairs SET updated_at=NOW() WHERE id=$1`, repairID)
}

func creditLedger(ctx context.Context, tx pgx.Tx, tech int64, kind string, amount int64, refType string, refID int64, note string, by *int64) error {
	_, err := tx.Exec(ctx, `INSERT INTO technician_ledger(technician_id,kind,amount,ref_type,ref_id,note,created_by) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		tech, kind, amount, refType, refID, note, by)
	return err
}

func balance(ctx context.Context, q db.Q, tech int64) int64 {
	n, _ := db.Scalar[int64](ctx, q, `SELECT COALESCE(SUM(amount),0)::bigint FROM technician_ledger WHERE technician_id=$1`, tech)
	return n
}

func itemsOrErr(n int, msg string) error {
	if n == 0 {
		return httpx.BadRequest(msg)
	}
	return nil
}

func money(n int64) string { return fmt.Sprintf("%d", n) }
