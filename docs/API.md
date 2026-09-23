# API reference — sing-box migration

The admin API listens on `http://127.0.0.1:8080` by default. `GET /health` and `POST /api/auth/login` are public. All other `/api/*` endpoints require `Authorization: Bearer <token>`.

```bash
curl -sS -X POST http://127.0.0.1:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"YOUR_ADMIN_PASSWORD"}'
```

Use the returned `token` in the requests below. The installer stores the generated admin password in `.env`. Never commit real credentials or WireGuard configuration files.

## Exits

`POST /api/exits` accepts a complete WireGuard connection. `runtime` defaults to `sing-box`; `protocol` must be `wireguard`. The private key and peer public key must be base64-encoded 32-byte keys, and the endpoint must be an IP address.

```json
{
  "name": "us-west",
  "runtime": "sing-box",
  "protocol": "wireguard",
  "provider": "custom",
  "wireguard_endpoint_ip": "203.0.113.10",
  "wireguard_endpoint_port": "51820",
  "wireguard_public_key": "BASE64_PEER_PUBLIC_KEY",
  "wireguard_private_key": "BASE64_CLIENT_PRIVATE_KEY",
  "wireguard_addresses": "10.2.0.2/32",
  "wireguard_allowed_ips": "0.0.0.0/0",
  "wireguard_dns": "9.9.9.9",
  "enabled": true
}
```

`wireguard_preshared_key` is optional. The peer must allow `0.0.0.0/0` because the gateway forwards arbitrary matched domains; there is no direct fallback. DNS queries for destination domains use the selected VPN exit. A provider name, country, private key, and address without the peer endpoint/public key are insufficient. OpenVPN credentials alone cannot be used in this runtime.

A `config_yaml` import/export format is also accepted. It contains a `services.singbox.environment` list of `WIREGUARD_*` variables, as used by the UI. This is an application format, not a standalone Compose deployment.

| Endpoint | Purpose |
| --- | --- |
| `GET /api/exits` | List exits without secrets |
| `POST /api/exits` | Create an exit and start it if enabled |
| `PUT /api/exits/:id` | Update and reconcile an exit |
| `DELETE /api/exits/:id` | Delete an unused exit |
| `POST /api/exits/:id/start` | Start an enabled exit |
| `POST /api/exits/:id/stop` | Stop an exit |
| `POST /api/exits/:id/restart` | Recreate an enabled exit |
| `POST /api/exits/stop-all` | Stop every exit |
| `GET /api/exits/:id/status` | Check the container and VPN public IP |
| `POST /api/exits/:id/test` | Perform the same connection check |
| `GET /api/exits/:id/logs` | Return sing-box logs |
| `GET /api/exits/:id/config` | Return configuration including secrets; handle carefully |

Older Gluetun exits remain in the database during migration. Edit one with a complete WireGuard configuration to convert it to sing-box. The API rejects attempts to start a legacy exit before conversion.

## Rules and subscriptions

Add a rule with `POST /api/rules`:

```json
{"pattern":"example.com","type":"suffix","exit_id":1,"priority":100,"enabled":true}
```

`type` is `exact` or `suffix`. Higher priority wins, with lower rule ID breaking ties. Other endpoints are `GET /api/rules`, `PUT /api/rules/:id`, `DELETE /api/rules/:id`, `POST /api/rules/bulk`, and `POST /api/rules/test`. Rules can be saved without becoming active until publication.

Subscriptions use `GET/POST /api/subscriptions`, `PUT/DELETE /api/subscriptions/:id`, and `POST /api/subscriptions/:id/sync`. A subscription includes `name`, `url`, `exit_id`, `enabled`, and `interval_minutes`. A successful sync updates generated rules and publishes a new routing snapshot.

## Publish

| Endpoint | Purpose |
| --- | --- |
| `GET /api/publish/status` | Check whether saved rules differ from active routes |
| `POST /api/publish/preview` | Preview the route snapshot as JSON |
| `POST /api/publish/validate` | Validate rules and referenced exits without applying |
| `POST /api/publish/apply` | Persist and activate the snapshot for new connections |
| `POST /api/publish/rollback` | Activate the previous snapshot |
| `GET /api/publish/history` | List recent publication events |

The published snapshot is separate from editable database rules and survives backend restarts. Existing TCP connections keep their selected exit; new connections use the latest published snapshot. `GET /api/stats` returns dashboard counters.
