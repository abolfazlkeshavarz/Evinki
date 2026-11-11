// Package server assembles the HTTP router from every module.
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/evinki/platform/internal/app"
	"github.com/evinki/platform/internal/modules/catalog"
	"github.com/evinki/platform/internal/modules/hr"
	"github.com/evinki/platform/internal/modules/iam"
	"github.com/evinki/platform/internal/modules/service"
	"github.com/evinki/platform/internal/modules/shop"
	"github.com/evinki/platform/internal/modules/surveys"
	"github.com/evinki/platform/internal/modules/system"
	"github.com/evinki/platform/internal/modules/warehouse"
	"github.com/evinki/platform/internal/modules/warranty"
	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/files"
	"github.com/evinki/platform/internal/platform/httpx"
)

const Version = "1.0.0"

func New(d *app.Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer)
	r.Use(timeoutExceptUpdate(60 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   d.Cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Cart-Token", "X-Device-Key", "X-Filename", "X-Sha256"},
		AllowCredentials: false,
		MaxAge:           600,
	}))
	r.Use(securityHeaders)

	r.Get("/healthz", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), 3*time.Second)
		defer cancel()
		status := "ok"
		code := 200
		if err := d.DB.Ping(ctx); err != nil {
			status, code = "database_unreachable", 503
		}
		httpx.JSON(w, code, map[string]string{"status": status, "version": Version})
	})

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(d.Auth.Authenticate)
		api.Use(d.Audit.Middleware)
		api.Use(noStore)

		iam.Mount(api, d)
		files.Mount(api, d)
		catalog.Mount(api, d)
		shop.Mount(api, d)
		warranty.Mount(api, d)
		service.Mount(api, d)
		warehouse.Mount(api, d)
		hr.Mount(api, d)
		surveys.Mount(api, d)
		system.Mount(api, d)

		// A live list of every route, for the mobile app teams. Only for staff.
		api.With(auth.Require(auth.PSettings)).Get("/_routes", func(w http.ResponseWriter, req *http.Request) {
			var out []string
			_ = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
				out = append(out, method+" "+strings.Replace(route, "/*", "", 1))
				return nil
			})
			sort.Strings(out)
			b, _ := json.Marshal(out)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(b)
		})
		api.Get("/version", func(w http.ResponseWriter, req *http.Request) {
			httpx.OK(w, map[string]string{"version": Version, "api": "v1"})
		})
	})
	return r
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// API answers are per-user and must never be cached by a proxy or the browser.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/files/") && !strings.HasPrefix(r.URL.Path, "/api/v1/shop/categories") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// timeoutExceptUpdate is chi's request timeout, except for the big image-bundle upload (PUT .../admin/system/update),
// which may legitimately take many minutes on a slow link.
func timeoutExceptUpdate(d time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		limited := middleware.Timeout(d)(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/admin/system/update") {
				next.ServeHTTP(w, r)
				return
			}
			limited.ServeHTTP(w, r)
		})
	}
}
