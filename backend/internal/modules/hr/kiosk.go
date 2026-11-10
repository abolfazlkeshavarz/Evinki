package hr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
	"github.com/evinki/platform/internal/platform/settings"
)

func strings_Itoa(n int) string { return strconv.Itoa(n) }

type deviceCtx struct{}

type device struct {
	ID   int64  `db:"id"`
	Name string `db:"name"`
	Kind string `db:"kind"`
}

// deviceAuth identifies a kiosk/reader by its API key (header X-Device-Key).
// Keys are stored as SHA-256 so they can be looked up with one indexed query.
func (m *mod) deviceAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-Device-Key")
		if key == "" {
			httpx.Fail(w, r, httpx.ErrUnauthorized)
			return
		}
		hashes := []string{auth.HashToken(key)}
		if m.Cfg.LegacyDeviceSecret != "" {
			// the old system stored sha256(raw key + its SECRET_KEY)
			sum := sha256.Sum256([]byte(key + m.Cfg.LegacyDeviceSecret))
			hashes = append(hashes, hex.EncodeToString(sum[:]))
		}
		d, err := db.One[device](r.Context(), m.DB, `SELECT id,name,kind FROM attendance_devices WHERE api_key_hash = ANY($1) AND is_active`, hashes)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrUnauthorized)
			return
		}
		_, _ = m.DB.Exec(r.Context(), `UPDATE attendance_devices SET last_seen_at=NOW() WHERE id=$1`, d.ID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), deviceCtx{}, d)))
	})
}

func dev(r *http.Request) device { return r.Context().Value(deviceCtx{}).(device) }

func (m *mod) mountKiosk(r chi.Router) {
	r.Route("/kiosk", func(r chi.Router) {
		r.Use(m.deviceAuth)
		r.Get("/config", httpx.H(m.kioskConfig))
		r.Get("/gallery", httpx.H(m.kioskGallery))
		r.Post("/heartbeat", httpx.H(m.kioskHeartbeat))
		r.Post("/punches", httpx.H(m.kioskPunches))
		r.Post("/pin-punch", httpx.H(m.kioskPin))
		r.Get("/fingerprint/jobs", httpx.H(m.fpJobs))
		r.Post("/fingerprint/jobs/{id}/complete", httpx.H(m.fpComplete))
	})
}

func (m *mod) kioskConfig(w http.ResponseWriter, r *http.Request) error {
	s, err := settings.All(r.Context(), m.DB)
	if err != nil {
		return err
	}
	httpx.OK(w, map[string]any{"device": dev(r).Name, "server_time": time.Now().UTC(),
		"face": s["hr_face_enabled"], "fingerprint": s["hr_fingerprint_enabled"], "pin": s["hr_pin_enabled"], "liveness": s["hr_liveness"]})
	return nil
}

// kioskGallery sends every active employee's face vectors so recognition can
// run on the tablet with no network.
func (m *mod) kioskGallery(w http.ResponseWriter, r *http.Request) error {
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
	  SELECT e.id AS employee_id, e.first_name||' '||e.last_name AS name, e.personnel_code,
	    (SELECT json_agg(f.vector::json) FROM face_embeddings f WHERE f.employee_id=e.id AND f.is_active) AS vectors
	  FROM employees e WHERE e.is_active AND EXISTS (SELECT 1 FROM face_embeddings f WHERE f.employee_id=e.id AND f.is_active)) t`)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
	return nil
}

func (m *mod) kioskHeartbeat(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		AppVersion   string          `json:"app_version"`
		PendingCount int             `json:"pending_count"`
		LastScan     json.RawMessage `json:"last_scan"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if len(in.LastScan) == 0 {
		in.LastScan = json.RawMessage(`{}`)
	}
	_, err := m.DB.Exec(r.Context(), `UPDATE attendance_devices SET app_version=$2,pending_count=$3,last_scan=$4 WHERE id=$1`, dev(r).ID, in.AppVersion, in.PendingCount, in.LastScan)
	httpx.OK(w, map[string]any{"server_time": time.Now().UTC()})
	return err
}

type punchIn struct {
	ClientUUID     string   `json:"client_uuid"`
	EmployeeID     int64    `json:"employee_id"`
	Kind           string   `json:"kind"`
	Method         string   `json:"method"`
	HappenedAt     string   `json:"happened_at"`
	Confidence     *float64 `json:"confidence"`
	CreatedOffline bool     `json:"created_offline"`
}

// kioskPunches accepts a batch. Re-sending the same batch is harmless: client_uuid is unique.
func (m *mod) kioskPunches(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Punches []punchIn `json:"punches"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if len(in.Punches) > 500 {
		return httpx.BadRequest("حداکثر ۵۰۰ تردد در هر ارسال")
	}
	d := dev(r)
	accepted, dup, rejected := 0, 0, 0
	for _, p := range in.Punches {
		at, err := time.Parse(time.RFC3339, p.HappenedAt)
		okMethod := p.Method == "face" || p.Method == "fingerprint" || p.Method == "pin"
		if err != nil || p.ClientUUID == "" || (p.Kind != "in" && p.Kind != "out") || !okMethod || at.After(time.Now().Add(10*time.Minute)) {
			rejected++
			continue
		}
		uuid := p.ClientUUID
		ok, err := m.insertPunch(r.Context(), m.DB, p.EmployeeID, &d.ID, p.Kind, p.Method, at, p.Confidence, &uuid, p.CreatedOffline, "", nil)
		switch {
		case err != nil:
			rejected++ // unknown employee etc.: do not block the rest of the batch
		case ok:
			accepted++
		default:
			dup++
		}
	}
	_, _ = m.DB.Exec(r.Context(), `UPDATE attendance_devices SET last_sync_at=NOW() WHERE id=$1`, d.ID)
	httpx.OK(w, map[string]any{"accepted": accepted, "duplicates": dup, "rejected": rejected})
	return nil
}

// kioskPin is the always-available fallback: personnel code + PIN. The kind
// (in/out) alternates automatically from the employee's last punch today.
func (m *mod) kioskPin(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		PersonnelCode string `json:"personnel_code"`
		PIN           string `json:"pin"`
		Kind          string `json:"kind"`
		ClientUUID    string `json:"client_uuid"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if !settings.Bool(r.Context(), m.DB, "hr_pin_enabled") {
		return httpx.E(403, "disabled", "ورود با کد پرسنلی غیرفعال است")
	}
	e, err := m.employeeByPIN(r.Context(), in.PersonnelCode, in.PIN)
	if err != nil {
		return err
	}
	kind := in.Kind
	if kind != "in" && kind != "out" {
		last, _ := db.Scalar[string](r.Context(), m.DB, `SELECT kind FROM attendance_records WHERE employee_id=$1 AND happened_at > NOW()-INTERVAL '20 hours' ORDER BY happened_at DESC LIMIT 1`, e.ID)
		kind = "in"
		if last == "in" {
			kind = "out"
		}
	}
	var uuid *string
	if in.ClientUUID != "" {
		uuid = &in.ClientUUID
	}
	d := dev(r)
	if _, err = m.insertPunch(r.Context(), m.DB, e.ID, &d.ID, kind, "pin", time.Now(), nil, uuid, false, "", nil); err != nil {
		return err
	}
	httpx.OK(w, map[string]any{"employee_id": e.ID, "name": e.Name, "kind": kind})
	return nil
}

// ───────── devices (admin) ─────────

func (m *mod) listDevices(w http.ResponseWriter, r *http.Request) error {
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t ORDER BY t.id),'[]') FROM (
	  SELECT id,name,kind,device_uid,location,last_seen_at,last_sync_at,app_version,pending_count,is_active,last_scan,
	    (last_seen_at > NOW()-INTERVAL '5 minutes') AS online FROM attendance_devices) t`)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
	return nil
}

func (m *mod) createDevice(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Name     string `json:"name"`
		Kind     string `json:"kind"`
		Location string `json:"location"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Name) == "" {
		return httpx.BadRequest("نام دستگاه لازم است")
	}
	if in.Kind != "fingerprint" {
		in.Kind = "tablet"
	}
	key := auth.RandToken(32)
	id, err := db.Scalar[int64](r.Context(), m.DB, `INSERT INTO attendance_devices(name,kind,device_uid,api_key_hash,location) VALUES($1,$2,$3,$4,$5) RETURNING id`,
		in.Name, in.Kind, auth.RandToken(9), auth.HashToken(key), in.Location)
	if err != nil {
		return err
	}
	m.Audit.Log(r, "device.create", "device", id, in.Name)
	// the raw key is shown exactly once
	httpx.Created(w, map[string]any{"id": id, "api_key": key})
	return nil
}

func (m *mod) rotateKey(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	key := auth.RandToken(32)
	tag, err := m.DB.Exec(r.Context(), `UPDATE attendance_devices SET api_key_hash=$2 WHERE id=$1`, id, auth.HashToken(key))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return httpx.ErrNotFound
	}
	m.Audit.Log(r, "device.rotate_key", "device", id, "")
	httpx.OK(w, map[string]any{"api_key": key})
	return nil
}

func (m *mod) patchDevice(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Name     *string `json:"name"`
		Location *string `json:"location"`
		IsActive *bool   `json:"is_active"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	_, err = m.DB.Exec(r.Context(), `UPDATE attendance_devices SET name=COALESCE($2,name),location=COALESCE($3,location),is_active=COALESCE($4,is_active) WHERE id=$1`, id, in.Name, in.Location, in.IsActive)
	httpx.NoContent(w)
	return err
}

// ───────── fingerprint jobs ─────────

func (m *mod) fingerprintJob(w http.ResponseWriter, r *http.Request) error {
	emp, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		DeviceID int64 `json:"device_id"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	id, err := db.Scalar[int64](r.Context(), m.DB, `INSERT INTO fingerprint_enroll_jobs(employee_id,device_id) VALUES($1,$2) RETURNING id`, emp, in.DeviceID)
	if err != nil {
		return err
	}
	httpx.Created(w, map[string]any{"id": id})
	return nil
}

func (m *mod) fpJobs(w http.ResponseWriter, r *http.Request) error {
	b, err := db.Scalar[json.RawMessage](r.Context(), m.DB, `SELECT COALESCE(json_agg(t),'[]') FROM (
	  SELECT j.id,j.employee_id,e.personnel_code FROM fingerprint_enroll_jobs j JOIN employees e ON e.id=j.employee_id WHERE j.device_id=$1 AND j.status='pending') t`, dev(r).ID)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
	return nil
}

func (m *mod) fpComplete(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		OK       bool   `json:"ok"`
		Error    string `json:"error"`
		Template []byte `json:"template"` // base64 in JSON
		Model    string `json:"model"`
		SlotID   int    `json:"slot_id"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	d := dev(r)
	err = m.DB.Tx(r.Context(), func(tx pgxTx) error {
		emp, e := db.Scalar[int64](r.Context(), tx, `SELECT employee_id FROM fingerprint_enroll_jobs WHERE id=$1 AND device_id=$2 AND status='pending' FOR UPDATE`, id, d.ID)
		if e != nil {
			return httpx.ErrNotFound
		}
		if !in.OK {
			_, e = tx.Exec(r.Context(), `UPDATE fingerprint_enroll_jobs SET status='failed',error=$2,completed_at=NOW() WHERE id=$1`, id, in.Error)
			return e
		}
		if len(in.Template) == 0 {
			return httpx.BadRequest("قالب اثر انگشت ارسال نشده است")
		}
		// the central copy is the source of truth; the slot only maps it to this reader's memory
		if _, e = tx.Exec(r.Context(), `INSERT INTO fingerprint_templates(employee_id,template_data,model_name) VALUES($1,$2,$3)
			ON CONFLICT (employee_id) DO UPDATE SET template_data=EXCLUDED.template_data, model_name=EXCLUDED.model_name`, emp, in.Template, in.Model); e != nil {
			return e
		}
		if _, e = tx.Exec(r.Context(), `INSERT INTO fingerprint_slots(device_id,employee_id,slot_id,synced_at) VALUES($1,$2,$3,NOW())
			ON CONFLICT (device_id,employee_id) DO UPDATE SET slot_id=EXCLUDED.slot_id, synced_at=NOW()`, d.ID, emp, in.SlotID); e != nil {
			return e
		}
		_, e = tx.Exec(r.Context(), `UPDATE fingerprint_enroll_jobs SET status='done',completed_at=NOW() WHERE id=$1`, id)
		return e
	})
	if err != nil {
		return err
	}
	httpx.NoContent(w)
	return nil
}

var _ = fmt.Sprint
