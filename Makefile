# Evinki platform - everyday commands. Run `make help`.
COMPOSE ?= docker compose
.DEFAULT_GOAL := help

help: ## show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[1m%-22s\033[0m %s\n",$$1,$$2}'

# ---------- run ----------
secrets: ## create .env with random secrets (only if missing)
	@./scripts/gen-secrets.sh

up: ## build and start everything (database, API, web)
	@test -f .env || ./scripts/gen-secrets.sh
	$(COMPOSE) up -d --build
	@./scripts/health.sh || true

down: ## stop (data is kept in Docker volumes)
	$(COMPOSE) down

restart: ## restart the app containers
	$(COMPOSE) restart api web

ps: ## container status
	$(COMPOSE) ps

logs: ## follow API logs (the first-run admin password is printed here if you did not set one)
	$(COMPOSE) logs -f --tail=100 api

health: ## check that the system answers and the database is reachable
	@./scripts/health.sh

update: ## pull the latest code, backup, rebuild, restart (rolls back if unhealthy)
	@./scripts/update.sh

# ---------- data ----------
backup: ## database + uploads backup into ./backups
	@./scripts/backup.sh

restore: ## restore a backup:  make restore FILE=backups/evinki-YYYYmmdd-HHMM.tar.gz
	@./scripts/restore.sh "$(FILE)"

shell-db: ## psql shell
	$(COMPOSE) exec db psql -U evinki evinki

# ---------- operators ----------
ctl: ## admin tool:  make ctl CMD="create-user -username ali -password Secret-123 -roles admin"
	$(COMPOSE) run --rm --no-deps -T api evinkictl $(CMD)

create-admin: ## make create-admin U=ali P='Secret-123'
	$(COMPOSE) run --rm --no-deps -T api evinkictl create-user -username "$(U)" -password "$(P)" -roles admin

reset-password: ## make reset-password U=ali P='New-secret-1'
	$(COMPOSE) run --rm --no-deps -T api evinkictl reset-password -username "$(U)" -password "$(P)"

seed-demo: ## sample catalogue (FZ530, accessories, services, standard survey)
	$(COMPOSE) run --rm --no-deps -T api evinkictl seed-demo

stats: ## row counts of the main tables
	$(COMPOSE) run --rm --no-deps -T api evinkictl stats

import-warehouse: ## import the old warehouse system:  make import-warehouse ARGS="-src sqlite:/legacy/warehouse.db -images /legacy/parts -dry-run"
	@./scripts/import-legacy.sh warehouse $(ARGS)

import-guarantee: ## import the old guarantee system:   make import-guarantee ARGS="-src postgres://... -dry-run"
	@./scripts/import-legacy.sh guarantee $(ARGS)

import-attendance: ## import the old attendance system: make import-attendance ARGS="-src sqlite:/legacy/attendance.db -dry-run"
	@./scripts/import-legacy.sh attendance $(ARGS)

# ---------- checks ----------
smoke: ## end-to-end test of the main flows (needs PAYMENT_GATEWAY=mock; creates its own data)
	@./scripts/smoke.sh

deploy-check: ## verify the server is ready before the first deployment
	@./scripts/deploy-check.sh

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

# ---------- small servers ----------
images: ## build images on this PC and save to dist/ (PLATFORM=linux/arm64 for ARM)
	@./scripts/build-images.sh

load-images: ## on the server: load dist/evinki-images.tar.gz
	@./scripts/load-images.sh

.PHONY: help secrets up down restart ps logs health update backup restore shell-db ctl create-admin reset-password seed-demo stats import-warehouse import-guarantee import-attendance smoke deploy-check dev-db dev-api dev-web test images load-images
