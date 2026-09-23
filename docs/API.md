# API reference — sing-box migration

The admin API listens on `http://127.0.0.1:8080` by default. `GET /health` and `POST /api/auth/login` are public. All other `/api/*` endpoints require `Authorization: Bearer <token>`.

```bash
curl -sS -X POST http://127.0.0.1:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"YOUR_ADMIN_PASSWORD"}'
```

Use the returned `token` in the requests below. The installer stores the generated admin password in `.env`. Never commit real credentials or VPN configuration files.

## Provider server catalog

Authenticated `GET /api/providers` lists providers in the cached [Gluetun servers catalog](https://github.com/qdm12/gluetun-servers), protocol counts, countries, and source commit. The catalog checks upstream at startup and daily by default, retaining the last working copy on failed updates. `GET /api/providers/locations` accepts `provider`, `protocol` (`openvpn` or `wireguard`), and optional `country`; it returns country and city counts. The UI selects provider, country, and optional city; the backend selects a compatible server automatically when building the connection. `GET /api/providers/servers` remains available with `provider`, `protocol`, optional `country` and `query`, and `offset`/`limit` (at most 100). The response contains `total`, `servers`, and `source_commit`. Client credentials, WireGuard interface keys/addresses, and OpenVPN certificates are not part of the catalog.

## Exits

`POST /api/exits` accepts a complete WireGuard or OpenVPN connection. `runtime` defaults to `sing-box`; `protocol` is `wireguard` or `openvpn`. For WireGuard, the private key and peer public key must be base64-encoded 32-byte keys, and the endpoint must be an IP address.

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

`wireguard_preshared_key` is optional. The peer must allow `0.0.0.0/0` because the gateway forwards arbitrary matched domains; there is no direct fallback. DNS queries for destination domains use the selected VPN exit. Set `auto_server: true` with a catalog `provider` and `country` (optional `city`) to have the backend choose a WireGuard endpoint and peer public key. The client private key, address, and endpoint port are still required. Without `auto_server`, supply the endpoint IP and peer public key as before. OpenVPN credentials alone are insufficient; a complete `.ovpn` profile is required.

For OpenVPN, submit the `.ovpn` text in `openvpn_profile`. With `auto_server: true`, `provider`, `country`, and optional `city`, the backend substitutes a compatible catalog server IP for the profile remote host while retaining the profile port and security settings. A server-specific certificate name or provider-specific port may prevent that profile from working with another catalog server. The profile needs a `remote` server and inline `<ca>` certificate. Include inline `<cert>` and `<key>` together when client certificate authentication is required. Set `openvpn_user` and `openvpn_password` together when the provider requires username/password authentication. External file references and unsupported directives return validation errors. Example:

```json
{
  "name": "office-vpn",
  "protocol": "openvpn",
  "openvpn_profile": "client\nproto udp\nremote vpn.example.com 1194\nremote-cert-tls server\n<ca>\n-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n</ca>\n",
  "openvpn_user": "YOUR_USERNAME",
  "openvpn_password": "YOUR_PASSWORD",
  "enabled": true
}
```

The `config_yaml` import/export format is also accepted. WireGuard uses a `services.singbox.environment` list of `WIREGUARD_*` variables. OpenVPN export uses top-level `openvpn_profile`, `openvpn_user`, `openvpn_password`, `auto_server`, and location fields. This is an application format, not a standalone Compose deployment. The config endpoint returns secrets and should be handled accordingly.

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

Starting an exit probes its public IP through the sing-box SOCKS listener. A successful probe records `status: "running"` and `public_ip`; a failed probe preserves the exit as `status: "unhealthy"` so its logs can be inspected and the connection retried. `POST /api/exits/:id/start` and `/restart` return the resulting status and public IP.

Older Gluetun exits remain in the database during migration. Edit one with a complete WireGuard or OpenVPN configuration to convert it to sing-box. The API rejects attempts to start a legacy exit before conversion.

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
