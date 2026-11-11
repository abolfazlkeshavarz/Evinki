package system

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/httpx"
)

// Updating the running system with an image bundle built on a development computer (`make images-bundle`).
//
// The API container never touches Docker. It only drops the uploaded file and a request.json into UPDATES_DIR
// (a folder shared with the host); a small root service on the host (scripts/updater.sh, installed with
// `make install-updater`) picks the request up, takes a backup, loads the images, restarts the stack, checks
// health and rolls back on failure. It writes status.json, which this API serves to the admin screen.

const maxBundle = 3 << 30 // 3 GiB

var safeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func (m *mod) updDir() string { return m.Cfg.UpdatesDir }

func readJSON(path string, v any) bool {
	b, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(b, v) == nil
}

// updateBusy: a request exists that the updater has not finished.
func (m *mod) updateBusy() bool {
	var req map[string]any
	if !readJSON(filepath.Join(m.updDir(), "request.json"), &req) {
		return false
	}
	var st map[string]any
	if readJSON(filepath.Join(m.updDir(), "status.json"), &st) {
		if id, _ := st["id"].(string); id == req["id"] {
			s, _ := st["state"].(string)
			return s != "done" && s != "failed"
		}
	}
	return true
}

func (m *mod) updateStatus(w http.ResponseWriter, r *http.Request) error {
	var req, st, up map[string]any
	readJSON(filepath.Join(m.updDir(), "request.json"), &req)
	readJSON(filepath.Join(m.updDir(), "status.json"), &st)
	readJSON(filepath.Join(m.updDir(), "updater.json"), &up)
	online := false
	if up != nil {
		if s, _ := up["seen_at"].(string); s != "" {
			if t, err := time.Parse(time.RFC3339, s); err == nil && time.Since(t) < 45*time.Second {
				online = true
			}
		}
	}
	history := []json.RawMessage{}
	if b, err := os.ReadFile(filepath.Join(m.updDir(), "history.jsonl")); err == nil {
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		for i := len(lines) - 1; i >= 0 && len(history) < 10; i-- {
			if lines[i] != "" && json.Valid([]byte(lines[i])) {
				history = append(history, json.RawMessage(lines[i]))
			}
		}
	}
	logTail := ""
	if b, err := os.ReadFile(filepath.Join(m.updDir(), "last.log")); err == nil {
		if len(b) > 6000 {
			b = b[len(b)-6000:]
		}
		logTail = string(b)
	}
	httpx.OK(w, map[string]any{"updater_online": online, "updater": up, "request": req, "status": st, "history": history, "log": logTail, "busy": m.updateBusy()})
	return nil
}

// uploadUpdate receives the raw .tar.gz body (PUT, X-Filename header) and hands it to the updater.
func (m *mod) uploadUpdate(w http.ResponseWriter, r *http.Request) error {
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(45 * time.Minute)) // a bundle can take a while on a slow line
	_ = rc.SetWriteDeadline(time.Now().Add(46 * time.Minute))
	if m.updateBusy() {
		return httpx.Conflict("یک به‌روزرسانی در حال انجام یا در صف است")
	}
	name := safeName.ReplaceAllString(filepath.Base(r.Header.Get("X-Filename")), "_")
	if !strings.HasSuffix(strings.ToLower(name), ".tar.gz") && !strings.HasSuffix(strings.ToLower(name), ".tgz") {
		return httpx.BadRequest("فایل باید بستهٔ فشردهٔ ایمیج‌ها با پسوند .tar.gz باشد (خروجی make images-bundle)")
	}
	in := filepath.Join(m.updDir(), "incoming")
	if err := os.MkdirAll(in, 0o777); err != nil {
		return httpx.E(500, "updates_dir", "پوشهٔ به‌روزرسانی در دسترس نیست")
	}
	id := time.Now().UTC().Format("20060102-150405")
	final := filepath.Join(in, id+"-"+name)
	tmp := final + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o666)
	if err != nil {
		return err
	}
	h := sha256.New()
	body := http.MaxBytesReader(w, r.Body, maxBundle)
	head := make([]byte, 2)
	n, _ := io.ReadFull(body, head)
	if n < 2 || head[0] != 0x1f || head[1] != 0x8b { // gzip magic
		f.Close()
		os.Remove(tmp)
		return httpx.BadRequest("این فایل فشردهٔ gzip نیست؛ بستهٔ ساخته‌شده با make images-bundle را انتخاب کنید")
	}
	_, _ = h.Write(head)
	_, _ = f.Write(head)
	size, err := io.Copy(io.MultiWriter(f, h), body)
	size += 2
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return httpx.BadRequest("بارگذاری کامل نشد: " + err.Error())
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if want := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Sha256"))); want != "" && want != sum {
		os.Remove(tmp)
		return httpx.BadRequest("امضای فایل (SHA-256) با فایل دریافت‌شده نمی‌خواند؛ دوباره بارگذاری کنید")
	}
	if err := os.Rename(tmp, final); err != nil {
		return err
	}
	req := map[string]any{"id": id, "file": filepath.Join("incoming", filepath.Base(final)), "name": name, "size": size, "sha256": sum,
		"uploaded_by": auth.From(r.Context()).Username, "uploaded_at": time.Now().UTC().Format(time.RFC3339)}
	b, _ := json.Marshal(req)
	// status first (so the screen shows "queued" at once); the request last, because the updater starts on request.json
	st, _ := json.Marshal(map[string]any{"id": id, "state": "queued", "message": "در صف اجرا"})
	_ = os.WriteFile(filepath.Join(m.updDir(), "status.json"), st, 0o666)
	if err := os.WriteFile(filepath.Join(m.updDir(), "request.json.tmp"), b, 0o666); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(m.updDir(), "request.json.tmp"), filepath.Join(m.updDir(), "request.json")); err != nil {
		return err
	}
	m.Audit.Log(r, "system.update_uploaded", "system", 0, name)
	httpx.JSON(w, http.StatusAccepted, req)
	return nil
}
