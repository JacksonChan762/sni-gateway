# sni-gateway

`sni-gateway` is a web-managed SNI/Host based gateway for routing traffic through multiple VPN exits.

It combines:

- `nginx` as the edge gateway
- `gluetun` + `relay` containers as VPN exits
- a Go backend API
- a React frontend for exits, rules, subscriptions, and publish operations

## What It Does

- Route `443` traffic by TLS SNI
- Redirect `80` traffic to HTTPS for matched domains
- Manage multiple VPN exits from the UI or API
- Sync rule subscriptions from remote `.list` files
- Generate and reload nginx config from database state

## Stack

- Backend: Go + Fiber + Gorm
- Frontend: React + Vite
- Database: PostgreSQL
- Edge: Nginx
- VPN exit runtime: Gluetun + relay sidecar

## Services

- Frontend + API: `http://127.0.0.1:8080`
- Edge HTTP: `http://127.0.0.1`
- Edge HTTPS: `https://127.0.0.1`

## Quick Start

```bash
docker compose up --build -d
```

Health check:

```bash
curl http://127.0.0.1:8080/health
```

Expected response:

```txt
ok
```

## Basic Workflow

1. Create an exit
2. Start or verify the exit is running
3. Create one or more rules pointing to that exit
4. Apply publish config
5. Test the target domain through `443`

## Example: Create a WireGuard Exit

```bash
curl -X POST http://127.0.0.1:8080/api/exits \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "us",
    "protocol": "wireguard",
    "provider": "surfshark",
    "country": "United States",
    "config_yaml": "services:\n  gluetun:\n    environment:\n      - VPN_SERVICE_PROVIDER=surfshark\n      - VPN_TYPE=wireguard\n      - SERVER_COUNTRIES=United States\n      - SERVER_CITIES=\n      - WIREGUARD_PRIVATE_KEY=YOUR_PRIVATE_KEY\n      - WIREGUARD_ADDRESSES=10.14.0.2/16\n",
    "enabled": true
  }'
```

## Example: Add a Rule

```bash
curl -X POST http://127.0.0.1:8080/api/rules \
  -H 'Content-Type: application/json' \
  -d '{
    "pattern": "ping0.cc",
    "type": "suffix",
    "exit_id": 1,
    "priority": 100,
    "enabled": true
  }'
```

## Example: Apply Nginx Config

```bash
curl -X POST http://127.0.0.1:8080/api/publish/apply
```

## How To Test Routing

If DNS already points `ping0.cc` to this gateway:

```bash
curl -I http://ping0.cc
curl -vk https://ping0.cc/
```

If DNS does not point to this gateway yet:

```bash
curl -I --resolve ping0.cc:80:YOUR_SERVER_IP http://ping0.cc/
curl -vk --resolve ping0.cc:443:YOUR_SERVER_IP https://ping0.cc/
```

To test raw TLS SNI:

```bash
openssl s_client -connect YOUR_SERVER_IP:443 -servername ping0.cc
```

## Important Behavior

- Creating or changing rules does not update nginx immediately
- You must call `POST /api/publish/apply` or use the UI `發布管理 -> 套用配置`
- `443` is the real SNI forwarding path
- `80` is only used to redirect matched domains to HTTPS

## API Docs

Full API reference with request and response examples:

- [docs/API.md](/root/sni-gateway/docs/API.md)

Internal architecture notes:

- [dev.md](/root/sni-gateway/dev.md)
