// Package bootstrap prepares a fresh database: roles, SMS patterns, the
// standard warehouses and the first administrator. Everything is idempotent.
package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/evinki/platform/internal/app"
	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/sms"
)

func Run(ctx context.Context, d *app.Deps) error {
	if err := d.Auth.SeedRoles(ctx); err != nil {
		return fmt.Errorf("roles: %w", err)
	}
	if err := sms.SeedTemplates(ctx, d.DB); err != nil {
		return fmt.Errorf("sms templates: %w", err)
	}
	for _, w := range []struct{ code, title, kind string }{
		{"1001", "انبار مرکزی قطعات", "central"},
		{"1004", "انبار ضایعات", "scrap"},
		{"RET", "انبار قطعات برگشتی", "returns"},
		{"1002", "انبار محصول", "products"},
	} {
		if _, err := d.DB.Exec(ctx, `INSERT INTO warehouses(code,title,kind) VALUES($1,$2,$3) ON CONFLICT (code) DO NOTHING`, w.code, w.title, w.kind); err != nil {
			return err
		}
	}
	n, err := db.Scalar[int](ctx, d.DB, `SELECT COUNT(*) FROM users WHERE deleted_at IS NULL`)
	if err != nil {
		return err
	}
	if n == 0 {
		pw := d.Cfg.BootstrapAdminPass
		generated := false
		if pw == "" {
			pw, generated = auth.RandToken(12), true
		}
		if err := CreateUser(ctx, d, d.Cfg.BootstrapAdminUser, "مدیر ارشد", "", pw, []string{"super_admin"}, true); err != nil {
			return err
		}
		if generated {
			// shown once in the container log; the user must change it at first login
			slog.Warn("FIRST RUN: administrator created", "username", d.Cfg.BootstrapAdminUser, "password", pw)
		} else {
			slog.Info("administrator created from BOOTSTRAP_ADMIN_PASSWORD", "username", d.Cfg.BootstrapAdminUser)
		}
	}
	return nil
}

// CreateUser inserts a user with roles.
func CreateUser(ctx context.Context, d *app.Deps, username, name, phone, password string, roles []string, mustChange bool) error {
	h, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	return d.DB.Tx(ctx, func(tx pgxTx) error {
		id, err := db.Scalar[int64](ctx, tx, `INSERT INTO users(username,full_name,phone,password_hash,must_change_password) VALUES($1,$2,$3,$4,$5) RETURNING id`,
			username, name, phone, h, mustChange)
		if err != nil {
			return err
		}
		for _, r := range roles {
			if _, err = tx.Exec(ctx, `INSERT INTO user_roles(user_id,role_code) VALUES($1,$2)`, id, r); err != nil {
				return err
			}
		}
		return nil
	})
}
