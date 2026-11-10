// Package warehouse: stock levels, movement documents, approvals, counts and
// exports. engine.go is the part other modules call (service uses it to move
// parts to technicians and to consume them on repairs).
package warehouse

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
	"github.com/evinki/platform/internal/platform/settings"
)

type Reason struct {
	Key       string `json:"key"`
	Direction string `json:"direction"`
	Label     string `json:"label"`
	Sepidar   string `json:"sepidar"`    // Sepidar document group; "" = never exported
	NeedsDest bool   `json:"needs_dest"`
	Internal  bool   `json:"internal"`   // produced by workflows, not selectable by hand
}

// Reasons is the single source of truth, also served to the apps via /wh/meta.
var Reasons = []Reason{
	{"recharge", "in", "ورود جدید / خرید (خرید داخلی)", "purchase", false, false},
	{"return_from_worker", "in", "بازگشت از کارگر / خط تولید (تولید)", "production", false, false},
	{"other_in", "in", "سایر ورودی‌ها", "other_in", false, false},
	{"opening", "in", "موجودی اولیه", "", false, false},
	{"adjust_in", "in", "اصلاح شمارش (افزایش)", "", false, false},
	{"to_worker", "out", "تحویل به کارگر (مصرف)", "consumption", false, false},
	{"sale", "out", "فروش", "sale", false, false},
	{"scrap", "out", "ضایعات (انتقال به انبار ضایعات)", "transfer", true, false},
	{"transfer", "out", "انتقال به انبار دیگر", "transfer", true, false},
	{"other_out", "out", "سایر خروجی‌ها", "other_out", false, false},
	{"adjust_out", "out", "اصلاح شمارش (کاهش)", "", false, false},
	// workflow reasons
	{"tech_delivery", "out", "تحویل قطعه به تکنسین", "", true, true},
	{"repair_use", "out", "مصرف در تعمیر", "", false, true},
	{"return_old", "in", "دریافت قطعه قدیمی از تکنسین", "", false, true},
}

func reasonByKey(k string) (Reason, bool) {
	for _, r := range Reasons {
		if r.Key == k {
			return r, true
		}
	}
	return Reason{}, false
}

type Line struct {
	PartID int64  `json:"part_id"`
	Qty    int    `json:"qty"`
	Note   string `json:"note"`
}

type DocIn struct {
	Direction   string
	Reason      string
	WarehouseID int64
	DestID      *int64
	WorkerName  string
	Note        string
	Approved    bool // apply immediately
	SourceType  string
	SourceID    *int64
	CreatedBy   *int64
	ClientRef   *string
	Lines       []Line
}

// CentralID returns the main warehouse (kind 'central', lowest id).
func CentralID(ctx context.Context, q db.Q) (int64, error) {
	return db.Scalar[int64](ctx, q, `SELECT id FROM warehouses WHERE kind='central' AND is_active ORDER BY id LIMIT 1`)
}

// KindID returns the first active warehouse of a kind (scrap, returns).
func KindID(ctx context.Context, q db.Q, kind string) (int64, error) {
	return db.Scalar[int64](ctx, q, `SELECT id FROM warehouses WHERE kind=$1 AND is_active ORDER BY id LIMIT 1`, kind)
}

// TechWarehouse returns (creating on first use) a technician's own warehouse.
func TechWarehouse(ctx context.Context, q db.Q, techUserID int64) (int64, error) {
	id, err := db.Scalar[int64](ctx, q, `SELECT id FROM warehouses WHERE technician_user_id=$1`, techUserID)
	if err == nil {
		return id, nil
	}
	if err != db.ErrNotFound {
		return 0, err
	}
	return db.Scalar[int64](ctx, q, `INSERT INTO warehouses(code,title,kind,technician_user_id)
		SELECT 'T'||u.id, 'انبار '||COALESCE(NULLIF(u.full_name,''),u.username), 'technician', u.id FROM users u WHERE u.id=$1
		ON CONFLICT DO NOTHING RETURNING id`, techUserID)
}

// CreateDoc inserts a document and, when Approved, applies it - all inside tx.
func CreateDoc(ctx context.Context, tx pgx.Tx, in DocIn) (int64, error) {
	if len(in.Lines) == 0 {
		return 0, httpx.BadRequest("حداقل یک ردیف لازم است")
	}
	rs, ok := reasonByKey(in.Reason)
	if !ok || rs.Direction != in.Direction {
		return 0, httpx.BadRequest("علت حرکت با جهت آن سازگار نیست")
	}
	if rs.NeedsDest && in.DestID == nil {
		return 0, httpx.BadRequest("انبار مقصد را انتخاب کنید")
	}
	if in.DestID != nil && *in.DestID == in.WarehouseID {
		return 0, httpx.BadRequest("انبار مبدا و مقصد یکی است")
	}
	status := "pending"
	if in.Approved {
		status = "approved"
	}
	var wid *int64
	if in.WorkerName != "" {
		id, err := db.Scalar[int64](ctx, tx, `INSERT INTO warehouse_workers(name) VALUES($1) ON CONFLICT (name) DO UPDATE SET name=EXCLUDED.name RETURNING id`, in.WorkerName)
		if err != nil {
			return 0, err
		}
		wid = &id
	}
	id, err := db.Scalar[int64](ctx, tx, `INSERT INTO stock_documents(direction,reason,warehouse_id,dest_warehouse_id,worker_id,worker_name,note,status,
		source_type,source_id,created_by,client_ref,decided_by,decided_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,CASE WHEN $8::text='approved' THEN $11::bigint END,CASE WHEN $8::text='approved' THEN NOW() END) RETURNING id`,
		in.Direction, in.Reason, in.WarehouseID, in.DestID, wid, in.WorkerName, in.Note, status,
		nz(in.SourceType, "manual"), in.SourceID, in.CreatedBy, in.ClientRef)
	if err != nil {
		return 0, err
	}
	for _, l := range in.Lines {
		if l.Qty <= 0 {
			return 0, httpx.BadRequest("تعداد باید بزرگتر از صفر باشد")
		}
		if _, err = tx.Exec(ctx, `INSERT INTO stock_document_lines(document_id,part_id,qty,note) VALUES($1,$2,$3,$4)`, id, l.PartID, l.Qty, l.Note); err != nil {
			return 0, err
		}
	}
	if in.Approved {
		if err = applyLines(ctx, tx, id); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// ApplyPending approves a pending document: re-checks stock and moves it.
func ApplyPending(ctx context.Context, tx pgx.Tx, docID, by int64, note string) error {
	tag, err := tx.Exec(ctx, `UPDATE stock_documents SET status='approved', decided_by=$2, decided_at=NOW(), decision_note=$3
		WHERE id=$1 AND status='pending' AND deleted_at IS NULL`, docID, by, note)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.Conflict("این سند در انتظار تایید نیست")
	}
	return applyLines(ctx, tx, docID)
}

func applyLines(ctx context.Context, tx pgx.Tx, docID int64) error {
	type doc struct {
		Dir  string `db:"direction"`
		WH   int64  `db:"warehouse_id"`
		Dest *int64 `db:"dest_warehouse_id"`
	}
	d, err := db.One[doc](ctx, tx, `SELECT direction,warehouse_id,dest_warehouse_id FROM stock_documents WHERE id=$1`, docID)
	if err != nil {
		return err
	}
	check := settings.Bool(ctx, tx, "wh_check_stock")
	type ln struct {
		ID     int64  `db:"id"`
		PartID int64  `db:"part_id"`
		Qty    int    `db:"qty"`
		Name   string `db:"name"`
	}
	lines, err := db.All[ln](ctx, tx, `SELECT l.id,l.part_id,l.qty,p.name FROM stock_document_lines l JOIN parts p ON p.id=l.part_id WHERE l.document_id=$1 ORDER BY l.part_id`, docID)
	if err != nil {
		return err
	}
	for _, l := range lines {
		// lock rows in part order so concurrent documents cannot deadlock
		if _, err = tx.Exec(ctx, `INSERT INTO stock_levels(warehouse_id,part_id,qty) VALUES($1,$2,0) ON CONFLICT DO NOTHING`, d.WH, l.PartID); err != nil {
			return err
		}
		before, err := db.Scalar[int](ctx, tx, `SELECT qty FROM stock_levels WHERE warehouse_id=$1 AND part_id=$2 FOR UPDATE`, d.WH, l.PartID)
		if err != nil {
			return err
		}
		delta := l.Qty
		if d.Dir == "out" {
			delta = -l.Qty
			if check && before < l.Qty {
				return httpx.Conflict(fmt.Sprintf("موجودی «%s» کافی نیست (موجود: %d، درخواست: %d)", l.Name, before, l.Qty))
			}
		}
		after := before + delta
		if _, err = tx.Exec(ctx, `UPDATE stock_levels SET qty=$3, updated_at=NOW() WHERE warehouse_id=$1 AND part_id=$2`, d.WH, l.PartID, after); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE stock_document_lines SET stock_before=$2, stock_after=$3 WHERE id=$1`, l.ID, before, after); err != nil {
			return err
		}
		if d.Dest != nil {
			if _, err = tx.Exec(ctx, `INSERT INTO stock_levels(warehouse_id,part_id,qty) VALUES($1,$2,$3)
				ON CONFLICT (warehouse_id,part_id) DO UPDATE SET qty=stock_levels.qty+EXCLUDED.qty, updated_at=NOW()`, *d.Dest, l.PartID, l.Qty); err != nil {
				return err
			}
		}
	}
	return nil
}

// Reverse undoes an approved document's effect (used when staff delete it).
func Reverse(ctx context.Context, tx pgx.Tx, docID int64) error {
	type doc struct {
		Dir  string `db:"direction"`
		WH   int64  `db:"warehouse_id"`
		Dest *int64 `db:"dest_warehouse_id"`
	}
	d, err := db.One[doc](ctx, tx, `SELECT direction,warehouse_id,dest_warehouse_id FROM stock_documents WHERE id=$1`, docID)
	if err != nil {
		return err
	}
	sign := 1 // an 'out' document returns the goods to its warehouse
	if d.Dir == "in" {
		sign = -1
	}
	rows, err := tx.Query(ctx, `SELECT part_id, qty FROM stock_document_lines WHERE document_id=$1`, docID)
	if err != nil {
		return err
	}
	type pq struct {
		part int64
		qty  int
	}
	var list []pq
	for rows.Next() {
		var x pq
		if err := rows.Scan(&x.part, &x.qty); err != nil {
			return err
		}
		list = append(list, x)
	}
	rows.Close()
	for _, x := range list {
		if _, err = tx.Exec(ctx, `INSERT INTO stock_levels(warehouse_id,part_id,qty) VALUES($1,$2,$3)
			ON CONFLICT (warehouse_id,part_id) DO UPDATE SET qty=stock_levels.qty+EXCLUDED.qty, updated_at=NOW()`, d.WH, x.part, sign*x.qty); err != nil {
			return err
		}
		if d.Dest != nil {
			if _, err = tx.Exec(ctx, `UPDATE stock_levels SET qty=qty-$3, updated_at=NOW() WHERE warehouse_id=$1 AND part_id=$2`, *d.Dest, x.part, x.qty); err != nil {
				return err
			}
		}
	}
	return nil
}

// Stock returns the quantity of a part in a warehouse.
func Stock(ctx context.Context, q db.Q, wh, part int64) int {
	n, _ := db.Scalar[int](ctx, q, `SELECT qty FROM stock_levels WHERE warehouse_id=$1 AND part_id=$2`, wh, part)
	return n
}

func nz(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
