// Package legacy imports backups of the three old systems into the unified
// schema. Imports are idempotent (legacy_map remembers old ids), run in one
// transaction, and can be rehearsed with DryRun.
package legacy

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"github.com/evinki/platform/internal/platform/db"
)

type Row map[string]any

type Importer struct {
	New    *db.DB
	Source string // guarantee | warehouse | attendance
	DryRun bool
	Ctx    context.Context
	// UploadDir receives copied images (warehouse part photos).
	UploadDir string
	Log    func(format string, a ...any)

	Stats map[string]int
	tx    pgx.Tx
	ctx   context.Context
}

func (im *Importer) logf(f string, a ...any) {
	if im.Log != nil {
		im.Log(f, a...)
	}
}
func (im *Importer) count(k string) { im.Stats[k]++ }

// OpenSource opens a legacy database: "sqlite:/path/file.db" or a postgres URL.
func OpenSource(dsn string) (*sql.DB, error) {
	if strings.HasPrefix(dsn, "sqlite:") {
		path := strings.ReplaceAll(strings.TrimPrefix(dsn, "sqlite:"), "\\", "/")
		// read-only on purpose: an import must never modify the old database
		return sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=query_only(1)")
	}
	return sql.Open("pgx", dsn)
}

// Run executes fn inside one transaction; DryRun rolls it back at the end.
func (im *Importer) Run(ctx context.Context, fn func() error) error {
	im.Stats = map[string]int{}
	im.ctx = ctx
	tx, err := im.New.Begin(ctx)
	if err != nil {
		return err
	}
	im.tx = tx
	if err = fn(); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	if im.DryRun {
		im.logf("dry run: rolling back")
		return tx.Rollback(ctx)
	}
	return tx.Commit(ctx)
}

func query(src *sql.DB, q string, args ...any) ([]Row, error) {
	rows, err := src.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", firstLine(q), err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var out []Row
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		r := Row{}
		for i, c := range cols {
			r[c] = vals[i]
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}

// tableExists lets an import skip tables an older backup does not have.
func tableExists(src *sql.DB, name string) bool {
	_, err := src.Query("SELECT 1 FROM " + name + " WHERE 1=0")
	return err == nil
}

// ───────── value conversion (sqlite and postgres return different Go types) ─────────

func S(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case time.Time:
		return t.Format(time.RFC3339)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	}
	return fmt.Sprint(v)
}

func I(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int32:
		return int64(t)
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case bool:
		if t {
			return 1
		}
	case []byte:
		n, _ := strconv.ParseInt(string(t), 10, 64)
		return n
	case string:
		n, _ := strconv.ParseInt(t, 10, 64)
		return n
	}
	return 0
}

func B(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return I(v) != 0
}

func F(v any) *float64 {
	switch t := v.(type) {
	case nil:
		return nil
	case float64:
		return &t
	case int64:
		f := float64(t)
		return &f
	}
	f, err := strconv.ParseFloat(S(v), 64)
	if err != nil {
		return nil
	}
	return &f
}

var timeLayouts = []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999", "2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05", "2006-01-02"}

func T(v any) *time.Time {
	switch t := v.(type) {
	case nil:
		return nil
	case time.Time:
		if t.IsZero() {
			return nil
		}
		return &t
	}
	s := strings.TrimSpace(S(v))
	if s == "" {
		return nil
	}
	for _, l := range timeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return &t
		}
	}
	return nil
}

func NullS(v any) *string {
	if v == nil {
		return nil
	}
	s := S(v)
	return &s
}

func ptrID(v any, m func(int64) (int64, bool)) *int64 {
	if v == nil {
		return nil
	}
	if id, ok := m(I(v)); ok {
		return &id
	}
	return nil
}

// ───────── id mapping ─────────

func (im *Importer) mapGet(entity string, legacyID any) (int64, bool) {
	var id int64
	err := im.tx.QueryRow(im.ctx, `SELECT new_id FROM legacy_map WHERE source=$1 AND entity=$2 AND legacy_id=$3`, im.Source, entity, S(legacyID)).Scan(&id)
	return id, err == nil
}

func (im *Importer) mapPut(entity string, legacyID any, newID int64) error {
	_, err := im.tx.Exec(im.ctx, `INSERT INTO legacy_map(source,entity,legacy_id,new_id) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, im.Source, entity, S(legacyID), newID)
	return err
}

// ensure inserts at most once per legacy id. fresh reports whether a row was created now.
func (im *Importer) ensure(entity string, legacyID any, insert func() (int64, error)) (id int64, fresh bool, err error) {
	if id, ok := im.mapGet(entity, legacyID); ok {
		im.Stats[entity+" (already imported)"]++
		return id, false, nil
	}
	id, err = insert()
	if err != nil {
		return 0, false, fmt.Errorf("%s %v: %w", entity, legacyID, err)
	}
	if err = im.mapPut(entity, legacyID, id); err != nil {
		return 0, false, err
	}
	im.count(entity)
	return id, true, nil
}

func (im *Importer) one(q string, args ...any) (int64, error) {
	var id int64
	err := im.tx.QueryRow(im.ctx, q, args...).Scan(&id)
	return id, err
}

func (im *Importer) exec(q string, args ...any) error {
	_, err := im.tx.Exec(im.ctx, q, args...)
	return err
}

// uniqueUsername avoids clashes when two old systems used the same login.
func (im *Importer) uniqueUsername(want string) (string, error) {
	want = strings.TrimSpace(want)
	if want == "" {
		want = "user"
	}
	name := want
	for i := 2; ; i++ {
		var n int
		if err := im.tx.QueryRow(im.ctx, `SELECT COUNT(*) FROM users WHERE lower(username)=lower($1) AND deleted_at IS NULL`, name).Scan(&n); err != nil {
			return "", err
		}
		if n == 0 {
			return name, nil
		}
		name = fmt.Sprintf("%s_%d", want, i)
	}
}

// upsertUser creates a user from a legacy account. Hashes in a foreign format
// go to legacy_hash and are upgraded at first login.
func (im *Importer) upsertUser(username, name, phone, email, hash string, active bool, roles ...string) (int64, error) {
	un, err := im.uniqueUsername(username)
	if err != nil {
		return 0, err
	}
	pw, legacy := "", hash
	if strings.HasPrefix(hash, "$2") {
		pw, legacy = hash, ""
	}
	id, err := im.one(`INSERT INTO users(username,full_name,phone,email,password_hash,legacy_hash,is_active) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		un, name, phone, email, pw, legacy, active)
	if err != nil {
		return 0, err
	}
	for _, r := range roles {
		if err := im.exec(`INSERT INTO user_roles(user_id,role_code) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, r); err != nil {
			return 0, err
		}
	}
	return id, nil
}

var normalizer = strings.NewReplacer("ي", "ی", "ك", "ک", "‌", " ", "‌", " ")

// NormName makes part names comparable across systems (Arabic/Persian letters, spacing).
func NormName(s string) string {
	return strings.Join(strings.Fields(normalizer.Replace(strings.ToLower(s))), " ")
}

// Ctx is optional; imports use context.Background() when it is nil.
func (im *Importer) ctxOrBackground() context.Context {
	if im.Ctx != nil {
		return im.Ctx
	}
	return context.Background()
}
