// Command evinkictl is the operator's tool: migrations, accounts, imports.
//
//	evinkictl migrate
//	evinkictl create-user  -username u -password p -roles admin,technical [-name N] [-phone P]
//	evinkictl reset-password -username u -password p
//	evinkictl import guarantee  -src postgres://...            [-dry-run]
//	evinkictl import warehouse  -src sqlite:/path/warehouse.db [-images /path/public/img/parts] [-dry-run]
//	evinkictl import attendance -src sqlite:/path/attendance.db | postgres://... [-dry-run]
//	evinkictl stats
//	evinkictl seed-catalog   (real products and photos of evinki.com)
//	evinkictl seed-demo      (catalogue + service items, shift, departments, standard survey)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/evinki/platform/internal/app"
	"github.com/evinki/platform/internal/bootstrap"
	"github.com/evinki/platform/internal/config"
	"github.com/evinki/platform/internal/legacy"
	"github.com/evinki/platform/internal/platform/audit"
	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/migrations"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	if err := run(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: evinkictl <migrate|create-user|reset-password|import|stats|seed-catalog|seed-demo> [flags]")
	os.Exit(2)
}

func deps(ctx context.Context) (*app.Deps, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	pool, err := db.Connect(ctx, cfg.DBURL)
	if err != nil {
		return nil, err
	}
	return &app.Deps{Cfg: cfg, DB: pool, Auth: auth.New(pool, cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL), Audit: &audit.Logger{DB: pool}}, nil
}

func run(cmd string, args []string) error {
	ctx := context.Background()
	d, err := deps(ctx)
	if err != nil {
		return err
	}
	defer d.DB.Close()
	logf := func(f string, a ...any) { fmt.Printf(f+"\n", a...) }

	switch cmd {
	case "migrate":
		if err := d.DB.Migrate(ctx, migrations.FS, func(s string) { fmt.Println(s) }); err != nil {
			return err
		}
		fmt.Println("database is up to date")
		return bootstrap.Run(ctx, d)

	case "create-user":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		u := fs.String("username", "", "")
		p := fs.String("password", "", "")
		name := fs.String("name", "", "")
		phone := fs.String("phone", "", "")
		roles := fs.String("roles", "admin", "comma separated role codes")
		_ = fs.Parse(args)
		if *u == "" || len(*p) < 8 {
			return fmt.Errorf("-username and a -password of at least 8 characters are required")
		}
		if err := d.Auth.SeedRoles(ctx); err != nil {
			return err
		}
		if err := bootstrap.CreateUser(ctx, d, *u, *name, *phone, *p, strings.Split(*roles, ","), false); err != nil {
			return err
		}
		fmt.Println("created", *u)
		return nil

	case "reset-password":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		u := fs.String("username", "", "")
		p := fs.String("password", "", "")
		_ = fs.Parse(args)
		if *u == "" || len(*p) < 8 {
			return fmt.Errorf("-username and a -password of at least 8 characters are required")
		}
		h, err := auth.HashPassword(*p)
		if err != nil {
			return err
		}
		tag, err := d.DB.Exec(ctx, `UPDATE users SET password_hash=$2, legacy_hash='', must_change_password=TRUE WHERE lower(username)=lower($1) AND deleted_at IS NULL`, *u, h)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("no such user: %s", *u)
		}
		_, _ = d.DB.Exec(ctx, `UPDATE refresh_tokens SET revoked_at=NOW() WHERE user_id=(SELECT id FROM users WHERE lower(username)=lower($1)) AND revoked_at IS NULL`, *u)
		fmt.Println("password reset; the user must change it at next sign-in")
		return nil

	case "import":
		if len(args) < 1 {
			return fmt.Errorf("import needs a source: guarantee | warehouse | attendance")
		}
		kind := args[0]
		fs := flag.NewFlagSet("import "+kind, flag.ExitOnError)
		srcDSN := fs.String("src", "", "legacy database: postgres URL or sqlite:/path")
		images := fs.String("images", "", "warehouse: folder with the part photos (public/img/parts)")
		dry := fs.Bool("dry-run", false, "run everything, then roll back")
		_ = fs.Parse(args[1:])
		if *srcDSN == "" {
			return fmt.Errorf("-src is required")
		}
		src, err := legacy.OpenSource(*srcDSN)
		if err != nil {
			return err
		}
		defer src.Close()
		if err := src.Ping(); err != nil {
			return fmt.Errorf("cannot open the source database: %w", err)
		}
		if err := d.Auth.SeedRoles(ctx); err != nil {
			return err
		}
		if err := bootstrap.Run(ctx, d); err != nil {
			return err
		}
		im := &legacy.Importer{New: d.DB, DryRun: *dry, Log: logf, UploadDir: d.Cfg.UploadDir, Ctx: ctx}
		switch kind {
		case "guarantee":
			err = im.ImportGuarantee(src)
		case "warehouse":
			err = im.ImportWarehouse(src, *images)
		case "attendance":
			err = im.ImportAttendance(src)
		default:
			return fmt.Errorf("unknown source %q", kind)
		}
		if err != nil {
			return err
		}
		fmt.Println("\nimported:")
		for k, v := range im.Stats {
			fmt.Printf("  %-45s %d\n", k, v)
		}
		if *dry {
			fmt.Println("\n(dry run: nothing was saved)")
		}
		return nil

	case "stats":
		for _, t := range []string{"users", "products", "parts", "customers", "guarantees", "repairs", "part_requests", "shipments", "orders", "stock_documents", "employees", "attendance_records", "surveys"} {
			n, _ := db.Scalar[int](ctx, d.DB, "SELECT COUNT(*) FROM "+t)
			fmt.Printf("%-20s %d\n", t, n)
		}
		return nil

	case "seed-demo":
		return seedDemo(ctx, d)

	case "seed-catalog":
		return seedCatalog(ctx, d)
	}
	usage()
	return nil
}
