// Package audit writes the append-only audit trail.
package audit

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/db"
)

type Logger struct{ DB db.Q }

// Log records one business event ("guarantee.approve", entity "guarantee", id).
func (l *Logger) Log(r *http.Request, action, entity string, id any, detail string) {
	p := auth.From(r.Context())
	var uid *int64
	name, role := "", ""
	if p != nil {
		uid, name, role = &p.UserID, p.Username, p.MainRole()
	}
	_, err := l.DB.Exec(r.Context(), `INSERT INTO audit_log(actor_id,actor_name,actor_role,action,entity,entity_id,detail,ip)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, uid, name, role, action, entity, toStr(id), detail, auth.ClientIP(r))
	if err != nil {
		slog.Error("audit write failed", "err", err)
	}
}

// Middleware records every successful mutating request, so coverage does not
// depend on remembering to instrument each handler. It stores method + route
// pattern only, never bodies: the same middleware sees the login endpoints.
func (l *Logger) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		if ww.Status() >= 200 && ww.Status() < 400 {
			pattern := chi.RouteContext(r.Context()).RoutePattern()
			if strings.Contains(pattern, "/auth/") || strings.Contains(pattern, "/kiosk/") {
				return
			}
			l.Log(r, "http."+strings.ToLower(r.Method), pattern, "", "")
		}
	})
}

func toStr(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case int64:
		return itoa(t)
	case int:
		return itoa(int64(t))
	}
	return ""
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
