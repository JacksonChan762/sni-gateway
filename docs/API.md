# API Reference

Base URL:

```txt
http://127.0.0.1:8080
```

All API routes are under `/api`.

This project currently does not require authentication.

## Conventions

- Content type: `application/json`
- Error format:

```json
{
  "error": "message"
}
```

- Success responses usually return a JSON object or model record

## Health

### `GET /health`

Check backend health.

```bash
curl http://127.0.0.1:8080/health
```

Response:

```txt
ok
```

## Stats

### `GET /api/stats`

Get dashboard summary counts.

```bash
curl http://127.0.0.1:8080/api/stats
```

Response example:

```json
{
  "rule_count": 1,
  "sub_count": 0,
  "exit_count": 1,
  "running_exit_count": 1,
  "last_publish": {
    "id": 1,
    "version": "v1712345678",
    "status": "success",
    "message": "config applied successfully",
    "created_at": "2026-04-15T00:28:36Z"
  }
}
```

## Exits

An exit is a VPN runtime backed by `gluetun` and `relay`.

### Exit object

Response example:

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

### `GET /api/exits`

List exits.

```bash
curl http://127.0.0.1:8080/api/exits
```

### `POST /api/exits`

Create an exit.

Minimal WireGuard example:

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

Minimal OpenVPN example:

```bash
curl -X POST http://127.0.0.1:8080/api/exits \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "jp",
    "protocol": "openvpn",
    "provider": "protonvpn",
    "country": "Japan",
    "config_yaml": "services:\n  gluetun:\n    environment:\n      - VPN_SERVICE_PROVIDER=protonvpn\n      - VPN_TYPE=openvpn\n      - SERVER_COUNTRIES=Japan\n      - SERVER_CITIES=\n      - OPENVPN_USER=YOUR_USER\n      - OPENVPN_PASSWORD=YOUR_PASSWORD\n",
    "enabled": true
  }'
```

Accepted request fields:

```json
{
  "name": "string, required",
  "runtime": "string, optional, default gluetun",
  "protocol": "wireguard|openvpn",
  "provider": "string, optional, default protonvpn",
  "country": "string, optional",
  "city": "string, optional",
  "wireguard_private_key": "string, optional if config_yaml supplied",
  "wireguard_addresses": "string, optional",
  "openvpn_user": "string, optional if config_yaml supplied",
  "openvpn_password": "string, optional if config_yaml supplied",
  "config_yaml": "string, optional",
  "enabled": true
}
```

Notes:

- `name` is separate from `config_yaml`
- `config_yaml` is parsed from `services.gluetun.environment`
- Environment values can be given as `KEY=VALUE` entries
- For WireGuard, `WIREGUARD_PRIVATE_KEY` is required
- For OpenVPN, `OPENVPN_USER` and `OPENVPN_PASSWORD` are required
- `WIREGUARD_ADDRESSES` is optional
- At least one of `SERVER_COUNTRIES` or `SERVER_CITIES` should be set in practice

### `PUT /api/exits/:id`

Update an exit.

Example:

```bash
curl -X PUT http://127.0.0.1:8080/api/exits/8 \
  -H 'Content-Type: application/json' \
  -d '{
    "provider": "surfshark",
    "country": "United States",
    "enabled": true,
    "config_yaml": "services:\n  gluetun:\n    environment:\n      - VPN_SERVICE_PROVIDER=surfshark\n      - VPN_TYPE=wireguard\n      - SERVER_COUNTRIES=United States\n      - SERVER_CITIES=\n      - WIREGUARD_PRIVATE_KEY=YOUR_PRIVATE_KEY\n"
  }'
```

Notes:

- Exit `name` is not updated here
- Updating config only changes database config
- To recreate runtime with the new config, restart the exit

### `DELETE /api/exits/:id`

Delete an exit.

```bash
curl -X DELETE http://127.0.0.1:8080/api/exits/8
```

Response:

```json
{
  "message": "deleted"
}
```

If active rules still point to this exit:

```json
{
  "error": "cannot delete exit with active rules",
  "rule_count": 1
}
```

### `POST /api/exits/:id/start`

Start the exit.

```bash
curl -X POST http://127.0.0.1:8080/api/exits/8/start
```

### `POST /api/exits/:id/stop`

Stop the exit.

```bash
curl -X POST http://127.0.0.1:8080/api/exits/8/stop
```

### `POST /api/exits/:id/restart`

Restart the exit.

```bash
curl -X POST http://127.0.0.1:8080/api/exits/8/restart
```

### `POST /api/exits/:id/test`

Get live exit status and current public IP.

```bash
curl -X POST http://127.0.0.1:8080/api/exits/8/test
```

Response:

```json
{
  "status": "running",
  "public_ip": "104.255.173.142"
}
```

### `GET /api/exits/:id/status`

Get status and public IP and persist them back to DB.

```bash
curl http://127.0.0.1:8080/api/exits/8/status
```

### `GET /api/exits/:id/logs`

Get exit logs.

Default tail is `100`.

```bash
curl "http://127.0.0.1:8080/api/exits/8/logs?tail=200"
```

Response:

```json
{
  "logs": "..."
}
```

### `GET /api/exits/:id/config`

Get exit config as YAML.

```bash
curl http://127.0.0.1:8080/api/exits/8/config
```

Response:

```json
{
  "config_yaml": "enabled: true\nservices:\n  gluetun:\n    image: qmcgaw/gluetun:latest\n    environment:\n      - VPN_SERVICE_PROVIDER=surfshark\n      - VPN_TYPE=wireguard\n      - SERVER_COUNTRIES=United States\n      - SERVER_CITIES=\n      - WIREGUARD_PRIVATE_KEY=...\n"
}
```

## Rules

Rules map a domain pattern to an exit.

### Rule object

```json
{
  "id": 1,
  "pattern": "ping0.cc",
  "type": "suffix",
  "exit_id": 8,
  "priority": 100,
  "source": "manual",
  "enabled": true,
  "created_at": "2026-04-15T00:22:11Z",
  "updated_at": "2026-04-15T00:22:11Z"
}
```

### `GET /api/rules`

List rules.

Query parameters:

- `source`
- `exit_id`

Examples:

```bash
curl http://127.0.0.1:8080/api/rules
curl "http://127.0.0.1:8080/api/rules?source=manual"
curl "http://127.0.0.1:8080/api/rules?exit_id=8"
```

### `POST /api/rules`

Create a rule.

```bash
curl -X POST http://127.0.0.1:8080/api/rules \
  -H 'Content-Type: application/json' \
  -d '{
    "pattern": "ping0.cc",
    "type": "suffix",
    "exit_id": 8,
    "priority": 100,
    "enabled": true
  }'
```

Request fields:

```json
{
  "pattern": "string, required",
  "type": "exact|suffix, required",
  "exit_id": 8,
  "priority": 100,
  "enabled": true
}
```

### `PUT /api/rules/:id`

Update a rule.

```bash
curl -X PUT http://127.0.0.1:8080/api/rules/1 \
  -H 'Content-Type: application/json' \
  -d '{
    "pattern": "ping0.cc",
    "type": "suffix",
    "exit_id": 8,
    "priority": 200,
    "enabled": true
  }'
```

### `DELETE /api/rules/:id`

Delete a rule.

```bash
curl -X DELETE http://127.0.0.1:8080/api/rules/1
```

Response:

```json
{
  "message": "deleted"
}
```

## Subscriptions

Subscriptions download remote `.list` files and convert them into rules.

### Subscription object

```json
{
  "id": 1,
  "name": "AI Sites",
  "url": "https://example.com/ai.list",
  "exit_id": 8,
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

List subscriptions.

```bash
curl http://127.0.0.1:8080/api/subscriptions
```

### `POST /api/subscriptions`

Create a subscription.

```bash
curl -X POST http://127.0.0.1:8080/api/subscriptions \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "AI Sites",
    "url": "https://example.com/ai.list",
    "exit_id": 8,
    "enabled": true,
    "interval_minutes": 360
  }'
```

Request fields:

```json
{
  "name": "string, required",
  "url": "string, required",
  "exit_id": 8,
  "enabled": true,
  "interval_minutes": 360
}
```

### `PUT /api/subscriptions/:id`

Update a subscription.

```bash
curl -X PUT http://127.0.0.1:8080/api/subscriptions/1 \
  -H 'Content-Type: application/json' \
  -d '{
    "enabled": true,
    "interval_minutes": 120
  }'
```

### `DELETE /api/subscriptions/:id`

Delete a subscription and its generated rules.

```bash
curl -X DELETE http://127.0.0.1:8080/api/subscriptions/1
```

### `POST /api/subscriptions/:id/sync`

Trigger immediate sync.

```bash
curl -X POST http://127.0.0.1:8080/api/subscriptions/1/sync
```

Response:

```json
{
  "message": "sync completed"
}
```

## Publish

Rules are stored in DB first. They become active in nginx only after publish apply.

### `POST /api/publish/preview`

Preview generated config.

```bash
curl -X POST http://127.0.0.1:8080/api/publish/preview
```

Response:

```json
{
  "config": "# --- stream context ---\n..."
}
```

### `POST /api/publish/validate`

Validate generated config.

```bash
curl -X POST http://127.0.0.1:8080/api/publish/validate
```

Response:

```json
{
  "valid": true
}
```

Failure example:

```json
{
  "valid": false,
  "error": "message"
}
```

### `POST /api/publish/apply`

Write generated config to nginx shared volume and reload nginx.

```bash
curl -X POST http://127.0.0.1:8080/api/publish/apply
```

Response:

```json
{
  "message": "config applied and nginx reloaded"
}
```

### `POST /api/publish/rollback`

Restore previous generated config.

```bash
curl -X POST http://127.0.0.1:8080/api/publish/rollback
```

Response:

```json
{
  "message": "rolled back to previous config"
}
```

### `GET /api/publish/history`

Get publish history.

```bash
curl http://127.0.0.1:8080/api/publish/history
```

Response example:

```json
[
  {
    "id": 1,
    "version": "v1712345678",
    "status": "success",
    "message": "config applied successfully",
    "created_at": "2026-04-15T00:28:36Z"
  }
]
```

## End-to-End Example

Create exit:

```bash
curl -X POST http://127.0.0.1:8080/api/exits \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "us",
    "protocol": "wireguard",
    "provider": "surfshark",
    "country": "United States",
    "config_yaml": "services:\n  gluetun:\n    environment:\n      - VPN_SERVICE_PROVIDER=surfshark\n      - VPN_TYPE=wireguard\n      - SERVER_COUNTRIES=United States\n      - SERVER_CITIES=\n      - WIREGUARD_PRIVATE_KEY=YOUR_PRIVATE_KEY\n",
    "enabled": true
  }'
```

Create rule:

```bash
curl -X POST http://127.0.0.1:8080/api/rules \
  -H 'Content-Type: application/json' \
  -d '{
    "pattern": "ping0.cc",
    "type": "suffix",
    "exit_id": 8,
    "priority": 100,
    "enabled": true
  }'
```

Apply:

```bash
curl -X POST http://127.0.0.1:8080/api/publish/apply
```

Test:

```bash
curl -I --resolve ping0.cc:80:YOUR_SERVER_IP http://ping0.cc/
curl -vk --resolve ping0.cc:443:YOUR_SERVER_IP https://ping0.cc/
openssl s_client -connect YOUR_SERVER_IP:443 -servername ping0.cc
```

## Current Runtime Notes

- Rules do not auto-apply to nginx
- Use `POST /api/publish/apply` after rule or subscription changes
- `GET /api/exits/:id/config` returns compose-style YAML
- The UI may edit only the `environment` section conceptually, but the API stores normalized exit config in `config_json`
- For matched domains, port `80` redirects to HTTPS
