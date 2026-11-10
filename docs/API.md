# API for the mobile apps

Base: `/api/v1`. JSON in/out. Errors: `{"error":{"code","message"}}` (messages are Persian, `code` is stable).
Lists: `{"items":[…],"total":n,"page":p,"limit":l}` (`?page=&limit=&q=`). Money: integer Toman. Dates: ISO.
The live route list (staff only): `GET /api/v1/_routes`.

## Authentication

| | |
|---|---|
| `POST /auth/login` `{username,password,client}` | staff / technicians (username or phone). `client`: `web`, `customer-app`, `technician-app`, `manager-app` |
| `POST /auth/otp/request` `{phone}` → `POST /auth/otp/verify` `{phone,code,name?,client}` | customers; creates the account on first use and links existing guarantees by phone |
| `POST /auth/refresh` `{refresh_token}` | rotates; access token lives 30 min, refresh 30 days |
| `POST /auth/logout`, `GET /auth/me`, `POST /auth/change-password` | |
| `GET /notifications`, `POST /notifications/{id}/read`, `POST /push-tokens {platform,token}` | in-app inbox and FCM/APNs registration |

Send `Authorization: Bearer <access_token>`. `me.permissions` tells the app which screens to show.

## Customer app
`GET /shop/categories|products|products/{slug}|parts`, cart (`GET/PUT /shop/cart/items`, `POST /shop/cart/coupon`,
guests use header `X-Cart-Token`), `POST /shop/checkout` → `payment_url` (open in the in-app browser;
the gateway returns to `PUBLIC_URL/account/orders/{id}`), `GET /shop/orders[/{id}]`, addresses, wishlist,
`GET/POST /my/guarantees`, `GET /public/guarantees/check?code=`, `POST /public/guarantees/validate|register`,
`POST /my/service-requests {guarantee_id,complaint}`, `GET /my/repairs`, `POST /files` (multipart `file`).

## Technician app (`tech.portal`)
`GET /tech/dashboard`, `GET /tech/guarantees/lookup?code=`, repairs (`GET/POST /tech/repairs`, `GET/PATCH /tech/repairs/{id}`,
`POST …/start|parts|services|photos|submit|cancel|reopen`), part requests, `GET /tech/stock`, `GET /tech/returns`,
shipments (`GET/POST /tech/shipments`, `POST …/{id}/cancel`), `GET /tech/ledger`, `GET /lookup/parts|service-items|products`.
Adding a part that is not in the technician's stock returns `409 {error.code:"not_in_stock", part_id, available}` — offer "request it".

## Manager app (`reports.read` …)
`GET /manager/overview` (one call: everything needing attention), plus every `/admin/*`, `/wh/*`, `/hr/*` endpoint
the role is allowed to use (orders, guarantees approve/renew, pipeline, part requests, shipments receive/pay,
stock documents approve, leaves review, survey results…).

## Attendance devices
Header `X-Device-Key`. `GET /kiosk/config|gallery`, `POST /kiosk/heartbeat`, `POST /kiosk/punches` (batch of
≤500, idempotent by `client_uuid`), `POST /kiosk/pin-punch`, `GET /kiosk/fingerprint/jobs`,
`POST /kiosk/fingerprint/jobs/{id}/complete`.

## Conventions
* Mutating `/admin/*` calls need the matching permission (403 otherwise). Delete permissions are separate (`*.delete`).
* Status values are lowercase snake_case strings; transitions are validated server-side (409 on illegal ones).
* Every successful mutation is written to the audit log with user, route and IP.
