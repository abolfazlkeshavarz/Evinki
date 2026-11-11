# Updating the running system with an image bundle

Build on your development computer, upload the compressed file, done. The server never needs Go or Node.

```
 dev PC: make images-bundle ──► dist/evinki-images.tar.gz ──► admin panel  (or: make push-update)
                                                                  │
 server:  API stores the file ──► updater service: backup → load images → restart → health check
                                                         └─ unhealthy / bad file → previous images come back
```

## One-time setup on the server

```bash
cd /opt/evinki
git pull && docker compose up -d --build   # this version once (adds the 'updates' volume)
sudo make install-updater                  # systemd service that applies uploaded bundles
make ssl                                   # only if the host nginx was installed before: allows 3 GB uploads on one URL
```

The admin screen **سامانه ← به‌روزرسانی سیستم** shows whether the service is running.
The API container never touches Docker: it only saves the file in the `updates` volume; the host service does the work.

## Every release

1. On your PC: `make images-bundle` (`PLATFORM=linux/arm64` for ARM servers; set `GOPROXY` in `.env` if proxy.golang.org is blocked for you,
   e.g. `GOPROXY=https://package-mirror.liara.ir/repository/go/,direct`).
2. Admin panel → *به‌روزرسانی سیستم* → drop `dist/evinki-images.tar.gz` → confirm. Progress, log and history are shown.
   Or from the PC: `make push-update URL=https://shop.evinki.com ADMIN=admin` (asks for the password; builds first unless `FILE=` is given).

What happens: a backup (`backups/`), the images are loaded and checked (a file without `evinki-api`/`evinki-web` is refused, the wrong
CPU architecture is refused), `docker compose up -d --no-build`, then the health check. If the new version is not healthy, the previous
images (`:previous`) are put back automatically. Database migrations only add things, so going back is safe. A file that cannot be loaded
does not restart anything.

Tested end to end on a real Docker stack: good bundle → done; garbage bundle → refused without a restart.
