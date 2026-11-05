#!/usr/bin/env bash
set -euo pipefail
# Deploy Zeal Account to zeal-staging — issuer https://account.zealhaven.id.
#
# One-time prerequisites (wired manually, kept by the server):
#   - cloudflared ingress in ~/.cloudflared/config.yml on the server:
#       - hostname: account.zealhaven.id
#         service: http://localhost:8082
#     plus `cloudflared tunnel route dns <tunnel> account.zealhaven.id`
#   - a user systemd lingering session (loginctl enable-linger)
#
# Everything below is idempotent: re-run to ship changes.
#
# Usage: ops/deploy-staging.sh       (or: just deploy-staging)
cd "$(dirname "${BASH_SOURCE[0]}")/.."

HOST="${STAGING_HOST:-zeal-staging}"
REMOTE="${STAGING_DIR:-/home/ridzeal/Zeal/Account}"
URL="${STAGING_URL:-https://account.zealhaven.id}"

echo "==> [1/5] build backend (static; local Go is newer than the server's)"
mkdir -p bin
(cd backend/account && CGO_ENABLED=0 go build -trimpath -o ../../bin/accountd ./cmd/accountd)

echo "==> [2/5] prepare remote dir"
ssh "$HOST" "mkdir -p '$REMOTE'"

echo "==> [3/5] sync source -> $HOST:$REMOTE"
rsync -a --delete \
    --exclude '.git/' \
    --exclude '.env' \
    --exclude 'node_modules/' \
    --exclude 'bin/' \
    --exclude 'frontend/account/build/' \
    --exclude 'frontend/account/.svelte-kit/' \
    --exclude 'test-results/' \
    --exclude 'playwright-report/' \
    --exclude 'tmp/' \
    ./ "$HOST:$REMOTE/"

echo "==> [4/5] sync binary"
ssh "$HOST" "mkdir -p '$REMOTE/bin'"
rsync -a bin/accountd "$HOST:$REMOTE/bin/accountd"

echo "==> [5/5] remote: infra, secrets, build, migrations, seeds, services"
ssh "$HOST" "REMOTE='$REMOTE' bash -s" <<'REMOTE'
set -euo pipefail
cd "$REMOTE"

# First run: mint the runtime secrets on the server (never in the repo).
if [ ! -f .env ]; then
    cat > .env <<EOF
# Staging runtime config — created by ops/deploy-staging.sh on first deploy.
ACCOUNT_ADDR=127.0.0.1:8082
FRONTEND_URL=http://127.0.0.1:8083
OIDC_ISSUER=https://account.zealhaven.id
COOKIE_SECURE=true
KEY_ENC_HEX=$(openssl rand -hex 32)
GLOBAL_SECRET=$(openssl rand -hex 32)
DATABASE_URL=postgres://account:account@127.0.0.1:5440/account?sslmode=disable
REDIS_URL=redis://127.0.0.1:6390/0
# Identity-verification OTP only; empty SMTP logs codes server-side.
SMTP_HOST=
SMTP_PORT=587
SMTP_USER=
SMTP_PASS=
SMTP_FROM=noreply@zealhaven.id
EOF
    chmod 600 .env
    echo "    created .env (secrets generated server-side)"
fi

just up          # docker compose: postgres + redis on loopback ports
just migrate-up  # schema (forward-only)
just seed-required # master data (idempotent; migrations never INSERT)

# Frontend deps + production build; npm ci only when the lockfile changed.
if [ ! -d frontend/account/node_modules ] \
 || [ frontend/account/package-lock.json -nt frontend/account/node_modules/.package-lock.json ]; then
    echo "==> npm ci (frontend)"
    (cd frontend/account && npm ci)
fi
echo "==> build frontend"
(cd frontend/account && npm run build)

install -d -m 755 ~/.config/systemd/user
install -m 644 ops/systemd/*.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable accountd account-web
systemctl --user restart accountd account-web
sleep 2
systemctl --user is-active accountd account-web

echo "==> local smoke (front door serves API + frontend)"
curl -fsS http://127.0.0.1:8082/healthz
echo
curl -fsS -o /dev/null http://127.0.0.1:8082/login
echo "    GET /login through the front door: OK"
REMOTE

echo "==> public smoke: $URL"
curl -fsS "$URL/healthz"
echo
echo "deployed: $URL"
