# Deployment

## Requirements

A Linux server (Ubuntu/Debian), 2 GB RAM, 10 GB disk, a domain whose DNS **A record points at the server**
(needed for HTTPS — **phone cameras for QR scanning and the kiosk only work over HTTPS**), ports 80/443 open.

## Three ways to deploy

### A. One command on the server (it builds there)

```bash
git clone https://github.com/abolfazlkeshavarz/Evinki.git /opt/evinki && cd /opt/evinki
DOMAIN=shop.evinki.com LETSENCRYPT_EMAIL=you@evinki.com ZARINPAL_MERCHANT_ID=xxxxxxxx-xxxx ./scripts/bootstrap-vps.sh
```

It installs Docker, writes `.env` with random secrets and **free ports** (it never collides with another
project's 8090 / 15432), builds and starts the stack, gets the HTTPS certificate through the host's nginx,
schedules the nightly backup (02:30) and prints the first admin login. If an nginx from another project is
already there it only adds its own vhost file.

### B. Build on your PC, push over SSH (small servers, recommended for updates)

```bash
make deploy-remote HOST=root@SERVER                 # build here → upload → backup → switch → health check
make rollback-remote HOST=root@SERVER               # one command back to the previous version
# first time on a brand-new server, in one go:
make deploy-remote HOST=root@SERVER DOMAIN=shop.evinki.com LETSENCRYPT_EMAIL=you@evinki.com
```

The server never builds and needs no git. `.env`, database, uploads and backups are never touched; the
upload is checksum-verified; the old images are kept as `:previous`.

### C. Carry the images by hand

```bash
make images-bundle                                  # on your PC → dist/evinki-images.tar.gz (PLATFORM=linux/arm64 for ARM)
scp dist/evinki-images.tar.gz root@SERVER:/opt/evinki/dist/
ssh root@SERVER 'cd /opt/evinki && make load-images && ./scripts/bootstrap-vps.sh'   # prebuilt images are detected
```

## Step by step (if you prefer manual control)

```bash
make env DOMAIN=shop.evinki.com LETSENCRYPT_EMAIL=you@evinki.com   # .env with secrets + free ports
nano .env                                                          # ZARINPAL_MERCHANT_ID, SMS_*
make deploy-check                                                  # lists anything still missing
make up                      # or: make up-prebuilt (images loaded)
make ssl                     # certificate + nginx (DNS must already point here)
make health
```

## Configuration (`.env`)

| Variable | Meaning |
|---|---|
| `APP_DOMAIN`, `PUBLIC_URL` | main domain and `https://` address; payment callbacks and SMS links use it (`make change-domain` edits both) |
| `DB_PASSWORD`, `JWT_SECRET` | random; the server **refuses to start in production** with placeholders |
| `PAYMENT_GATEWAY` | `zarinpal` (`ZARINPAL_MERCHANT_ID`, `ZARINPAL_SANDBOX=true` to test) or `mock` (development only — refused in production) |
| `SMS_ENABLED/USERNAME/PASSWORD` | Melli Payamak account. Pattern IDs live in *Settings → SMS patterns* |
| `BOOTSTRAP_ADMIN_USER/PASSWORD` | first super admin (used only when the database has no users) |
| `WEB_BIND`, `WEB_PORT`, `DB_HOST_PORT` | where containers listen on the host (auto-picked when free); nginx forwards to `WEB_PORT` |
| `EXTRA_DOMAINS` | alias domains (managed by `make add-domain`) |
| `BACKUP_REMOTE`, `BACKUP_REMOTE_PORT` | off-server backup copy (managed by `make backup-remote-setup`) |
| `LEGACY_DEVICE_KEY_SECRET` | see MIGRATION.md |
| `LETSENCRYPT_EMAIL`, `LETSENCRYPT_STAGING` | certificate contact; `true` = test certificates |

After editing: `make up` (or `docker compose up -d --no-build`).

## Domains

```bash
make add-domain DOMAIN=www.shop.evinki.com      # certificate + redirect (301) to the main domain
make remove-domain DOMAIN=www.shop.evinki.com
make change-domain DOMAIN=newname.com           # new main domain; the old one keeps redirecting (DROP=--drop-old to stop)
```

Each needs the domain's DNS A record to point to the server first. If the payment gateway asks for the
shop's domain in its panel, update it there after a domain change.

## Backups and moving servers

| | |
|---|---|
| `make backup` | database + uploads → `backups/` (14 newest kept); copied to `BACKUP_REMOTE` too when set |
| `make backup-remote-setup REMOTE=user@backup-host:/backups/evinki` | creates an SSH key, shows you the line to add on the backup server, tests and sends a first copy |
| `make restore FILE=backups/evinki-….tar.gz` | asks for confirmation; safety backup first |
| `make export` | one file with data **and** `.env` + domains, for a new server (contains secrets — delete after the move) |
| `make migrate-import FILE=evinki-migration-….tar.gz ARGS="--domain new.com --no-ssl"` | on the new server: installs Docker, starts, restores, re-enables domains |

Safe move with no downtime: `migrate-import … --no-ssl` → test the new server → switch DNS → `make ssl`.

## Day to day

| | |
|---|---|
| `make ps` / `make logs` | status / API log |
| `make health` | containers, data, public side, backup age, missing payment/SMS config |
| `make deploy-health HOST=https://shop.evinki.com` | check only the public side |
| `make update` | on a server with git: backup → pull → rebuild → verify → auto roll back |
| `make create-admin U=ali P='Secret-1234'` · `make reset-password U=ali P='…'` · `make admin-password` | accounts from the command line |
| `make seed-demo` | sample catalogue |
| `make ctl CMD="stats"` | any `evinkictl` command |
| `make smoke` | end-to-end test (staging copy with `PAYMENT_GATEWAY=mock`) |

Migrations run automatically and are additive.

## Security notes

* Only the web container publishes a port, on `127.0.0.1` by default; the database listens on loopback only.
* The API trusts `X-Real-IP` only because nginx always overwrites it; never expose the `api` container directly.
* Verify one real payment in sandbox (`ZARINPAL_SANDBOX=true`) before launch.
* Customers sign in with an SMS code: configure SMS patterns **before** launch or nobody can sign in
  (in development the code is returned by the API when `SMS_ENABLED=false`; never in production).
