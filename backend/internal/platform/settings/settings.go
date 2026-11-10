// Package settings reads the key/value settings table with typed defaults.
package settings

import (
	"context"
	"encoding/json"

	"github.com/evinki/platform/internal/platform/db"
)

// Defaults are what a fresh install uses; the settings screen overrides them.
var Defaults = map[string]any{
	"company_name":            "اوینکی",
	"company_phone":           "028-33362824",
	"currency_label":          "تومان",
	"shipping_fee":            0,       // flat fee per order
	"free_shipping_over":      0,       // 0 = never free
	"wh_check_stock":          true,    // refuse outgoing lines larger than stock
	"wh_approval_out":         true,    // storekeeper outgoing documents wait for approval
	"wh_approval_in":          false,
	"wh_count_blind":          true,    // hide system quantity while counting
	"service_auto_survey":     true,
	"guarantee_auto_approve_orders": true,
	"hr_face_enabled":         true,
	"hr_fingerprint_enabled":  false,
	"hr_pin_enabled":          true,
	"hr_liveness":             true,
	"hr_face_threshold":       0.42, // cosine similarity needed to accept a face
	"hr_face_margin":          0.04, // best match must beat the runner-up by this much
	"hr_punch_cooldown":       60,   // seconds before the same person can punch again on a kiosk
	"sepidar_main_warehouse":  "1001",
}

func Raw(ctx context.Context, q db.Q) (map[string]json.RawMessage, error) {
	type row struct {
		Key   string          `db:"key"`
		Value json.RawMessage `db:"value"`
	}
	rows, err := db.All[row](ctx, q, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	out := map[string]json.RawMessage{}
	for _, r := range rows {
		out[r.Key] = r.Value
	}
	return out, nil
}

// All merges stored values over the defaults.
func All(ctx context.Context, q db.Q) (map[string]any, error) {
	raw, err := Raw(ctx, q)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	for k, v := range Defaults {
		out[k] = v
	}
	for k, v := range raw {
		var x any
		if json.Unmarshal(v, &x) == nil {
			out[k] = x
		}
	}
	return out, nil
}

func Int(ctx context.Context, q db.Q, key string) int64 {
	m, err := All(ctx, q)
	if err != nil {
		return 0
	}
	if f, ok := m[key].(float64); ok {
		return int64(f)
	}
	if i, ok := m[key].(int); ok {
		return int64(i)
	}
	return 0
}

func Bool(ctx context.Context, q db.Q, key string) bool {
	m, err := All(ctx, q)
	if err != nil {
		return false
	}
	b, _ := m[key].(bool)
	return b
}

func Set(ctx context.Context, q db.Q, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO settings(key,value) VALUES($1,$2)
		ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value, updated_at=NOW()`, key, b)
	return err
}
