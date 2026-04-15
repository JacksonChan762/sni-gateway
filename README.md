# SNI Gateway

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

Rules are stored in the database. They become active in nginx only after you publish and apply the generated config.

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

The backend needs Docker socket access because it creates and removes `gluetun-*` and `relay-*` containers when exits are started or deleted.

## Basic Workflow

1. Sign in to the admin UI at `http://127.0.0.1:8080`.
2. Create a VPN exit (WireGuard or OpenVPN via Gluetun).
3. Start the exit; check its status and public IP.
4. Create manual routing rules or add a subscription.
5. Go to Publish → preview → apply to write and reload nginx config.
6. Test the target domain through the gateway.

Rules and subscriptions are stored in the database first. They do not affect nginx until you apply the generated config.

## Subscription List Format

Subscriptions download a remote domain-list file and convert each line into a routing rule. The parser accepts these line formats:

```
# comment lines are ignored

# plain domain → exact match rule
example.com

# dot prefix → suffix match rule (matches example.com and all subdomains)
+.example.com

# Clash/Surge/Meta style prefixes are stripped before parsing
DOMAIN,example.com
DOMAIN-SUFFIX,example.com
```

Subscription rules get priority `50`. Manual rules default to priority `100` and are evaluated first.

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

Exit config is stored in normalized form. The UI edits `services.gluetun.environment` values and preserves additional Gluetun environment keys.

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

- Changing exits, rules, or subscriptions does not automatically publish nginx config. Run Publish → Apply after routing changes.
- Exit config updates change database state only. Restart the exit to recreate containers with new settings.
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
