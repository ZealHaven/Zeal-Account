set dotenv-load := true
set shell := ["bash", "-uc"]

backend_dir  := "backend/account"
frontend_dir := "frontend/account"
bin_dir      := "bin"
db_url       := env("DATABASE_URL", "postgres://account:account@localhost:5440/account?sslmode=disable")
# ssh alias of the staging box — set in .env (git-ignored), never committed
staging_host := env("STAGING_HOST", "staging")

default:
    @just --list

# --- infra -------------------------------------------------------------------

up:
    docker compose up -d --wait

down:
    docker compose down

# --- migrations --------------------------------------------------------------
# Schema only — data lives in database/seeds/ (see seeds section below).

migrate-up:
    migrate -path database/migrations -database "{{db_url}}" up

migrate-down:
    migrate -path database/migrations -database "{{db_url}}" down 1

migrate-new name:
    migrate -path database/migrations -database "{{db_url}}" create -ext sql {{name}}

# --- seeds -------------------------------------------------------------------
# Idempotent (ON CONFLICT DO NOTHING) — safe to re-run at any time.

# Master data the app needs to function (all environments, after migrate-up)
seed-required:
    psql "{{db_url}}" -v ON_ERROR_STOP=1 -f database/seeds/required.sql

# Demo/test rows — local dev and integration tests only, never production
seed-demo:
    psql "{{db_url}}" -v ON_ERROR_STOP=1 -f database/seeds/demo.sql

# Both files (local dev, fresh-DB test runs)
seed: seed-required seed-demo

# Fresh OAuth client secret pair: plaintext (product .env) + bcrypt hash (seed)
client-secret:
    cd {{backend_dir}} && go run ./cmd/clientsecret

# --- dev ---------------------------------------------------------------------

# Backend (:8082) + Vite dev server (:5273) behind the Go proxy
dev:
    #!/usr/bin/env bash
    set -euo pipefail
    ( cd {{frontend_dir}} && npm run dev -- --port 5273 ) &
    vite_pid=$!
    trap 'kill $vite_pid 2>/dev/null || true' EXIT
    cd {{backend_dir}}
    go run ./cmd/accountd

dev-backend:
    cd {{backend_dir}} && go run ./cmd/accountd

dev-frontend:
    cd {{frontend_dir}} && npm run dev -- --port 5273

# --- build / test ------------------------------------------------------------

build: build-backend build-frontend

build-backend:
    mkdir -p {{bin_dir}}
    cd {{backend_dir}} && go build -o ../../{{bin_dir}}/accountd ./cmd/accountd

build-frontend:
    cd {{frontend_dir}} && npm run build

test:
    cd {{backend_dir}} && go test ./...

test-cover:
    cd {{backend_dir}} && go test -cover ./...

# Full OIDC/API flows against real Postgres + Redis (requires `just up`,
# `just migrate-up`, `just seed` — phase 3 needs the zeal-test client).
# NOTE: tests share the dev DATABASE_URL and rotate signing_keys — after
# running them, restart `just dev` so the server regenerates its own key.
test-integration:
    cd {{backend_dir}} && go test -tags=integration ./...

check: check-backend check-frontend

check-backend:
    cd {{backend_dir}} && go vet ./... && gofmt -l .

check-frontend:
    cd {{frontend_dir}} && npm run check

# --- staging -----------------------------------------------------------------

# Ship to staging (https://account.zealhaven.id); idempotent.
# Requires STAGING_HOST — see the header of ops/deploy-staging.sh.
# First-time extras (already done): cloudflared ingress + DNS route —
# see the header of ops/deploy-staging.sh.
deploy-staging:
    ops/deploy-staging.sh

# Service status / recent logs on the staging box
staging-status:
    ssh {{staging_host}} 'systemctl --user status accountd account-web --no-pager || true'

staging-logs service="accountd":
    ssh {{staging_host}} 'journalctl --user -u {{service}} -n 200 --no-pager'

fmt:
    cd {{backend_dir}} && gofmt -w .
    cd {{frontend_dir}} && npm run format
