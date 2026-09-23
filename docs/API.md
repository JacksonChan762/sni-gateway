# API reference — sing-box provider and custom exits

The admin API listens on `http://127.0.0.1:8080` by default. `GET /health` and `POST /api/auth/login` are public. All other `/api/*` endpoints require `Authorization: Bearer <token>`.

```bash
curl -sS -X POST http://127.0.0.1:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"YOUR_ADMIN_PASSWORD"}'
```

Use the returned `token` in the requests below. The installer stores the generated admin password in `.env`. Never commit real credentials or VPN configuration files.

## Provider server catalog

Authenticated `GET /api/providers` lists providers in the cached [Gluetun servers catalog](https://github.com/qdm12/gluetun-servers), protocol counts, countries, and source commit. The catalog checks upstream at startup and daily by default, retaining the last working copy on failed updates. `GET /api/providers/locations` accepts `provider`, `protocol` (`openvpn` or `wireguard`), and optional `country`; it returns country and city counts. The UI selects provider, country, and optional city. sing-box selects a compatible server from this catalog when the exit starts. `GET /api/providers/servers` remains available with `provider`, `protocol`, optional `country` and `query`, and `offset`/`limit` (at most 100). The response contains `total`, `servers`, and `source_commit`. Client credentials, WireGuard interface keys/addresses, and OpenVPN certificates are not part of the catalog.

## Exits

`POST /api/exits` uses `runtime: "sing-box"` for both built-in provider and complete custom connections. It is the default when omitted. `protocol` is `wireguard` or `openvpn`.

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

For built-in OpenVPN providers, submit `provider`, `country`, optional `city`, `openvpn_user`, and `openvpn_password`. AirVPN and VPN Secure do not require username/password; IVPN account IDs can omit the password. AirVPN, CyberGhost, VPN Secure, and VPN Unlimited also require `openvpn_cert` and `openvpn_key` (PEM text or base64 body). VPN Secure requires `openvpn_key_passphrase` for its encrypted key. The application supplies the provider’s public CA, TLS settings, server and port to sing-box. For built-in WireGuard, submit `wireguard_private_key`; most providers also require `wireguard_addresses` in CIDR format, while Proton VPN and NordVPN use their documented defaults. AirVPN also requires `wireguard_preshared_key`. `auto_server: true` selects provider mode; it is inferred for a provider if no manual endpoint or profile is supplied. The API validates required fields before creating a container. For example:

```json
{
  "name": "proton-nl",
  "runtime": "sing-box",
  "protocol": "openvpn",
  "provider": "protonvpn",
  "country": "Netherlands",
  "openvpn_user": "YOUR_OPENVPN_SERVICE_USERNAME",
  "openvpn_password": "YOUR_OPENVPN_SERVICE_PASSWORD",
  "enabled": true
}
```

Custom WireGuard uses a private key, peer public key, endpoint IP/port, and tunnel address. The peer must allow `0.0.0.0/0`. `wireguard_preshared_key` is optional. Custom OpenVPN accepts a complete portable `.ovpn` profile in `openvpn_profile`; inline CA and any client certificate/key are required. External file references and unsupported directives return validation errors. Custom examples:

```json
{
  "name": "office-vpn",
  "runtime": "sing-box",
  "protocol": "openvpn",
  "provider": "custom",
  "openvpn_profile": "client\nproto udp\nremote vpn.example.com 1194\nremote-cert-tls server\n<ca>\n-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n</ca>\n",
  "openvpn_user": "YOUR_USERNAME",
  "openvpn_password": "YOUR_PASSWORD",
  "enabled": true
}
```

The `config_yaml` import/export format is also accepted. WireGuard uses a `services.singbox.environment` list of `WIREGUARD_*` variables. OpenVPN export uses top-level profile or provider credentials, `runtime`, and location fields. This is an application format, not a standalone Compose deployment. The config endpoint returns secrets and should be handled accordingly.

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
| `GET /api/exits/:id/logs` | Return sing-box logs; legacy Gluetun exits retain their old logs until converted |
| `GET /api/exits/:id/config` | Return configuration including secrets; handle carefully |

Starting an exit probes its public IP through the sing-box SOCKS listener. A successful probe records `status: "running"`, `public_ip`, and the chosen `endpoint_ip`; a failed probe preserves the exit as `status: "unhealthy"` so its logs can be inspected and the connection retried. `POST /api/exits/:id/start` and `/restart` return the resulting status and public IP. For automatic server selection, provide a country, a city, or both. When a city is set, server selection ignores the country; the country only filters cities in the picker. Every connection tests up to four distinct catalog endpoints through the VPN and chooses the one with the shortest successful connect-and-public-IP probe. The previously chosen endpoint remains eligible and can win again. The catalog holds VPN endpoint IPs from Gluetun servers data and checks upstream daily; `public_ip` is measured through the connected VPN.

The trial time covers container startup through the successful public-IP probe. Each candidate has a 40-second probe deadline, with a five-minute overall connection deadline. If more than four endpoints match, four are sampled; the chosen endpoint is the fastest measured sample, not necessarily the fastest in the catalog. If the first choice fails on final connection, another successful sample is tried. `GET /api/exits/:id/status` and `POST /api/exits/:id/test` only probe the current endpoint; `/restart` reruns endpoint selection. Existing running exits keep their endpoint until restarted.

Older Gluetun and relay-based exits remain in the database during migration. Edit one with valid provider credentials or a complete custom configuration to convert it to sing-box. The API rejects new Gluetun exits and attempts to start an unconverted relay exit.

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
