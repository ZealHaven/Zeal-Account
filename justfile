set dotenv-load := true
set shell := ["bash", "-uc"]

backend_dir  := "backend/account"
frontend_dir := "frontend/account"
bin_dir      := "bin"
db_url       := env("DATABASE_URL", "postgres://account:account@localhost:5440/account?sslmode=disable")

default:
    @just --list

# --- infra -------------------------------------------------------------------

up:
    docker compose up -d --wait

down:
    docker compose down

# --- migrations --------------------------------------------------------------

migrate-up:
    migrate -path migrations -database "{{db_url}}" up

migrate-down:
    migrate -path migrations -database "{{db_url}}" down 1

migrate-new name:
    migrate -path migrations -database "{{db_url}}" create -ext sql {{name}}

# Fresh OAuth client secret pair: plaintext (product .env) + bcrypt hash (migration)
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

# Full OIDC/API flows against real Postgres + Redis (requires `just up`).
# NOTE: tests share the dev DATABASE_URL and rotate signing_keys — after
# running them, restart `just dev` so the server regenerates its own key.
test-integration:
    cd {{backend_dir}} && go test -tags=integration ./...

check: check-backend check-frontend

check-backend:
    cd {{backend_dir}} && go vet ./... && gofmt -l .

check-frontend:
    cd {{frontend_dir}} && npm run check

fmt:
    cd {{backend_dir}} && gofmt -w .
    cd {{frontend_dir}} && npm run format
