package legacy

import (
	"database/sql"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

// ImportWarehouse loads the SQLite database of WareHouseManager. imagesDir is
// the old `public/img/parts` folder; its photos are copied into the uploads
// volume. QR tokens are kept, so labels that are already printed stay valid.
func (im *Importer) ImportWarehouse(src *sql.DB, imagesDir string) error {
	im.Source = "warehouse"
	return im.Run(im.ctxOrBackground(), func() error {
		central, err := im.one(`SELECT id FROM warehouses WHERE kind='central' ORDER BY id LIMIT 1`)
		if err != nil {
			return fmt.Errorf("run the server once first so the standard warehouses exist: %w", err)
		}
		im.logf("importing users ...")
		users, err := query(src, `SELECT * FROM users`)
		if err != nil {
			return err
		}
		for _, r := range users {
			role := "storekeeper"
			if S(r["role"]) == "admin" {
				role = "admin"
			}
			roles := []string{role}
			if B(r["super_admin"]) {
				roles = []string{"super_admin"}
			}
			if _, _, err := im.ensure("user", r["id"], func() (int64, error) {
				id, err := im.upsertUser(S(r["username"]), S(r["display_name"]), "", "", S(r["password_hash"]), B(r["active"]), roles...)
				if err != nil {
					return 0, err
				}
				// force a new password: the old ones were generated and may sit in a text file
				return id, im.exec(`UPDATE users SET must_change_password=$2 WHERE id=$1`, id, B(r["must_change_password"]))
			}); err != nil {
				return err
			}
		}

		im.logf("importing parts ...")
		items, err := query(src, `SELECT * FROM items`)
		if err != nil {
			return err
		}
		for _, r := range items {
			img := im.copyImage(imagesDir, S(r["image"]))
			qr := strings.TrimPrefix(S(r["qr_token"]), "WH:")
			if _, _, err := im.ensure("part", r["id"], func() (int64, error) {
				id, err := im.one(`INSERT INTO parts(code,sepidar_code,part_no,name,variant,material,weight_g,unit,location,min_stock,image,note,row_no,qr_token,is_active,created_at)
					VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,COALESCE($16,NOW())) RETURNING id`,
					S(r["code"]), S(r["sepidar_code"]), S(r["part_no"]), S(r["name"]), S(r["variant"]), S(r["material"]), F(r["weight_g"]), nz(S(r["unit"]), "عدد"),
					S(r["location"]), I(r["min_stock"]), img, S(r["note"]), nullInt(r["row_no"]), qr, B(r["active"]), T(r["created_at"]))
				if err != nil {
					return 0, err
				}
				// opening balance = the stock the old system showed; its document history is imported below for reference
				return id, im.exec(`INSERT INTO stock_levels(warehouse_id,part_id,qty) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, central, id, I(r["stock"]))
			}); err != nil {
				return err
			}
		}

		for _, w := range mustRows(query(src, `SELECT * FROM workers`)) {
			if err := im.exec(`INSERT INTO warehouse_workers(name,active) VALUES($1,$2) ON CONFLICT (name) DO NOTHING`, S(w["name"]), B(w["active"])); err != nil {
				return err
			}
		}

		im.logf("importing stock documents ...")
		docs, err := query(src, `SELECT * FROM documents ORDER BY id`)
		if err != nil {
			return err
		}
		maxNo := int64(0)
		for _, r := range docs {
			status := S(r["status"])
			if r["deleted_at"] != nil {
				status = "cancelled"
			}
			by := ptrID(r["created_by"], func(i int64) (int64, bool) { return im.mapGet("user", i) })
			dec := ptrID(r["decided_by"], func(i int64) (int64, bool) { return im.mapGet("user", i) })
			reason := S(r["reason"])
			if !validReason(reason) {
				reason = map[string]string{"in": "other_in", "out": "other_out"}[S(r["direction"])]
			}
			no := I(r["doc_no"])
			if no > maxNo {
				maxNo = no
			}
			docID, fresh, err := im.ensure("document", r["id"], func() (int64, error) {
				return im.one(`INSERT INTO stock_documents(doc_no,direction,reason,warehouse_id,worker_name,note,status,source_type,created_by,created_at,decided_by,decided_at,decision_note,client_ref,deleted_at,delete_note)
					VALUES($1,$2,$3,$4,$5,$6,$7,'manual',$8,COALESCE($9,NOW()),$10,$11,$12,$13,$14,$15) RETURNING id`,
					docNo(no, I(r["id"])), S(r["direction"]), reason, central, S(r["worker_name"]), S(r["note"]), status, by, T(r["created_at"]), dec, T(r["decided_at"]),
					S(r["decision_note"]), NullS(r["client_ref"]), T(r["deleted_at"]), S(r["delete_note"]))
			})
			if err != nil {
				return err
			}
			if !fresh {
				continue
			}
			lines, err := query(src, `SELECT * FROM document_lines WHERE document_id=?`, r["id"])
			if err != nil {
				return err
			}
			for _, l := range lines {
				part, ok := im.mapGet("part", l["item_id"])
				if !ok || I(l["qty"]) <= 0 {
					continue
				}
				if err := im.exec(`INSERT INTO stock_document_lines(document_id,part_id,qty,note,stock_before,stock_after) VALUES($1,$2,$3,$4,$5,$6)`,
					docID, part, I(l["qty"]), S(l["note"]), nullInt(l["stock_before"]), nullInt(l["stock_after"])); err != nil {
					return err
				}
			}
		}
		// new documents continue the old numbering
		return im.exec(`SELECT setval('stock_doc_no_seq', GREATEST($1, (SELECT COALESCE(MAX(doc_no),0) FROM stock_documents)), true)`, maxNo)
	})
}

func docNo(no, fallback int64) int64 {
	if no > 0 {
		return no
	}
	return 1_000_000 + fallback
}

func validReason(r string) bool {
	switch r {
	case "recharge", "return_from_worker", "other_in", "opening", "adjust_in", "to_worker", "sale", "scrap", "transfer", "other_out", "adjust_out":
		return true
	}
	return false
}

func nullInt(v any) *int64 {
	if v == nil {
		return nil
	}
	n := I(v)
	return &n
}

func mustRows(r []Row, err error) []Row {
	if err != nil {
		return nil
	}
	return r
}

// copyImage copies a part photo into the uploads folder and registers it.
func (im *Importer) copyImage(dir, name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if im.DryRun || dir == "" || name == "" || name == "." {
		return ""
	}
	in, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return ""
	}
	defer in.Close()
	if im.UploadDir == "" {
		return ""
	}
	_ = os.MkdirAll(im.UploadDir, 0o755)
	dst := "legacy-" + name
	out, err := os.Create(filepath.Join(im.UploadDir, dst))
	if err != nil {
		return ""
	}
	defer out.Close()
	n, _ := io.Copy(out, in)
	var id string
	if err := im.tx.QueryRow(im.ctx, `INSERT INTO files(path,original,mime,size_bytes,purpose) VALUES($1,$2,$3,$4,'part') RETURNING id::text`,
		dst, name, mime.TypeByExtension(filepath.Ext(name)), n).Scan(&id); err != nil {
		return ""
	}
	return "/api/v1/files/" + id
}
