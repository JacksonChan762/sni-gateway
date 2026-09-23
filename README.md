# SNI Gateway — Gluetun providers and sing-box custom exits

[繁體中文](README.zh-TW.md)

This branch uses a Go SNI ingress. Built-in provider exits run Gluetun with a sing-box SOCKS proxy in the same protected network namespace; Custom exits use sing-box directly. The admin UI and API remain on `127.0.0.1:8080` by default. Ports 80 and 443 are served by the backend. Port 80 redirects matched hosts to HTTPS; port 443 passes the original TLS connection through the selected VPN without terminating TLS. Unmatched SNI names are closed.

This is a migration branch, not the v1.1 stable release. The existing [v1.1 release notes](docs/releases/v1.1.md) describe the stable Nginx/Gluetun deployment.

## Requirements

- Docker and Docker Compose v2
- Free host ports 80 and 443
- Provider exits need the credentials required by that provider: OpenVPN service username/password and, for some providers, client certificate/key; or WireGuard private key and, where required, tunnel address and preshared key. Custom exits need a complete WireGuard configuration or portable OpenVPN `.ovpn` profile.

## Install

```bash
./sni-gateway.sh install
```

The script creates `.env` with random admin credentials if needed. Open `http://127.0.0.1:8080` and read `ADMIN_PASSWORD` from `.env`. Keep that file private. The installer builds the backend and UI and pulls the official sing-box and Gluetun images. It removes the old Nginx Compose service and stops legacy relay containers after the new stack is healthy. PostgreSQL data is retained.

For an existing checkout, back up the database before switching branches. The updater described in [application updates](docs/UPDATES.md) works after this branch is installed. When moving from an older Nginx/Gluetun revision, stop the old Compose stack, switch to this branch, then run `./sni-gateway.sh install`; this avoids port 80/443 conflicts during the first migration.

## Use

1. Open **Exits** and select a built-in provider, country, optional city, and protocol. For OpenVPN, enter provider service credentials; providers that use client certificates ask for those too. For WireGuard, enter the private key and any additional fields shown for that provider. To use your own VPN server, select **Custom configuration** and import a WireGuard `.conf` or complete OpenVPN `.ovpn` profile. The gateway checks the public IP through the VPN on startup. **Running** means the probe succeeded; **Unhealthy** means it did not. Use **Logs** and **Check status** to investigate.
2. Add exact or suffix domain rules and select an exit. A rule targeting an unconverted legacy exit cannot be published until that exit is edited.
3. Open **Publish**, validate and publish the saved rules. New connections use the published table immediately. Rollback restores the previous published table. Subscription syncs can also publish updates.
4. Point the domain to the gateway host and test a TLS connection. DNS resolution for the destination occurs inside the selected VPN network. Gluetun provider exits use its firewall to prevent direct fallback.

The WireGuard environment YAML and OpenVPN YAML export are application import/export formats, not standalone Docker Compose files. Each exit has a sing-box SOCKS listener on port 1080 inside the private `vpn-net` Docker network. Provider exits use Gluetun’s network namespace and VPN firewall. The SOCKS listener is not published on the host.

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

The UI location catalog starts with a pinned snapshot of [Gluetun servers data](https://github.com/qdm12/gluetun-servers) and checks upstream daily. Gluetun itself updates the active provider’s servers every 480 hours after its tunnel is ready, following [Gluetun’s update guidance](https://github.com/qdm12/gluetun-wiki/blob/main/setup/servers.md). Its data is stored in a per-exit Docker volume. To refresh the bundled UI fallback snapshot for releases, run `python3 scripts/update_provider_catalog.py /path/to/gluetun-servers`. The catalog data is MIT licensed; see `backend/internal/catalog/data/LICENSE.gluetun-servers`.

The application currently supports HTTPS over TCP with SNI. It does not proxy QUIC/HTTP3 or TLS clients without SNI. Docker is required for full integration testing; unit tests and the sing-box configuration checker do not establish a real VPN tunnel.
