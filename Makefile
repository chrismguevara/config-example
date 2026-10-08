# Convenience targets. Everything also works through `docker compose`.
DATABASE_URL ?= postgres://settings:settings@localhost:5432/settings?sslmode=disable

.PHONY: up down migrate migrate-status rollback-one api web test test-db gen-types

up:                ## Start everything (Postgres, Liquibase, API, web)
	docker compose up --build

down:              ## Stop and remove containers (keeps the pgdata volume)
	docker compose down

migrate:           ## Apply pending changesets (with the dev seed)
	docker compose run --rm liquibase update --contexts=dev

migrate-status:    ## Show pending / applied changesets
	docker compose run --rm liquibase status --verbose

rollback-one:      ## Roll back the most recent changeset
	docker compose run --rm liquibase rollback-count 1

api:               ## Run the Go API locally against DATABASE_URL
	DATABASE_URL=$(DATABASE_URL) go run ./backend/cmd/api

web:               ## Run the Vite dev server locally (proxies /api to :8080)
	cd frontend && npm install && npm run dev

gen-types:         ## Regenerate frontend/src/generated/user-settings.ts from the current schema
	cd frontend && npm run gen:types

test:              ## Go unit tests (no database needed)
	go test ./...

test-db:           ## Go unit + integration tests against a migrated DATABASE_URL
	TEST_DATABASE_URL=$(DATABASE_URL) go test ./...
