// Package files stores uploads on disk (a Docker volume) and serves them back
// by unguessable id.
package files

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/evinki/platform/internal/app"
	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
)

var allowed = map[string]string{
	"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "application/pdf": ".pdf",
}

type File struct {
	ID   string `db:"id" json:"id"`
	URL  string `json:"url"`
	Mime string `db:"mime" json:"mime"`
	Size int64  `db:"size_bytes" json:"size"`
}

// Mount: POST /files (any signed-in user, or guest for guarantee registration with purpose=invoice or guarantee_card)
// and GET /files/{id}. Content type is sniffed from the bytes, never trusted from the client.
func Mount(r chi.Router, d *app.Deps) {
	_ = os.MkdirAll(d.Cfg.UploadDir, 0o755)

	r.Post("/files", httpx.H(func(w http.ResponseWriter, req *http.Request) error {
		p := auth.From(req.Context())
		purpose := req.URL.Query().Get("purpose")
		if p == nil && purpose != "invoice" && purpose != "guarantee_card" {
			return httpx.ErrUnauthorized
		}
		req.Body = http.MaxBytesReader(w, req.Body, d.Cfg.MaxUploadSize)
		f, hdr, err := req.FormFile("file")
		if err != nil {
			return httpx.BadRequest("فایل ارسال نشده یا بزرگ‌تر از حد مجاز است")
		}
		defer f.Close()
		head := make([]byte, 512)
		n, _ := io.ReadFull(f, head)
		mime := http.DetectContentType(head[:n])
		ext, ok := allowed[mime]
		if !ok {
			return httpx.BadRequest("فقط تصویر (JPG/PNG/WebP) یا PDF مجاز است")
		}
		id := uuid.New()
		name := id.String() + ext
		out, err := os.Create(filepath.Join(d.Cfg.UploadDir, name))
		if err != nil {
			return err
		}
		defer out.Close()
		if _, err = out.Write(head[:n]); err != nil {
			return err
		}
		size, err := io.Copy(out, f)
		if err != nil {
			return err
		}
		var owner *int64
		if p != nil {
			owner = &p.UserID
		}
		_, err = d.DB.Exec(req.Context(), `INSERT INTO files(id,path,original,mime,size_bytes,purpose,owner_id) VALUES($1,$2,$3,$4,$5,$6,$7)`,
			id, name, filepath.Base(hdr.Filename), mime, size+int64(n), purpose, owner)
		if err != nil {
			return err
		}
		httpx.Created(w, File{ID: id.String(), URL: "/api/v1/files/" + id.String(), Mime: mime, Size: size + int64(n)})
		return nil
	}))

	r.Get("/files/{id}", httpx.H(func(w http.ResponseWriter, req *http.Request) error {
		id, err := uuid.Parse(chi.URLParam(req, "id"))
		if err != nil {
			return httpx.ErrNotFound
		}
		type row struct {
			Path string `db:"path"`
			Mime string `db:"mime"`
		}
		f, err := db.One[row](req.Context(), d.DB, `SELECT path, mime FROM files WHERE id=$1`, id)
		if err != nil {
			return err
		}
		w.Header().Set("Content-Type", f.Mime)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, max-age=86400")
		if strings.HasPrefix(f.Mime, "application/") {
			w.Header().Set("Content-Disposition", "inline")
		}
		http.ServeFile(w, req, filepath.Join(d.Cfg.UploadDir, filepath.Base(f.Path)))
		return nil
	}))
}
