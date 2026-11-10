# Architecture

## Shape

A **modular monolith**: one Go binary, one PostgreSQL database, modules that talk through function
calls and shared tables — not network calls. For a company this size that is the simplest thing that
is correct: one transaction can move stock, create a guarantee and queue an SMS together.

```
browser / mobile apps ──► nginx (web container) ──► /api/v1 (Go) ──► PostgreSQL
                              │                         │
                              └─ static React app       ├─ SMS outbox worker (Melli Payamak)
                                                        ├─ payment gateway (Zarinpal / mock)
                                                        └─ uploads volume
```

| Module | Owns |
|---|---|
| `iam` | sign-in (password, SMS code), refresh tokens, users, roles → permissions, audit trail, notifications |
| `catalog` | categories, **products** (finished goods) and **parts** (spare parts/accessories), service items |
| `shop` | cart (guest + user), coupons, checkout, payment, orders, addresses, wishlist |
| `warranty` | customers, guarantees (register, approve, renew, golden period), code validation per product |
| `service` | repairs, part requests, technician warehouse, old-part shipments, invoices, technician ledger, pipeline |
| `warehouse` | warehouses, stock levels, movement documents + approvals, counts, QR labels, Sepidar barcode export |
| `hr` | employees, shifts, kiosk devices (offline sync), punches, leaves, tasks, reports |
| `surveys` | questions, automatic triggers, public answer page, results |
| `system` | settings, SMS patterns, manager overview |

## The one idea that removes the technicians' complexity

The old system modelled a part four times (request line, repair line, shipment line, price). Here:

* **A warehouse row per technician.** Delivering a part request is a stock *transfer* central → technician.
  Submitting a report *consumes* the parts from the technician's warehouse. "Do I have this part?" is a real answer.
* **`repair_parts` — one row per used part** carries: where the new part came from, whether the old
  one is owed back (`return_required`), `return_state` (`pending → in_shipment → received`), the parcel,
  and the price snapshot. Parcels and invoices are views over these rows.
* **`technician_ledger` — signed, immutable rows.** Invoicing a parcel adds a credit, paying it adds a
  debit, labour adds a credit; corrections are new rows. Balance = `SUM(amount)`.

```
request ──deliver──► technician warehouse ──submit report──► consumed
                                                              │ old part owed
   repair_parts.return_state: pending ─send─► in_shipment ─receive─► received ─(invoice)─► credit ─pay─► debit
```

## Roles and permissions

Permissions are strings (`wh.approve`, `service.manage`, …); a role is a bundle stored in
`role_permissions`, editable in *Settings → Roles* without a deploy. Defaults merge the three old systems:

| Role | Came from | Can |
|---|---|---|
| `super_admin` | warehouse "super admin" | everything, incl. creating admins |
| `admin` | guarantee admin / warehouse admin / attendance admin | everything except creating super admins |
| `technical` | guarantee "technical" | everything an admin can **except delete** and user/role/settings management |
| `shop_manager` | new | catalog, orders, coupons |
| `warehouse_manager` | new | stock, approvals, counts, exports |
| `storekeeper` | warehouse storekeeper | file movements; outgoing ones wait for approval |
| `hr_manager` | attendance "manager" | employees, leaves, tasks, devices |
| `viewer` | attendance "viewer" | read-only everywhere |
| `technician` | guarantee technician | technician portal only (own repairs, own stock, own parcels, own account) |
| `customer` | new | account: orders, guarantees, service requests |

A user can hold several roles; permissions are the union.

## Safety properties (enforced in the database where possible)

* Audit log, stock documents/lines and the technician ledger are **append-only** (triggers refuse
  UPDATE/DELETE). Stock documents are "deleted" by reversal + a recorded reason.
* Money is whole Toman in `BIGINT` with `CHECK >= 0`; no floats.
* Stock changes happen only inside document transactions with row locks; negative stock is refused
  unless the setting is turned off.
* A guarantee code can be re-registered after rejection/cancellation, never while live
  (partial unique index).
* `guarantee_was_expired` is stamped when a repair is opened, so renewing later cannot rewrite history.
* Refresh tokens rotate; reuse of an old one revokes the whole session family. Login and PIN
  attempts are throttled. Uploads are type-sniffed from bytes. Secrets are refused if left at the
  example values in production.
* SMS never blocks a request: messages go to an outbox table with retry/back-off.
* Payment callbacks are verified server-side and idempotent; stock leaves only when money arrives.

## Frontend

Route-level code splitting — customers never download back-office code. Admin menu is generated from
the signed-in user's permissions; unreachable screens are not shown and are also refused by the API.
Technician portal: bottom navigation, large targets, one primary action per screen, autosaved report.
Tables turn into cards on phones.

## Mobile apps (not built yet, endpoints ready)

Customer / technician / manager apps use the same `/api/v1`: `POST /auth/login|otp/*`, `/auth/refresh`
(pass `client: "customer-app" | "technician-app" | "manager-app"`), `/push-tokens`, `/notifications`.
`GET /api/v1/_routes` (admin) prints the live route list. See [API.md](API.md).

## Known limits / next steps

* Kiosk face recognition runs in the browser/tablet in the old attendance app; this repository ships
  the **server side** (gallery, offline-safe punch sync, devices, enrolment storage, reports) but not
  the tablet UI or ESP32 firmware — reuse them from the attendance repository against `/kiosk/*`
  (see MIGRATION.md for the key mapping).
* Sepidar: barcode TXT export is implemented; the optional `.xls` document-creation bundle of the old
  warehouse system is not ported.
* The scanner screen needs a connection (the old PWA's offline queue is not ported).
* Payment: Zarinpal v4 (+ sandbox) is implemented; other Iranian gateways need a new `payment.Gateway`.
* Single API instance (login throttle and SMS worker are in-process). Fine for this load.
