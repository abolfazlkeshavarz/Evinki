# Evinki platform - everyday commands. Run `make help`.
COMPOSE ?= docker compose
.DEFAULT_GOAL := help

help: ## show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[1m%-22s\033[0m %s\n",$$1,$$2}'

# ---------- first install ----------
env: ## create .env with random secrets and FREE ports:  DOMAIN=shop.evinki.com LETSENCRYPT_EMAIL=me@x.com make env   (DEV=1 for local testing)
	@./scripts/init-env.sh

secrets: env ## alias of env

bootstrap: ## everything on a fresh server (Docker, .env, start, HTTPS, backups): DOMAIN=... LETSENCRYPT_EMAIL=... make bootstrap
	@./scripts/bootstrap-vps.sh

ssl: ## certificate + host nginx for APP_DOMAIN (DNS must already point here)
	@./scripts/deploy-host-nginx.sh

deploy-check: ## verify the server/.env is ready before the first deployment
	@./scripts/deploy-check.sh

# ---------- run ----------
up: ## build (if needed) and start everything
	@test -f .env || ./scripts/init-env.sh
	$(COMPOSE) up -d --build
	@./scripts/health.sh || true

up-prebuilt: ## start with images that were loaded (make load-images), no building
	$(COMPOSE) up -d --no-build
	@./scripts/health.sh || true

down: ## stop (data is kept in Docker volumes)
	$(COMPOSE) down

restart: ## restart the app containers
	$(COMPOSE) restart api web

ps: ## container status
	$(COMPOSE) ps

logs: ## follow API logs (the first-run admin password is printed here if you did not set one)
	$(COMPOSE) logs -f --tail=100 api

health: ## containers, data, public side, backups
	@./scripts/health.sh

deploy-health: ## check a public address:  make deploy-health HOST=https://shop.evinki.com
	@REMOTE=1 ./scripts/health.sh "$(HOST)"

update: ## on the server with git: backup, pull, rebuild, verify, roll back if unhealthy
	@./scripts/update.sh

# ---------- deploy from your PC (builds here, pushes over SSH, keeps a rollback) ----------
images-bundle: ## build images on this PC into dist/evinki-images.tar.gz (PLATFORM=linux/arm64 for ARM)
	@./scripts/build-images.sh

images: images-bundle ## alias of images-bundle

load-images: ## on the server: load dist/evinki-images.tar.gz (FILE=... to choose)
	@./scripts/load-images.sh $(FILE)

deploy-remote: ## build here and deploy:  make deploy-remote HOST=root@server  [DIR=/opt/evinki] [DOMAIN=.. LETSENCRYPT_EMAIL=..]
	@./scripts/deploy-remote.sh

rollback-remote: ## go back to the previous version on the server:  make rollback-remote HOST=root@server
	@./scripts/deploy-remote.sh --rollback

# ---------- domains ----------
add-domain: ## extra domain that redirects to the main one:  make add-domain DOMAIN=www.shop.evinki.com
	@./scripts/add-domain.sh "$(DOMAIN)"

remove-domain: ## stop serving an extra domain:  make remove-domain DOMAIN=www.shop.evinki.com
	@./scripts/remove-domain.sh "$(DOMAIN)"

change-domain: ## move to a new main domain (old one redirects):  make change-domain DOMAIN=new.com [DROP=--drop-old]
	@./scripts/change-domain.sh "$(DOMAIN)" $(DROP)

# ---------- data ----------
backup: ## database + uploads -> ./backups (14 kept; copied off-server if BACKUP_REMOTE is set)
	@./scripts/backup.sh

backup-remote-setup: ## copy every backup to a second server:  make backup-remote-setup REMOTE=user@host:/path
	@./scripts/backup-remote-setup.sh "$(REMOTE)"

restore: ## restore a backup:  make restore FILE=backups/evinki-YYYYmmdd-HHMM.tar.gz
	@./scripts/restore.sh "$(FILE)"

export: ## one file with everything needed to move to a new server (data + settings)
	@./scripts/export-bundle.sh

migrate-import: ## on the NEW server: make migrate-import FILE=evinki-migration-....tar.gz [ARGS="--domain new.com --no-ssl"]
	@./scripts/migrate-import.sh "$(FILE)" $(ARGS)

shell-db: ## psql shell
	$(COMPOSE) exec db psql -U evinki evinki

# ---------- operators ----------
ctl: ## admin tool:  make ctl CMD="create-user -username ali -password Secret-123 -roles admin"
	@./scripts/admin.sh $(CMD)

create-admin: ## make create-admin U=ali P='Secret-1234'
	@./scripts/admin.sh create-user -username "$(U)" -password "$(P)" -roles admin

reset-password: ## make reset-password U=ali P='New-secret-1'
	@./scripts/admin.sh reset-password -username "$(U)" -password "$(P)"

admin-password: ## new random password for the first admin (U=name for another user, PASSWORD=... to choose)
	@./scripts/rotate-admin-password.sh

seed-catalog: ## load the real products and photos of evinki.com (safe to re-run; keeps your edits)
	@./scripts/admin.sh seed-catalog

seed-demo: ## seed-catalog + service items, shift, departments, standard survey
	@./scripts/demo-data.sh

demo: seed-demo ## alias of seed-demo

stats: ## row counts of the main tables
	@./scripts/admin.sh stats

import-warehouse: ## old warehouse system:  make import-warehouse ARGS="-src sqlite:/legacy/warehouse.db -images /legacy/parts -dry-run"
	@./scripts/import-legacy.sh warehouse $(ARGS)

import-guarantee: ## old guarantee system:   make import-guarantee ARGS="-src postgres://... -dry-run"
	@./scripts/import-legacy.sh guarantee $(ARGS)

import-attendance: ## old attendance system: make import-attendance ARGS="-src sqlite:/legacy/attendance.db -dry-run"
	@./scripts/import-legacy.sh attendance $(ARGS)

smoke: ## end-to-end test of the main flows (needs PAYMENT_GATEWAY=mock; creates its own data)
	@./scripts/smoke.sh

# ---------- development ----------
dev-db: ## PostgreSQL for local development (port 55433)
	docker run -d --name evinki-dev-db -e POSTGRES_PASSWORD=dev -e POSTGRES_DB=evinki -p 127.0.0.1:55433:5432 postgres:16-alpine || docker start evinki-dev-db

dev-api: ## run the API from source
	cd backend && DATABASE_URL="postgres://postgres:dev@127.0.0.1:55433/evinki?sslmode=disable" BOOTSTRAP_ADMIN_PASSWORD=dev-admin-pass go run ./cmd/api

dev-web: ## run the web app with hot reload on :5173
	cd frontend && npm install && npm run dev

test: ## unit tests + type check
	cd backend && go vet ./... && go test ./...
	cd frontend && npm run typecheck

.PHONY: help env secrets bootstrap ssl deploy-check up up-prebuilt down restart ps logs health deploy-health update images-bundle images load-images deploy-remote rollback-remote add-domain remove-domain change-domain backup backup-remote-setup restore export migrate-import shell-db ctl create-admin reset-password admin-password seed-catalog seed-demo demo stats import-warehouse import-guarantee import-attendance smoke dev-db dev-api dev-web test
