# API Reference

Base URL:

```
http://127.0.0.1:8080
```

Public routes (no token required):

- `GET /health`
- `POST /api/auth/login`

All other `/api/*` routes require:

```
Authorization: Bearer <token>
```

## Contents

- [Conventions](#conventions)
- [Health](#health)
- [Auth](#auth)
- [Stats](#stats)
- [Exits](#exits)
- [Rules](#rules)
- [Subscriptions](#subscriptions)
- [Publish](#publish)
- [End-to-End Example](#end-to-end-example)
- [Runtime Notes](#runtime-notes)

## Conventions

- Request and response content type: `application/json`
- Timestamps are RFC 3339 strings.
- Most errors use:

```json
{
  "error": "message"
}
```

Set a shell token for the examples below:

```bash
TOKEN="$(
  curl -s -X POST http://127.0.0.1:8080/api/auth/login \
    -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"admin"}' |
  sed -n 's/.*"token":"\([^"]*\)".*/\1/p'
)"
```

Use a real password if `ADMIN_PASSWORD` is set in `.env`.

---

## Health

### `GET /health`

```bash
curl http://127.0.0.1:8080/health
```

Response:

```
ok
```

---

## Auth

### `POST /api/auth/login`

Create a 24-hour JWT for the admin UI/API.

```bash
curl -X POST http://127.0.0.1:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin"}'
```

Response:

```json
{
  "token": "jwt-token",
  "username": "admin",
  "expires_at": 1776220000
}
```

Errors:

- `400` — malformed JSON or missing username/password.
- `401` — invalid credentials.

---

## Stats

### `GET /api/stats`

Get dashboard summary counts.

```bash
curl http://127.0.0.1:8080/api/stats \
  -H "Authorization: Bearer $TOKEN"
```

Response:

```json
{
  "rule_count": 1,
  "sub_count": 0,
  "exit_count": 1,
  "running_exit_count": 1,
  "last_publish": {
    "id": 1,
    "version": "v1776216000",
    "status": "success",
    "message": "config applied successfully",
    "created_at": "2026-04-15T00:28:36Z"
  }
}
```

---

## Exits

An exit is a VPN runtime backed by a Gluetun container and a relay sidecar.

### Exit Object

```json
{
  "id": 8,
  "name": "us",
  "runtime": "gluetun",
  "protocol": "wireguard",
  "provider": "surfshark",
  "country": "United States",
  "city": "",
  "status": "running",
  "public_ip": "104.255.173.142",
  "enabled": true,
  "created_at": "2026-04-15T00:20:22Z",
  "updated_at": "2026-04-15T00:20:22Z"
}
```

`config_json` is intentionally scrubbed from list/create/update responses.

### `GET /api/exits`

List exits.

```bash
curl http://127.0.0.1:8080/api/exits \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/exits`

Create an exit. If `enabled` is true, the backend creates and starts runtime containers immediately.

WireGuard example:

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

OpenVPN example:

```bash
curl -X POST http://127.0.0.1:8080/api/exits \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "jp",
    "enabled": true,
    "config_yaml": "enabled: true\nservices:\n  gluetun:\n    image: qmcgaw/gluetun:latest\n    environment:\n      - VPN_SERVICE_PROVIDER=protonvpn\n      - VPN_TYPE=openvpn\n      - SERVER_COUNTRIES=Japan\n      - SERVER_CITIES=\n      - OPENVPN_USER=YOUR_USER\n      - OPENVPN_PASSWORD=YOUR_PASSWORD\n"
  }'
```

Accepted fields:

| Field | Type | Notes |
| --- | --- | --- |
| `name` | string | Required. |
| `runtime` | string | Optional. Default `gluetun`. |
| `protocol` | `wireguard` \| `openvpn` | Optional. Derived from `VPN_TYPE` in `config_yaml`. |
| `provider` | string | Optional. Derived from `VPN_SERVICE_PROVIDER`. |
| `country` | string | Optional. Derived from `SERVER_COUNTRIES`. |
| `city` | string | Optional. Derived from `SERVER_CITIES`. |
| `wireguard_private_key` | string | Optional when `config_yaml` supplies `WIREGUARD_PRIVATE_KEY`. |
| `wireguard_addresses` | string | Optional. |
| `openvpn_user` | string | Optional when `config_yaml` supplies `OPENVPN_USER`. |
| `openvpn_password` | string | Optional when `config_yaml` supplies `OPENVPN_PASSWORD`. |
| `config_yaml` | string | Optional. Compose-style YAML with `services.gluetun.environment`. |
| `enabled` | bool | Optional. Default `true`. |

Notes:

- `config_yaml` environment may be a list of `KEY=VALUE` entries or a key-value mapping.
- WireGuard exits require `WIREGUARD_PRIVATE_KEY`.
- OpenVPN exits require `OPENVPN_USER` and `OPENVPN_PASSWORD`.
- Extra Gluetun environment variables are preserved across config round trips and passed to the Gluetun container.
- `VPN_SERVICE_PROVIDER=custom` is supported for WireGuard. Custom WireGuard exits require `WIREGUARD_ENDPOINT_IP`, `WIREGUARD_ENDPOINT_PORT`, `WIREGUARD_PUBLIC_KEY`, `WIREGUARD_PRIVATE_KEY`, and `WIREGUARD_ADDRESSES`; the UI can import wg0-style ini configs. Imported `DNS = ...` values are stored as `WIREGUARD_DNS` and passed to Gluetun as plain upstream DNS addresses.

### `PUT /api/exits/:id`

Update exit metadata/config in the database.

```bash
curl -X PUT http://127.0.0.1:8080/api/exits/8 \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "enabled": true,
    "config_yaml": "enabled: true\nservices:\n  gluetun:\n    image: qmcgaw/gluetun:latest\n    environment:\n      - VPN_SERVICE_PROVIDER=surfshark\n      - VPN_TYPE=wireguard\n      - SERVER_COUNTRIES=United States\n      - SERVER_CITIES=New York\n      - WIREGUARD_PRIVATE_KEY=YOUR_PRIVATE_KEY\n"
  }'
```

Notes:

- Exit `name` can be changed. If the running runtime needs to be recreated, old `gluetun-*` and `relay-*` containers are removed and recreated under the new name.
- Config changes update database state first. Running exits are reconciled automatically when runtime-affecting fields change; stopped exits are recreated on next start.

### `DELETE /api/exits/:id`

Delete an exit and its runtime containers.

```bash
curl -X DELETE http://127.0.0.1:8080/api/exits/8 \
  -H "Authorization: Bearer $TOKEN"
```

Response:

```json
{
  "message": "deleted"
}
```

Blocked when active dependencies exist:

```json
{
  "error": "cannot delete exit with active dependencies",
  "active_rule_count": 1,
  "inactive_rule_count": 0,
  "active_subscription_count": 1,
  "inactive_subscription_count": 0
}
```

Inactive rule/subscription references are cleared automatically on delete.

### `POST /api/exits/:id/start`

```bash
curl -X POST http://127.0.0.1:8080/api/exits/8/start \
  -H "Authorization: Bearer $TOKEN"
```

Response:

```json
{
  "message": "started"
}
```

### `POST /api/exits/:id/stop`

```bash
curl -X POST http://127.0.0.1:8080/api/exits/8/stop \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/exits/:id/restart`

```bash
curl -X POST http://127.0.0.1:8080/api/exits/8/restart \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/exits/:id/test`

Get live container status and current public IP.

```bash
curl -X POST http://127.0.0.1:8080/api/exits/8/test \
  -H "Authorization: Bearer $TOKEN"
```

Response:

```json
{
  "status": "running",
  "public_ip": "104.255.173.142"
}
```

### `GET /api/exits/:id/status`

Get status and public IP, then persist them to the database.

```bash
curl http://127.0.0.1:8080/api/exits/8/status \
  -H "Authorization: Bearer $TOKEN"
```

### `GET /api/exits/:id/logs`

Get Gluetun container logs. Default `tail` is `100`.

```bash
curl "http://127.0.0.1:8080/api/exits/8/logs?tail=200" \
  -H "Authorization: Bearer $TOKEN"
```

Response:

```json
{
  "logs": "..."
}
```

### `GET /api/exits/:id/config`

Get exit config as compose-style YAML.

```bash
curl http://127.0.0.1:8080/api/exits/8/config \
  -H "Authorization: Bearer $TOKEN"
```

Response:

```json
{
  "config_yaml": "enabled: true\nservices:\n  gluetun:\n    image: qmcgaw/gluetun:latest\n    environment:\n      - VPN_SERVICE_PROVIDER=surfshark\n      - VPN_TYPE=wireguard\n      - SERVER_COUNTRIES=United States\n      - SERVER_CITIES=\n      - WIREGUARD_PRIVATE_KEY=...\n"
}
```

---

## Rules

Rules map a domain pattern to an exit. Higher-priority rules are evaluated first.

### Rule Object

```json
{
  "id": 1,
  "pattern": "example.com",
  "type": "suffix",
  "exit_id": 8,
  "exit": {
    "id": 8,
    "name": "us"
  },
  "priority": 100,
  "source": "manual",
  "enabled": true,
  "created_at": "2026-04-15T00:22:11Z",
  "updated_at": "2026-04-15T00:22:11Z"
}
```

`source` is `manual` for user-created rules and `subscription` for rules generated by a subscription sync.

### `GET /api/rules`

List manual rules by default. Subscription-generated rules remain active for publishing/routing, but are hidden from the normal rules list unless explicitly requested.

Query parameters:

- `source` — filter by `manual` or `subscription`; use `all` to include both.
- `exit_id` — filter by exit ID.

```bash
curl http://127.0.0.1:8080/api/rules \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/rules`

Create a manual rule.

```bash
curl -X POST http://127.0.0.1:8080/api/rules \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "pattern": "example.com",
    "type": "suffix",
    "exit_id": 8,
    "priority": 100,
    "enabled": true
  }'
```

Fields:

| Field | Type | Notes |
| --- | --- | --- |
| `pattern` | string | Required. Hostname or domain. |
| `type` | `auto` \| `exact` \| `suffix` | Optional. Defaults to `auto`; `+.` patterns become suffix matches, otherwise exact. `suffix` matches the domain and all subdomains. |
| `exit_id` | integer \| null | Must reference an existing exit when non-null. |
| `priority` | integer | Default `100` when omitted or `0`. Higher = evaluated first. |
| `enabled` | bool | Default `true`. |

`source` is always `manual` for this endpoint.

### `POST /api/rules/bulk`

Create many manual rules in one request.

```bash
curl -X POST http://127.0.0.1:8080/api/rules/bulk \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "rules": [
      {
        "pattern": "example.com",
        "type": "suffix",
        "exit_id": 8,
        "priority": 100,
        "enabled": true
      },
      {
        "pattern": "+.example.org",
        "type": "exact",
        "exit_id": 8,
        "priority": 100,
        "enabled": true
      }
    ]
  }'
```

The bulk endpoint accepts up to `5000` rules per request and de-duplicates identical `type` + `pattern` pairs within the request after type detection. A pattern beginning with `+.` is normalized to a suffix rule, so `+.example.org` is stored as `example.org` with `type: "suffix"`.

### `PUT /api/rules/:id`

Update a rule. Send `"exit_id": null` to clear the exit binding.

```bash
curl -X PUT http://127.0.0.1:8080/api/rules/1 \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "pattern": "example.com",
    "type": "suffix",
    "exit_id": null,
    "priority": 200,
    "enabled": false
  }'
```

### `DELETE /api/rules/:id`

```bash
curl -X DELETE http://127.0.0.1:8080/api/rules/1 \
  -H "Authorization: Bearer $TOKEN"
```

Response:

```json
{
  "message": "deleted"
}
```

---

## Subscriptions

Subscriptions download remote domain-list files on a schedule and convert each entry into a routing rule.

### Subscription List Format

The parser accepts these line formats:

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

Subscription rules get priority `50`. Manual rules default to `100` and are always evaluated first.

### Subscription Object

```json
{
  "id": 1,
  "name": "AI Sites",
  "url": "https://example.com/ai.list",
  "exit_id": 8,
  "exit": {
    "id": 8,
    "name": "us"
  },
  "enabled": true,
  "interval_minutes": 360,
  "last_sync_at": null,
  "last_sync_status": "",
  "last_error": "",
  "rule_count": 0,
  "created_at": "2026-04-15T00:30:00Z",
  "updated_at": "2026-04-15T00:30:00Z"
}
```

### `GET /api/subscriptions`

```bash
curl http://127.0.0.1:8080/api/subscriptions \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/subscriptions`

Create a subscription. If enabled, the backend syncs it immediately and schedules future syncs.

```bash
curl -X POST http://127.0.0.1:8080/api/subscriptions \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "AI Sites",
    "url": "https://example.com/ai.list",
    "exit_id": 8,
    "enabled": true,
    "interval_minutes": 360
  }'
```

Fields:

| Field | Type | Notes |
| --- | --- | --- |
| `name` | string | Required. |
| `url` | string | Required. URL of the domain list file. |
| `exit_id` | integer \| null | Optional. Exit to route matched domains through. |
| `enabled` | bool | Default `true`. |
| `interval_minutes` | integer | Default `360` when omitted or `0`. |

### `PUT /api/subscriptions/:id`

Update a subscription. Send `"exit_id": null` to clear the exit binding.

```bash
curl -X PUT http://127.0.0.1:8080/api/subscriptions/1 \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "enabled": true,
    "exit_id": 8,
    "interval_minutes": 120
  }'
```

Disabling a subscription deletes its generated rules and automatically reloads the generated nginx config.

### `DELETE /api/subscriptions/:id`

Delete a subscription and all its generated rules.

```bash
curl -X DELETE http://127.0.0.1:8080/api/subscriptions/1 \
  -H "Authorization: Bearer $TOKEN"
```

### `POST /api/subscriptions/:id/sync`

Trigger an immediate sync.

```bash
curl -X POST http://127.0.0.1:8080/api/subscriptions/1/sync \
  -H "Authorization: Bearer $TOKEN"
```

Response:

```json
{
  "message": "sync completed"
}
```

---

## Publish

Rules and subscriptions are stored in the database. They become active in nginx only after publish apply.

### `POST /api/publish/preview`

Preview the generated nginx config without writing it.

```bash
curl -X POST http://127.0.0.1:8080/api/publish/preview \
  -H "Authorization: Bearer $TOKEN"
```

Response:

```json
{
  "config": "# --- stream context ---\n..."
}
```

### `POST /api/publish/validate`

Validate the generated config using `nginx -t`.

```bash
curl -X POST http://127.0.0.1:8080/api/publish/validate \
  -H "Authorization: Bearer $TOKEN"
```

Success:

```json
{
  "valid": true
}
```

Failure:

```json
{
  "valid": false,
  "error": "message"
}
```

### `POST /api/publish/apply`

Write the generated config to the nginx shared volume and reload nginx.

```bash
curl -X POST http://127.0.0.1:8080/api/publish/apply \
  -H "Authorization: Bearer $TOKEN"
```

Response:

```json
{
  "message": "config applied and nginx reloaded"
}
```

### `POST /api/publish/rollback`

Restore the previous generated config.

```bash
curl -X POST http://127.0.0.1:8080/api/publish/rollback \
  -H "Authorization: Bearer $TOKEN"
```

Response:

```json
{
  "message": "rolled back to previous config"
}
```

### `GET /api/publish/history`

Get the 50 most recent publish records.

```bash
curl http://127.0.0.1:8080/api/publish/history \
  -H "Authorization: Bearer $TOKEN"
```

Response:

```json
[
  {
    "id": 1,
    "version": "v1776216000",
    "status": "success",
    "message": "config applied successfully",
    "created_at": "2026-04-15T00:28:36Z"
  }
]
```

---

## End-to-End Example

```bash
# 1. Authenticate
TOKEN="$(
  curl -s -X POST http://127.0.0.1:8080/api/auth/login \
    -H 'Content-Type: application/json' \
    -d '{"username":"admin","password":"admin"}' |
  sed -n 's/.*"token":"\([^"]*\)".*/\1/p'
)"

# 2. Create a WireGuard exit
curl -X POST http://127.0.0.1:8080/api/exits \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "us",
    "enabled": true,
    "config_yaml": "enabled: true\nservices:\n  gluetun:\n    image: qmcgaw/gluetun:latest\n    environment:\n      - VPN_SERVICE_PROVIDER=surfshark\n      - VPN_TYPE=wireguard\n      - SERVER_COUNTRIES=United States\n      - WIREGUARD_PRIVATE_KEY=YOUR_PRIVATE_KEY\n"
  }'

# 3. Add a routing rule (exit_id from step 2)
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

# 4. Apply generated nginx config
curl -X POST http://127.0.0.1:8080/api/publish/apply \
  -H "Authorization: Bearer $TOKEN"
```

Test:

```bash
curl -I --resolve example.com:80:YOUR_SERVER_IP http://example.com/
curl -vk --resolve example.com:443:YOUR_SERVER_IP https://example.com/
openssl s_client -connect YOUR_SERVER_IP:443 -servername example.com
```

---

## Runtime Notes

- Rules and subscription syncs do not automatically apply nginx config. Always call `POST /api/publish/apply` after routing changes.
- `GET /api/exits/:id/config` returns compose-style YAML; extra `services.gluetun.environment` keys are preserved for editing and runtime creation.
- Enabled exits restart automatically every 12 hours by default. Set `EXIT_RESTART_CRON=disabled` to disable, or set another cron expression such as `@every 6h`.
- Matched port `80` traffic redirects to HTTPS. Matched port `443` traffic is forwarded by SNI through the selected relay/VPN exit.
- The subscription list file is fetched with a 10 MB limit.
- Subscription rules have priority `50`; manual rules default to `100`.
