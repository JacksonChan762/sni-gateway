# SNI Gateway

[v1.1 Beta — What’s New (English / 繁體中文)](docs/releases/v1.1-beta.md)

[English](README.md) | [繁體中文](README.zh-TW.md)

SNI Gateway is a web-managed TLS SNI routing gateway. It accepts HTTP/HTTPS traffic at the edge, matches domains against database-backed rules, and forwards matched HTTPS connections through selected VPN exits.

## Contents

- [How It Works](#how-it-works)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Configuration](#configuration)
- [Basic Workflow](#basic-workflow)
- [Subscription List Format](#subscription-list-format)
- [API Authentication](#api-authentication)
- [API Examples](#api-examples)
  - [Create a WireGuard Exit](#create-a-wireguard-exit)
  - [Add a Rule](#add-a-rule)
  - [Add a Subscription](#add-a-subscription)
  - [Publish Nginx Config](#publish-nginx-config)
  - [Test Routing](#test-routing)
- [Operations Notes](#operations-notes)
- [Development](#development)
- [Documentation](#documentation)

## How It Works

```
Client
  │
  ├── port 80  → nginx (HTTP) → redirect matched hosts to HTTPS
  │
  └── port 443 → nginx (stream, ssl_preread)
                    │
                    ├── SNI matched → relay sidecar (inside gluetun VPN namespace)
                    │                   └── proxy_pass $sni_host:443 via VPN
                    │
                    └── SNI not matched → default backend (no VPN)
```

The stack:

- **Nginx** — edge HTTP redirect and TCP stream routing by TLS SNI.
- **Gluetun** — one container per VPN exit; creates an isolated VPN network namespace.
- **Relay sidecar** — one nginx container per exit that runs inside the Gluetun network namespace and forwards SNI traffic through the VPN.
- **Go/Fiber backend** — REST API that manages exits, rules, and subscriptions; generates and applies nginx config; creates/removes Docker containers for exits.
- **React admin UI** — served by the backend; communicates with the API using a JWT token.
- **PostgreSQL** — stores exits, rules, subscriptions, and publish history.

Rules are stored in the database. Publish and apply manual changes to activate them in nginx; subscription syncs also automatically publish the generated configuration.

## Prerequisites

- Docker 26+
- Docker Compose v2
- Ports 80 and 443 free on the host

Generate a strong `JWT_SECRET` before first boot:

```bash
openssl rand -hex 32
```

## Quick Start

Create a `.env` file:

```bash
cp .env.example .env
# Edit .env and set JWT_SECRET and ADMIN_PASSWORD
```

Or write it directly:

```bash
cat > .env <<'EOF'
JWT_SECRET=replace-with-a-long-random-secret
ADMIN_USERNAME=admin
ADMIN_PASSWORD=replace-this-password
EOF
```

Start the stack:

```bash
docker compose up --build -d
```

Check health:

```bash
curl http://127.0.0.1:8080/health
```

Open the admin UI:

```
http://127.0.0.1:8080
```

Default service ports:

| Port | Purpose |
| --- | --- |
| `8080` | Admin UI and API |
| `80` | Edge HTTP (redirects matched hosts to HTTPS) |
| `443` | Edge HTTPS / SNI routing |

## Configuration

Environment variables passed to the backend:

| Variable | Default | Purpose |
| --- | --- | --- |
| `JWT_SECRET` | `change-me-in-production` | Signs admin JWTs. Always set this in `.env`. |
| `ADMIN_USERNAME` | `admin` | Admin login username. |
| `ADMIN_PASSWORD` | `admin` | Admin login password. Always set this in `.env`. |
| `DATABASE_URL` | compose-managed | PostgreSQL connection string. |
| `DOCKER_SOCKET` | `/var/run/docker.sock` | Docker socket used to create exit containers. |
| `NGINX_CONFIG_DIR` | `/etc/nginx/conf.d` | Shared volume where generated nginx config is written. |
| `EXIT_RESTART_CRON` | `@every 12h` | Schedule for restarting enabled, running exits; set to `disabled` to turn it off. |

The backend needs Docker socket access because it creates and removes `gluetun-*` and `relay-*` containers when exits are started or deleted.
Enabled exits whose recorded status is running are restarted every 12 hours by default. Manually stopped exits stay stopped.

## Automatic Updates

- **Application:** optional daily updates with database backups and service health checks. See [setup and recovery instructions](docs/UPDATES.md).
- **Dashboard:** live status every 15 seconds, with pause and manual refresh controls.
- **Subscriptions:** enabled lists sync on their configured schedule and automatically publish routing changes.

```bash
bash scripts/update.sh --check
bash scripts/update.sh --apply
```

Docker builds the UI directly from source, so application updates include frontend changes.

## Basic Workflow

1. Sign in to the admin UI at `http://127.0.0.1:8080`.
2. Create a VPN exit (WireGuard or OpenVPN via Gluetun).
3. Start the exit; check its status and public IP.
4. Create manual routing rules or add a subscription.
5. Go to Publish → preview → apply to write and reload nginx config.
6. Test the target domain through the gateway.

Manual routing changes require Publish → Apply. Successful subscription syncs automatically regenerate and reload nginx configuration, including any pending manual routing changes.

## Subscription List Format

Subscriptions download a remote domain-list file and convert each line into a routing rule. The parser accepts these line formats:

```
# comment lines are ignored

# plain domain → exact match rule
example.com

# dot prefix → suffix match rule (matches example.com and all subdomains)
+.example.com

# Clash/Surge/Meta style rules preserve exact/suffix matching
DOMAIN,example.com
DOMAIN-SUFFIX,example.com
```

Subscription rules get priority `50`. Manual rules default to priority `100` and are evaluated first.

Domain names are normalized to lowercase and duplicate rules are removed. Optional policy columns (for example, `DOMAIN-SUFFIX,example.com,Proxy`) are accepted; unsupported rule types and invalid hostnames are skipped. Use ASCII or punycode domain names. Downloads time out after 30 seconds and files larger than 10 MiB are rejected. Rules are replaced in a single database transaction with batched inserts, preserving the old rules if a database write fails. An nginx publish failure is reported separately after the database transaction commits.

## API Authentication

All `/api/*` routes except `/api/auth/login` require a bearer token.

Login and save the token:

```bash
TOKEN="$(
  curl -s -X POST http://127.0.0.1:8080/api/auth/login \
    -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"replace-this-password"}' |
  sed -n 's/.*"token":"\([^"]*\)".*/\1/p'
)"
```

Use the token:

```bash
curl http://127.0.0.1:8080/api/stats \
  -H "Authorization: Bearer $TOKEN"
```

Tokens expire after 24 hours. The UI checks expiry before each request and returns to the login page when the token is expired.

## API Examples

### Create a WireGuard Exit

```bash
curl -X POST http://127.0.0.1:8080/api/exits \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "us",
    "enabled": true,
    "config_yaml": "enabled: true\nservices:\n  gluetun:\n    image: qmcgaw/gluetun:latest\n    environment:\n      - VPN_SERVICE_PROVIDER=surfshark\n      - VPN_TYPE=wireguard\n      - SERVER_COUNTRIES=United States\n      - SERVER_CITIES=\n      - WIREGUARD_PRIVATE_KEY=YOUR_PRIVATE_KEY\n      - WIREGUARD_ADDRESSES=10.14.0.2/16\n"
  }'
```

Exit config is stored in normalized form. The UI edits `services.gluetun.environment` values and preserves additional Gluetun environment keys, including custom WireGuard provider settings from wg0-style configs. Imported WireGuard `DNS = ...` values are stored as `WIREGUARD_DNS` and translated to Gluetun plain upstream DNS settings.

### Add a Rule

```bash
curl -X POST http://127.0.0.1:8080/api/rules \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "pattern": "example.com",
    "type": "suffix",
    "exit_id": 1,
    "priority": 100,
    "enabled": true
  }'
```

Rule types:

- `exact` — match one hostname exactly.
- `suffix` — match the hostname and all subdomains.
- `auto` or omitted — `+.` patterns become suffix rules; all other patterns become exact rules.

Higher-priority rules are evaluated first. Manual rules default to `100`; subscription rules use `50`.

### Add a Subscription

```bash
curl -X POST http://127.0.0.1:8080/api/subscriptions \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "AI Sites",
    "url": "https://example.com/ai.list",
    "exit_id": 1,
    "enabled": true,
    "interval_minutes": 360
  }'
```

The backend syncs the list immediately and reschedules it every `interval_minutes`.

### Publish Nginx Config

Preview:

```bash
curl -X POST http://127.0.0.1:8080/api/publish/preview \
  -H "Authorization: Bearer $TOKEN"
```

Apply:

```bash
curl -X POST http://127.0.0.1:8080/api/publish/apply \
  -H "Authorization: Bearer $TOKEN"
```

Rollback:

```bash
curl -X POST http://127.0.0.1:8080/api/publish/rollback \
  -H "Authorization: Bearer $TOKEN"
```

### Test Routing

If DNS already points the domain to this server:

```bash
curl -I http://example.com/
curl -vk https://example.com/
```

If DNS does not point to the server yet, use `--resolve`:

```bash
curl -I --resolve example.com:80:YOUR_SERVER_IP http://example.com/
curl -vk --resolve example.com:443:YOUR_SERVER_IP https://example.com/
```

Raw TLS SNI test:

```bash
openssl s_client -connect YOUR_SERVER_IP:443 -servername example.com
```

## Operations Notes

- Run Publish → Apply after manual routing changes. Subscription syncs automatically publish the generated config, including pending manual changes.
- Running exits are reconciled automatically after name or runtime config changes; stopped exits are recreated on next start.
- Enabled, running exits restart automatically every 12 hours by default; stopped exits stay stopped.
- Deleting an exit is blocked when active rules or active subscriptions still reference it. Disable or delete those dependencies first.
- Disabling or deleting a subscription removes its generated rules and automatically reloads the generated nginx config.
- The relay image (`sni-gateway-relay:latest`) is built by compose. Exit creation expects this image to exist, so always run `docker compose up --build` for the first start.
- Port `443` is the real SNI forwarding path. Port `80` only redirects matched hosts to HTTPS.
- Subscription rules get priority `50` and are always overridden by manual rules at the default priority `100`.

## Development

Backend:

```bash
cd backend
go test ./...
go build ./...
```

Frontend (requires Node.js 16, Vite 4):

```bash
cd frontend
npm install
npm run build
```

The frontend build writes static files into `backend/public/`, which the backend serves at the root.

## Documentation

- API reference: [docs/API.md](docs/API.md)

## Routing and shutdown controls

- Routing uses native nginx hostname maps. Priority is resolved before publishing, including conflicts between exact and suffix rules. Higher priority wins; ties use the lowest rule ID. Domains are case-insensitive. HTTP matching ignores the Host port.
- Duplicate map keys and suffix regex scans are eliminated. Publish → Test domain routing previews saved rules, including unpublished changes; it is not a live connectivity test.
- Exits → Stop All stops each VPN and its relay, reports individual failures, and keeps rules/subscriptions. Start enabled exits again to resume. A stopped route remains unavailable until restarted; it does not silently fall back to another exit.
- To stop the complete stack **including dynamically created exits**, retaining settings and volumes:

  ```bash
  bash scripts/stop.sh
  ```

  The script stops the backend first, stops database-listed VPN/relay containers, then stops the Compose services. Run `docker compose up -d` to bring back the admin stack, then start the desired exits in the UI. Plain `docker compose stop` does not manage dynamically created containers. Backend-only restarts leave VPN containers running.

Validation: `go test -race ./...` from `backend`; `npm ci && npm run build` from `frontend`; `python3 -m unittest discover -s scripts/tests`. The nginx integration test additionally needs the nginx executable and `/usr/lib/nginx/modules/ngx_stream_module.so`; otherwise it reports a skip. Matching benchmarks: `go test ./internal/services -run '^$' -bench BenchmarkRoutingMatch -benchmem`.
