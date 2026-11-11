# Evinki Platform

One system for everything Evinki runs: the **online shop**, **guarantees and after-sales service**,
the **parts warehouse**, **employee attendance** and **customer surveys**. It replaces and imports
the data of the three current systems (guarantee, warehouse, attendance).

- **Backend**: Go (chi, pgx) + PostgreSQL 16 — one modular service, one database.
- **Web app**: React + TypeScript + Tailwind, Persian/RTL, Solar Hijri dates, light/dark, installable (PWA).
  One app with four areas: storefront, customer account, back-office (permission-driven menu), and a
  phone-first technician portal.
- **Mobile-ready API**: `/api/v1` with token + refresh-token auth, notifications and push-token
  endpoints. The customer, technician and manager apps can be built on it without backend changes
  (see [docs/API.md](docs/API.md)).
- **Deployment**: Docker Compose, one command on a fresh server, backups, safe updates.

## Start in 3 commands

```bash
DEV=1 make env    # .env for local testing (mock payment, http, free ports)
make up           # builds and starts database + API + web   → http://127.0.0.1:8090
make health
```

Production server (Ubuntu/Debian), with HTTPS and nightly backups:

```bash
DOMAIN=shop.evinki.com LETSENCRYPT_EMAIL=you@evinki.com ZARINPAL_MERCHANT_ID=xxxx ./scripts/bootstrap-vps.sh
# or from your PC, building here and pushing over SSH (with one-command rollback):
make deploy-remote HOST=root@SERVER DOMAIN=shop.evinki.com LETSENCRYPT_EMAIL=you@evinki.com
```

Details: [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md). Attendance (face enrolment, kiosk, graphs): [docs/ATTENDANCE.md](docs/ATTENDANCE.md). Guarantees (code formulas, manufacture-date warranty): [docs/GUARANTEE.md](docs/GUARANTEE.md). Updating with an image bundle: [docs/UPDATING.md](docs/UPDATING.md). Run `make help` for every command.

## Moving from the current systems

Back up each old system, drop the files in `./legacy`, rehearse, then import:

```bash
make import-warehouse  ARGS="-src sqlite:/legacy/warehouse.db -images /legacy/parts -dry-run"
make import-guarantee  ARGS="-src postgres://user:pass@host.docker.internal:5432/db -dry-run"
make import-attendance ARGS="-src sqlite:/legacy/attendance.db -dry-run"
```

Users keep their passwords (old hashes are verified and silently upgraded), printed QR labels stay
valid, kiosk tablets keep their keys, technician balances carry over. Full guide:
[docs/MIGRATION.md](docs/MIGRATION.md).

## What changed for the people using it

| Problem in the old guarantee system | In this platform |
|---|---|
| A technician had to follow one part across part request → repair line → shipment → invoice | One row per used part carries its whole life; the technician sees "parts to send" and sends them in one tap |
| "Own stock" was a checkbox | Each technician has a real warehouse: parts delivered to them are a stock transfer, parts used are consumed on submit; the app warns and offers "request this part" when it is not in their stock |
| The office typed a price for every returned part, every time | A price list (`buyback_price` per part). Receiving a parcel invoices it automatically; prices can still be overridden |
| Money split over invoices and payments | A technician account (ledger): credits, payments, adjustments — balance is a SUM, rows are immutable |
| Polls were a manual campaign | Surveys fire automatically after every approved repair / delivered order, results include a per-technician rating and NPS |
| Five screens to follow one case | **Service pipeline**: every case in the stage it is waiting in, with "stuck > 7 days" flagged |

## Layout

```
backend/            Go service (cmd/api, cmd/evinkictl, cmd/smoke) + SQL migrations
  internal/platform auth, RBAC, audit, SMS outbox, payments, settings, generic CRUD
  internal/modules  iam, catalog, shop, warranty, service, warehouse, hr, surveys, system
  internal/legacy   importers for the three old systems
frontend/           React app (storefront, account, admin, technician portal)
deploy/ scripts/    nginx templates, install/backup/restore/update/import scripts
docs/               architecture, deployment, migration, API, roles
```

## Quality gates

`make test` (Go vet + tests, TypeScript check) and `make smoke` — a ~75-step end-to-end run
(sign-in, shopping with OTP and payment, delivery → automatic guarantee, technician part flow →
shipment → invoice → payment, surveys, kiosk offline sync, permission checks) that creates its
own data and is safe to run against a live system with `PAYMENT_GATEWAY=mock` in a staging copy.
