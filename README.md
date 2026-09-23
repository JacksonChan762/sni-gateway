# SNI Gateway — sing-box provider and custom VPN exits

[繁體中文](README.zh-TW.md)

This branch uses a Go SNI ingress. sing-box connects built-in provider and custom VPN exits directly. The admin UI and API remain on `127.0.0.1:8080` by default. Ports 80 and 443 are served by the backend. Port 80 redirects matched hosts to HTTPS; port 443 passes the original TLS connection through the selected VPN without terminating TLS. Unmatched SNI names are closed.

This is a migration branch, not the v1.1 stable release. The existing [v1.1 release notes](docs/releases/v1.1.md) describe the stable Nginx/Gluetun deployment.

## Requirements

- Docker and Docker Compose v2
- Free host ports 80 and 443
- Provider exits need the credentials required by that provider: OpenVPN service username/password and, for some providers, client certificate/key; or WireGuard private key and, where required, tunnel address and preshared key. Custom exits need a complete WireGuard configuration or portable OpenVPN `.ovpn` profile.

## Install

```bash
./sni-gateway.sh install
```

The script creates `.env` with random admin credentials if needed. Open `http://127.0.0.1:8080` and read `ADMIN_PASSWORD` from `.env`. Keep that file private. The installer builds the backend and UI and pulls the official sing-box image. It removes the old Nginx Compose service and stops legacy relay containers after the new stack is healthy. PostgreSQL data is retained.

For an existing checkout, back up the database before switching branches. The updater described in [application updates](docs/UPDATES.md) works after this branch is installed. When moving from an older Nginx/Gluetun revision, stop the old Compose stack, switch to this branch, then run `./sni-gateway.sh install`; this avoids port 80/443 conflicts during the first migration.

## Use

1. Open **Exits** and select a built-in provider, a country or city, and a protocol. Country narrows the city picker; when a city is selected, only the city filters VPN endpoints. For OpenVPN, enter provider service credentials; providers that use client certificates ask for those too. For WireGuard, enter the private key and any additional fields shown for that provider. To use your own VPN server, select **Custom configuration** and import a WireGuard `.conf` or complete OpenVPN `.ovpn` profile. The gateway checks the public IP through the VPN on startup. **Running** means the probe succeeded; **Unhealthy** means it did not. Use **Logs** and **Check status** to investigate.
2. Add exact or suffix domain rules and select an exit. A rule targeting an unconverted legacy exit cannot be published until that exit is edited.
3. Open **Publish**, validate and publish the saved rules. New connections use the published table immediately. Rollback restores the previous published table. Subscription syncs can also publish updates.
4. Point the domain to the gateway host and test a TLS connection. DNS resolution for the destination occurs through the selected VPN. sing-box routes the exit's SOCKS traffic only through its VPN endpoint.

The WireGuard environment YAML and OpenVPN YAML export are application import/export formats, not standalone Docker Compose files. Each exit has a sing-box SOCKS listener on port 1080 inside the private `vpn-net` Docker network. The SOCKS listener is not published on the host.

See the [API reference](docs/API.md) for endpoint details and WireGuard/OpenVPN request examples. Custom OpenVPN server hostnames use local DNS to bootstrap the tunnel; destination lookups use VPN DNS.

## Operations

```bash
./sni-gateway.sh update --check
./sni-gateway.sh update --apply
./scripts/stop.sh
cd backend && go test ./...
cd frontend && npm run build
```

`update --apply` backs up PostgreSQL, fast-forwards the tracked branch, rebuilds images, and waits for Compose health. It does not automatically restart already-running VPN exits; restart exits in the UI to use a newer runtime image. The backend requires access to the Docker socket to manage exits. Backups and `.env` contain secrets and should be kept private.

The UI and sing-box provider selector use a pinned snapshot of [Gluetun servers data](https://github.com/qdm12/gluetun-servers) and check upstream daily. Country filters the city picker; a selected city overrides country, while Any city uses country. Every WireGuard and OpenVPN connection measures up to four candidate endpoints and uses the fastest successful one. Catalog IPs are VPN endpoints; the displayed public IP is measured after connection. Public OpenVPN provider CA and TLS templates are bundled from Gluetun's provider definitions; new exits use no Gluetun container. To refresh the bundled server fallback snapshot for releases, run `python3 scripts/update_provider_catalog.py /path/to/gluetun-servers`. Refresh the OpenVPN templates with `python3 scripts/update_openvpn_templates.py /path/to/gluetun` when provider definitions change, then run the sing-box configuration checks. License notices are in `backend/internal/catalog/data/LICENSE.gluetun-servers` and `backend/internal/provider/data/LICENSE.gluetun`.

The measurement covers container startup through the successful public-IP probe. If the fastest measured endpoint fails when selected for the final connection, the next successful candidate is tried. When more than four endpoints match, only four are sampled, so the result is not guaranteed to be the fastest in the full catalog. Each trial waits up to 40 seconds, and the complete connection is bounded to five minutes. **Check status** probes the current endpoint; use **Restart** to measure and select endpoints again. Existing running exits need a **Restart** after an application update to use the new selection rule.

The application currently supports HTTPS over TCP with SNI. It does not proxy QUIC/HTTP3 or TLS clients without SNI. Docker is required for full integration testing; unit tests and the sing-box configuration checker do not establish a real VPN tunnel.
