# Deployment

## Requirements

A Linux server (Ubuntu/Debian recommended), 2 GB RAM, 10 GB disk, a domain pointing at it (needed for
HTTPS — **phone cameras for QR scanning and the kiosk only work over HTTPS**), ports 80/443 open.

## One command

```bash
git clone <your repo> /opt/evinki && cd /opt/evinki
DOMAIN=shop.evinki.com LETSENCRYPT_EMAIL=you@evinki.com ZARINPAL_MERCHANT_ID=xxxxxxxx-xxxx ./scripts/bootstrap-vps.sh
```

It installs Docker, writes `.env` with random secrets, builds and starts the stack, installs nginx with a
Let's Encrypt certificate (auto-renewing), schedules a **nightly backup at 02:30**, and prints the first
administrator login. Several projects can share the server: only one nginx vhost is added.

`LAN_ONLY=1` skips the domain/TLS (internal network; no camera).

## Small server (1 GB)? Build elsewhere

```bash
make images                         # on your PC → dist/evinki-images.tar.gz  (PLATFORM=linux/arm64 for ARM)
scp -r . root@SERVER:/opt/evinki
ssh root@SERVER 'cd /opt/evinki && make load-images && docker compose up -d --no-build'
```

## Configuration (`.env`)

| Variable | Meaning |
|---|---|
| `PUBLIC_URL` | public https address; payment callbacks and SMS links are built from it |
| `DB_PASSWORD`, `JWT_SECRET` | random; the server **refuses to start in production** with placeholder values |
| `PAYMENT_GATEWAY` | `zarinpal` (production; `ZARINPAL_MERCHANT_ID`, `ZARINPAL_SANDBOX=true` to test) or `mock` (development only — refused in production) |
| `SMS_ENABLED`, `SMS_USERNAME`, `SMS_PASSWORD` | Melli Payamak. Patterns (`body_id`) are managed in *Settings → SMS patterns*, not in `.env` |
| `BOOTSTRAP_ADMIN_USER/PASSWORD` | first super admin (only used when the database has no users) |
| `LEGACY_DEVICE_KEY_SECRET` | see MIGRATION.md |
| `HTTP_PORT` | where the web container listens on the host (default `127.0.0.1:8090`) |

After editing: `make up`.

## Day to day

| | |
|---|---|
| `make ps` / `make logs` / `make health` | status, API logs, end-to-end health (also warns if no backup in 2 days) |
| `make backup` | database + uploads → `backups/` (14 newest kept) |
| `make restore FILE=backups/…tar.gz` | asks for confirmation, takes a safety backup first |
| `make update` | backup → `git pull --ff-only` → rebuild → verify; **rolls back the code automatically if unhealthy** |
| `make create-admin U=ali P='…'` / `make reset-password U=ali P='…'` | accounts from the command line |
| `make seed-demo` | sample catalogue (FZ530, accessories, standard survey) |
| `make smoke` | end-to-end test (staging copy with `PAYMENT_GATEWAY=mock`) |

Migrations run automatically and are additive. Off-site copies: add a cron `rsync` of `backups/`.

## Security notes

* Only the nginx container publishes a port, on `127.0.0.1` by default; the database listens on loopback only.
* The API trusts `X-Real-IP` only because nginx always overwrites it; do not expose the `api` container directly.
* Set a Zarinpal merchant id before launch; verify one real payment in sandbox first (`ZARINPAL_SANDBOX=true`).
* Customers have no passwords by default (SMS code); configure SMS patterns **before** launch or nobody can sign in
  (in development the code is returned by the API when `SMS_ENABLED=false`; never in production).
