# Attendance (حضور و غیاب)

Everything the old attendance system did, inside the admin panel (**منابع انسانی**) plus a kiosk app for the entrance tablet.

## Screens

| Screen | Where | What it does |
|---|---|---|
| Dashboard | `/admin/hr` | Today's counters, 7-day presence trend, status donut, who is inside, late today, arrival-hour histogram, people per department, attention cards (open tasks, pending leaves, people without a face, offline devices), full table of today. Refreshes every minute. |
| Attendance log | `/admin/hr/attendance` | Every punch (face / fingerprint / code / manual) with confidence and device, date + employee filter, manual punch with a reason, delete. |
| Employees | `/admin/hr/employees` | Photo, department, shift. Buttons per person: **چهره** (camera enrolment), **انگشت** (fingerprint enrolment on a reader), **رمز** (PIN). |
| Reports & graphs | `/admin/hr/report` | Presets (today, this week, this month, last month, 30/90 days) or any Jalali range. KPI tiles, daily stacked bars (present/leave/incomplete/absent + late line), worked hours & overtime per day, arrival histogram, attendance rate per department, per-employee table with day-by-day drill-down, rankings (late, absent, overtime, worked, early leave, incomplete). Excel-compatible CSV: summary and daily detail. |
| Leaves | `/admin/hr/leaves` | Approve / reject, add approved leave. Employees can ask themselves at **`/leave`** with personnel code + PIN. |
| Tasks | `/admin/hr/tasks` | Duties with owner, priority, repeat, due date and a **daily log** (what was done, minutes). |
| Devices | `/admin/hr/devices` | Register tablets / fingerprint readers, show the key once, rotate it, online state, unsent punches. |
| Shifts & holidays | `/admin/hr/shifts` | Shifts (grace times, break, work days, night shifts), departments, official holidays. |
| Settings | `/admin/settings` → Attendance | Face / PIN / fingerprint switches, liveness, similarity threshold, safety margin, cool-down. |
| **Kiosk** | **`/kiosk`** | Full-screen entrance app (below). |

## Face registration (employee photos)

*Employees → چهره*: the employee looks at the camera; **5 samples are taken automatically** (face must be large enough and
detected with confidence ≥ 0.75). For every sample the browser computes a 512-number vector (MobileFaceNet / ArcFace) and uploads
the vector plus a small thumbnail. Samples can be deleted one by one or all at once. Samples made by the older face-api model
(128-d) are flagged "old" and must be re-registered. Vectors imported from the old attendance system are the same kind
(512-d) and keep working.

The camera needs **https** (or localhost) - the deploy scripts already set up SSL.

## Kiosk (entrance tablet)

1. *Devices → new device* (type tablet) and copy the key (shown once).
2. On the tablet open `https://<your-domain>/kiosk`, paste the key. It is remembered in the browser.
3. Employees look at the camera and hold still ~2 s (liveness: a photo does not hold a stable, recognised face); the name and in/out
   are shown. Two faces in the frame → asked to come one at a time. Unknown face → points to the code + PIN pad.
4. In/out alternates automatically; the same person cannot punch again within the cool-down (settings).
5. **Offline-safe**: the face gallery is cached in the tablet; punches taken without internet are stored (IndexedDB) and sent in
   batches every 2 minutes (safe to resend: `client_uuid`). The top bar shows how many are waiting.
6. Double-tap the clock to forget the device key.

Fingerprint readers (ESP32 etc.) keep using `/api/v1/kiosk/fingerprint/*` with their own key; enrolment is requested from
*Employees → انگشت*.

## Licence note (important)

The recognition model `frontend/public/models/mobilefacenet/w600k_mbf.onnx` is InsightFace's `buffalo_sc` recogniser. Its code is
MIT but **the trained weights are for non-commercial / research use**. The old attendance system used it the same way (internal
factory use). If Evinki ever sells or hosts the attendance feature for third parties, obtain a licence from InsightFace or replace
the file (e.g. OpenCV Zoo SFace) and the constants in `frontend/src/lib/face/mobileFaceNet.ts`. The face detectors (face-api
SSD MobileNet / landmarks) are MIT.

## Demo data

`docker compose run --rm --no-deps api evinkictl seed-hr-demo` creates 12 fictitious employees (codes `D1001`…) and 45 days of
punches so the graphs can be shown; `... seed-hr-demo -clear` removes them.

## New endpoints (admin, `hr.read` / `hr.manage`)

`GET /hr/today`, `GET /hr/dashboard` (counters, trend, arrivals, departments), `GET /hr/reports/range` (now also `series`,
`arrivals`, `departments`, `totals`, per-person `late_count` and `attendance_rate`), `GET /hr/reports/range.csv[?detail=1]`,
`GET /hr/employees/{id}/faces`, `DELETE /hr/employees/{id}/faces/{faceId}`,
`GET|POST /hr/employees/{id}/fingerprint-jobs`, `DELETE /hr/fingerprint-jobs/{id}`; kiosk: `GET /kiosk/gallery/version`,
`/kiosk/config` now returns `threshold`, `ambiguity_margin`, `cooldown_seconds`.
