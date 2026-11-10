// Package crud mounts list/get/create/patch/delete endpoints for a table from
// a short declaration. Rows are serialised by Postgres (row_to_json), so
// simple entities need no Go structs. Anything with real rules (stock moves,
// approvals, payments) is written by hand in its module instead.
package crud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/evinki/platform/internal/platform/audit"
	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
)

type Kind string

const (
	Text  Kind = "text"
	Int   Kind = "int"
	Float Kind = "float"
	Bool  Kind = "bool"
	JSON  Kind = "json"
	Date  Kind = "date"
	TS    Kind = "ts"
)

type Body map[string]json.RawMessage

type Resource struct {
	Name         string // used in audit entries
	Table        string
	Select       string          // output columns; default "*"
	Cols         map[string]Kind // writable columns
	Required     []string        // must be present and non-empty on create
	Search       []string        // columns matched by ?q=
	Filters      map[string]string // ?param=value -> column (compared as text)
	Order        string
	SoftDelete   bool
	TouchUpdated bool
	// Defaults are written on create when the client did not send the column.
	Defaults map[string]any
	// Check runs after decoding, before writing. Return an error to refuse.
	Check func(r *http.Request, create bool, b Body) error
	// After runs after a successful write (in addition to the audit entry).
	After func(r *http.Request, id int64, action string)

	DB    *db.DB
	Audit *audit.Logger
}

// Perms names who may do what. Empty Delete falls back to Write.
type Perms struct{ Read, Write, Delete string }

func (res *Resource) Mount(r chi.Router, path string, p Perms) {
	if p.Delete == "" {
		p.Delete = p.Write
	}
	r.With(auth.Require(p.Read)).Get(path, httpx.H(res.list))
	r.With(auth.Require(p.Read)).Get(path+"/{id}", httpx.H(res.get))
	r.With(auth.Require(p.Write)).Post(path, httpx.H(res.create))
	r.With(auth.Require(p.Write)).Patch(path+"/{id}", httpx.H(res.patch))
	r.With(auth.Require(p.Delete)).Delete(path+"/{id}", httpx.H(res.remove))
}

func (res *Resource) sel() string {
	if res.Select == "" {
		return "*"
	}
	return res.Select
}

func (res *Resource) alive() string {
	if res.SoftDelete {
		return res.Table + ".deleted_at IS NULL"
	}
	return "TRUE"
}

func (res *Resource) list(w http.ResponseWriter, r *http.Request) error {
	where := []string{res.alive()}
	var args []any
	arg := func(v any) string { args = append(args, v); return "$" + strconv.Itoa(len(args)) }

	if q := httpx.Q(r, "q"); q != "" && len(res.Search) > 0 {
		ph := arg("%" + q + "%")
		var ors []string
		for _, c := range res.Search {
			ors = append(ors, c+"::text ILIKE "+ph)
		}
		where = append(where, "("+strings.Join(ors, " OR ")+")")
	}
	for param, col := range res.Filters {
		if v := httpx.Q(r, param); v != "" {
			where = append(where, col+"::text = "+arg(v))
		}
	}
	cond := strings.Join(where, " AND ")
	ctx := r.Context()
	total, err := db.Scalar[int](ctx, res.DB, fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE %s`, res.Table, cond), args...)
	if err != nil {
		return err
	}
	pg := httpx.Paging(r, 500)
	order := res.Order
	if order == "" {
		order = res.Table + ".id DESC"
	}
	sql := fmt.Sprintf(`SELECT COALESCE(json_agg(t),'[]'::json) FROM (SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT %s OFFSET %s) t`,
		res.sel(), res.Table, cond, order, arg(pg.Limit), arg(pg.Offset))
	raw, err := db.Scalar[json.RawMessage](ctx, res.DB, sql, args...)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fmt.Fprintf(w, `{"items":%s,"total":%d,"page":%d,"limit":%d}`, raw, total, pg.Page, pg.Limit)
	return nil
}

func (res *Resource) one(ctx context.Context, id int64) (json.RawMessage, error) {
	raw, err := db.Scalar[json.RawMessage](ctx, res.DB, fmt.Sprintf(
		`SELECT row_to_json(t) FROM (SELECT %s FROM %s WHERE %s.id=$1 AND %s) t`, res.sel(), res.Table, res.Table, res.alive()), id)
	if err == db.ErrNotFound || len(raw) == 0 {
		return nil, httpx.ErrNotFound
	}
	return raw, err
}

func (res *Resource) get(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	raw, err := res.one(r.Context(), id)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(raw)
	return nil
}

func (res *Resource) decode(r *http.Request) (Body, error) {
	var b Body
	if err := httpx.Decode(r, &b); err != nil {
		return nil, err
	}
	for k := range b {
		if _, ok := res.Cols[k]; !ok {
			return nil, httpx.BadRequest("فیلد مجاز نیست: " + k)
		}
	}
	return b, nil
}

func (res *Resource) create(w http.ResponseWriter, r *http.Request) error {
	b, err := res.decode(r)
	if err != nil {
		return err
	}
	for _, c := range res.Required {
		v, ok := b[c]
		if !ok || isEmpty(v) {
			return httpx.BadRequest("فیلد الزامی است: " + c)
		}
	}
	if res.Check != nil {
		if err := res.Check(r, true, b); err != nil {
			return err
		}
	}
	cols, ph, args, err := res.values(b)
	if err != nil {
		return err
	}
	for c, v := range res.Defaults {
		if _, sent := b[c]; !sent {
			cols = append(cols, c)
			args = append(args, v)
			ph = append(ph, "$"+strconv.Itoa(len(args)))
		}
	}
	if len(cols) == 0 {
		return httpx.BadRequest("اطلاعاتی ارسال نشده است")
	}
	id, err := db.Scalar[int64](r.Context(), res.DB, fmt.Sprintf(`INSERT INTO %s(%s) VALUES(%s) RETURNING id`,
		res.Table, strings.Join(cols, ","), strings.Join(ph, ",")), args...)
	if err != nil {
		return err
	}
	res.Audit.Log(r, res.Name+".create", res.Name, id, "")
	if res.After != nil {
		res.After(r, id, "create")
	}
	raw, err := res.one(r.Context(), id)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write(raw)
	return nil
}

func (res *Resource) patch(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	b, err := res.decode(r)
	if err != nil {
		return err
	}
	if res.Check != nil {
		if err := res.Check(r, false, b); err != nil {
			return err
		}
	}
	cols, ph, args, err := res.values(b)
	if err != nil {
		return err
	}
	if len(cols) > 0 {
		sets := make([]string, len(cols))
		for i := range cols {
			sets[i] = cols[i] + "=" + ph[i]
		}
		if res.TouchUpdated {
			sets = append(sets, "updated_at=NOW()")
		}
		args = append(args, id)
		tag, err := res.DB.Exec(r.Context(), fmt.Sprintf(`UPDATE %s SET %s WHERE id=$%d AND %s`,
			res.Table, strings.Join(sets, ","), len(args), res.alive()), args...)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return httpx.ErrNotFound
		}
		res.Audit.Log(r, res.Name+".update", res.Name, id, strings.Join(cols, ","))
		if res.After != nil {
			res.After(r, id, "update")
		}
	}
	raw, err := res.one(r.Context(), id)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(raw)
	return nil
}

func (res *Resource) remove(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var sql string
	if res.SoftDelete {
		sql = fmt.Sprintf(`UPDATE %s SET deleted_at=NOW() WHERE id=$1 AND deleted_at IS NULL`, res.Table)
	} else {
		sql = fmt.Sprintf(`DELETE FROM %s WHERE id=$1`, res.Table)
	}
	tag, err := res.DB.Exec(r.Context(), sql, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.ErrNotFound
	}
	res.Audit.Log(r, res.Name+".delete", res.Name, id, "")
	if res.After != nil {
		res.After(r, id, "delete")
	}
	httpx.NoContent(w)
	return nil
}

// values converts the JSON body into SQL columns, placeholders and arguments.
func (res *Resource) values(b Body) (cols, ph []string, args []any, err error) {
	for col, raw := range b {
		kind := res.Cols[col]
		v, cast, e := convert(kind, raw)
		if e != nil {
			return nil, nil, nil, httpx.BadRequest(fmt.Sprintf("مقدار فیلد %s نامعتبر است", col))
		}
		args = append(args, v)
		cols = append(cols, col)
		ph = append(ph, "$"+strconv.Itoa(len(args))+cast)
	}
	return
}

func isEmpty(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s == "null" || s == `""` || s == ""
}

func convert(kind Kind, raw json.RawMessage) (any, string, error) {
	null := strings.TrimSpace(string(raw)) == "null"
	switch kind {
	case Text:
		if null {
			return "", "", nil
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, "", err
		}
		return strings.TrimSpace(s), "", nil
	case Int:
		if null {
			return nil, "", nil
		}
		var s string
		if json.Unmarshal(raw, &s) != nil {
			s = strings.TrimSpace(string(raw))
		}
		s = strings.ReplaceAll(strings.ReplaceAll(httpx.Digits(strings.TrimSpace(s)), ",", ""), "٬", "")
		if s == "" {
			return nil, "", nil
		}
		n, err := strconv.ParseInt(s, 10, 64)
		return n, "", err
	case Float:
		if null {
			return nil, "", nil
		}
		var s string
		if json.Unmarshal(raw, &s) != nil {
			s = strings.TrimSpace(string(raw))
		}
		s = httpx.Digits(strings.TrimSpace(s))
		if s == "" {
			return nil, "", nil
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, "", err
	case Bool:
		var v bool
		return v, "", json.Unmarshal(raw, &v)
	case JSON:
		if null {
			return "null", "::jsonb", nil
		}
		return string(raw), "::jsonb", nil
	case Date:
		if null {
			return nil, "::date", nil
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, "", err
		}
		s = httpx.Digits(strings.TrimSpace(s))
		if s == "" {
			return nil, "::date", nil
		}
		if len(s) > 10 {
			s = s[:10]
		}
		return s, "::date", nil
	case TS:
		if null {
			return nil, "::timestamptz", nil
		}
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, "", err
		}
		if s == "" {
			return nil, "::timestamptz", nil
		}
		return s, "::timestamptz", nil
	}
	return nil, "", fmt.Errorf("unknown kind %q", kind)
}
