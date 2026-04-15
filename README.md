# SNI Gateway

SNI Gateway is a web-managed TLS SNI routing gateway. It accepts HTTP/HTTPS traffic at the edge, matches domains against database-backed rules, and forwards matched HTTPS connections through selected VPN exits.

The stack combines:

- Nginx for edge HTTP redirects and TCP stream routing.
- Gluetun containers for VPN exits.
- A relay sidecar per exit for SNI-aware forwarding through that VPN namespace.
- A Go/Fiber backend API.
- A React/Vite admin UI.
- PostgreSQL for exits, rules, subscriptions, and publish history.

## What It Does

- Routes port `443` traffic by TLS SNI.
- Redirects matched port `80` traffic to HTTPS.
- Manages multiple VPN exits from the UI or API.
- Imports subscription lists into generated rules.
- Previews, validates, applies, and rolls back generated nginx config.
- Protects the admin API and UI with JWT login.

## Quick Start

Create a `.env` file before first boot:

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

```txt
http://127.0.0.1:8080
```

The default service ports are:

- Admin UI and API: `http://127.0.0.1:8080`
- Edge HTTP: `http://127.0.0.1`
- Edge HTTPS/SNI routing: `https://127.0.0.1`

## Configuration

The compose file passes these important backend settings:

| Variable | Default | Purpose |
| --- | --- | --- |
| `JWT_SECRET` | `change-me-in-production` | Signs admin JWTs. Set this in `.env`. |
| `ADMIN_USERNAME` | `admin` | Admin login username. |
| `ADMIN_PASSWORD` | `admin` | Admin login password. Set this in `.env`. |
| `DATABASE_URL` | compose-managed | PostgreSQL connection string. |
| `DOCKER_SOCKET` | `/var/run/docker.sock` | Docker socket used to create exit containers. |
| `NGINX_CONFIG_DIR` | `/etc/nginx/conf.d` | Shared directory where generated nginx config is written. |

The backend needs Docker socket access because it creates and removes `gluetun-*` and `relay-*` containers for exits.

## Basic Workflow

1. Sign in to the admin UI.
2. Create a VPN exit.
3. Start or restart the exit and check logs/status.
4. Create manual rules or add a subscription.
5. Use Publish to preview and apply nginx config.
6. Test the target domain through the gateway.

Rules and subscriptions are stored in the database first. They do not affect nginx until you apply the generated config.

## API Authentication

All `/api/*` routes except `/api/auth/login` require a bearer token.

Login:

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

Tokens expire after 24 hours. The UI stores the expiry and returns to login before using an expired token.

## Create a WireGuard Exit

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

Exit config is stored in normalized form. The UI edits the `services.gluetun.environment` values and preserves additional Gluetun environment keys.

## Add a Rule

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

- `exact`: match one hostname exactly.
- `suffix`: match the hostname and subdomains.

Higher priority rules are evaluated first.

## Publish Nginx Config

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

## Test Routing

If DNS already points the domain to this gateway:

```bash
curl -I http://example.com/
curl -vk https://example.com/
```

If DNS does not point to the gateway yet:

```bash
curl -I --resolve example.com:80:YOUR_SERVER_IP http://example.com/
curl -vk --resolve example.com:443:YOUR_SERVER_IP https://example.com/
```

Raw TLS SNI test:

```bash
openssl s_client -connect YOUR_SERVER_IP:443 -servername example.com
```

## Operations Notes

- Changing exits, rules, or subscriptions does not automatically publish nginx config.
- Exit config updates change database state; restart the exit to recreate containers with new runtime settings.
- Deleting an exit is blocked when active rules or active subscriptions still reference it.
- Disabled or deleted subscriptions remove their generated rules and reload generated config through the backend.
- The relay image is built by compose as `sni-gateway-relay:latest`; exit creation expects that image to exist.
- Port `443` is the real SNI forwarding path. Port `80` only redirects matched hosts to HTTPS.

## Development

Backend:

```bash
cd backend
go test ./...
```

Frontend:

```bash
cd frontend
npm install
npm run build
```

The frontend build writes static files into `backend/public`, which the backend serves.

## Documentation

- API reference: [docs/API.md](docs/API.md)
