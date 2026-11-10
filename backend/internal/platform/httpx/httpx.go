// Package httpx holds the small HTTP helpers every module shares: JSON
// responses, uniform errors, body decoding, pagination.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/evinki/platform/internal/platform/db"
)

// Error is the one error shape the apps see: {"error":{"code","message"}}.
type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

func E(status int, code, msg string) *Error { return &Error{Status: status, Code: code, Message: msg} }

var (
	ErrUnauthorized = E(401, "unauthorized", "ورود لازم است")
	ErrForbidden    = E(403, "forbidden", "دسترسی ندارید")
	ErrNotFound     = E(404, "not_found", "پیدا نشد")
)

func BadRequest(msg string) *Error { return E(400, "bad_request", msg) }
func Conflict(msg string) *Error   { return E(409, "conflict", msg) }

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func OK(w http.ResponseWriter, v any)      { JSON(w, 200, v) }
func Created(w http.ResponseWriter, v any) { JSON(w, 201, v) }
func NoContent(w http.ResponseWriter)      { w.WriteHeader(http.StatusNoContent) }

// Fail writes err. Known *Error values keep their status; database errors are
// translated; anything else is a 500 whose detail is logged, never sent.
func Fail(w http.ResponseWriter, r *http.Request, err error) {
	var he *Error
	switch {
	case errors.As(err, &he):
	case errors.Is(err, db.ErrNotFound):
		he = ErrNotFound
	case db.IsUnique(err):
		he = Conflict("این مقدار قبلاً ثبت شده است")
	case db.IsFK(err):
		he = Conflict("این رکورد به موارد دیگری وابسته است")
	case db.IsCheck(err):
		he = BadRequest("مقدار نامعتبر است")
	default:
		slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		he = E(500, "internal", "خطای داخلی سرور")
	}
	JSON(w, he.Status, map[string]any{"error": he})
}

// Decode reads a JSON body, rejecting unknown fields and oversized bodies.
func Decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 4<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return BadRequest("بدنه درخواست نامعتبر است: " + err.Error())
	}
	return nil
}

// Handler lets handlers return errors instead of writing them everywhere.
type Handler func(w http.ResponseWriter, r *http.Request) error

func H(h Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			Fail(w, r, err)
		}
	}
}

func ID(r *http.Request, name string) (int64, error) {
	n, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || n <= 0 {
		return 0, BadRequest("شناسه نامعتبر است")
	}
	return n, nil
}

// Page is a parsed ?page=&limit= pair.
type Page struct{ Page, Limit, Offset int }

func Paging(r *http.Request, maxLimit int) Page {
	p, _ := strconv.Atoi(r.URL.Query().Get("page"))
	l, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if p < 1 {
		p = 1
	}
	if l < 1 {
		l = 20
	}
	if l > maxLimit {
		l = maxLimit
	}
	return Page{Page: p, Limit: l, Offset: (p - 1) * l}
}

// List is the standard paged envelope.
type List[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

func Q(r *http.Request, key string) string { return strings.TrimSpace(r.URL.Query().Get(key)) }

func QInt(r *http.Request, key string) (int64, bool) {
	n, err := strconv.ParseInt(Q(r, key), 10, 64)
	return n, err == nil
}

// Persian/Arabic-Indic digits → ASCII, so every numeric field accepts what people type.
func Digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '۰' && r <= '۹':
			b.WriteRune('0' + (r - '۰'))
		case r >= '٠' && r <= '٩':
			b.WriteRune('0' + (r - '٠'))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Phone normalises Iranian mobiles to 09xxxxxxxxx.
func Phone(s string) string {
	s = Digits(strings.TrimSpace(s))
	s = strings.NewReplacer(" ", "", "-", "", "+", "").Replace(s)
	switch {
	case strings.HasPrefix(s, "0098"):
		s = "0" + s[4:]
	case strings.HasPrefix(s, "98") && len(s) == 12:
		s = "0" + s[2:]
	case strings.HasPrefix(s, "9") && len(s) == 10:
		s = "0" + s
	}
	return s
}

func ValidMobile(s string) bool {
	if len(s) != 11 || !strings.HasPrefix(s, "09") {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
