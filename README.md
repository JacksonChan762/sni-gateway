# SNI Gateway — sing-box migration

[繁體中文](README.zh-TW.md)

This development branch replaces the Nginx edge and Gluetun/relay VPN exits with a Go SNI ingress and one sing-box container per WireGuard or OpenVPN exit. The admin UI and API remain on `127.0.0.1:8080` by default. Ports 80 and 443 are served by the backend. Port 80 redirects matched hosts to HTTPS; port 443 passes the original TLS connection through the selected VPN without terminating TLS. Unmatched SNI names are closed.

This is a migration branch, not the v1.1 stable release. The existing [v1.1 release notes](docs/releases/v1.1.md) describe the stable Nginx/Gluetun deployment.

## Requirements

- Docker and Docker Compose v2
- Free host ports 80 and 443
- A **complete VPN client configuration** for each exit: WireGuard interface/peer details, or a portable OpenVPN `.ovpn` profile with inline CA and any client certificate/key. Enter OpenVPN username and password separately when required.

Provider username/password and country selection alone cannot supply the client keys, tunnel address, CA certificate, or provider-specific authentication required for a VPN connection. Existing exits remain in the database; import a complete WireGuard or OpenVPN configuration before starting or publishing them. OpenVPN profiles that refer to external certificate/key files must be converted to inline `<ca>`, `<cert>`, `<key>`, and TLS key blocks. Unsupported directives are reported during import.

## Install

```bash
./sni-gateway.sh install
```

The script creates `.env` with random admin credentials if needed. Open `http://127.0.0.1:8080` and read `ADMIN_PASSWORD` from `.env`. Keep that file private. The installer builds the backend and UI and pulls the pinned official sing-box image. It removes the old Nginx Compose service and stops legacy Gluetun/relay containers after the new stack is healthy. PostgreSQL data is retained.

For an existing checkout, back up the database before switching branches. The updater described in [application updates](docs/UPDATES.md) works after this branch is installed. When moving from an older Nginx/Gluetun revision, stop the old Compose stack, switch to this branch, then run `./sni-gateway.sh install`; this avoids port 80/443 conflicts during the first migration.

## Use

1. Open **Exits** and create an exit. Import a provider WireGuard `.conf` or OpenVPN `.ovpn` file, or enter WireGuard connection fields manually. Use the provider catalog to search its server list and select a country/server; this fills the server IP and WireGuard public key or replaces the `.ovpn` remote host. For WireGuard, the interface address must use CIDR notation, such as `10.2.0.2/32`, and the endpoint must be an IP address. For OpenVPN, the `.ovpn` file must contain an inline CA certificate; enter separate credentials if the provider requires them. The gateway checks the public IP through the VPN when the exit starts. **Running** means that check succeeded; **Unhealthy** means the container started but the VPN check failed. Use **Logs** for details and **Check status** to retry the check.
2. Add exact or suffix domain rules and select an exit. A rule targeting a legacy exit cannot be published until that exit is edited with a complete WireGuard or OpenVPN configuration.
3. Open **Publish**, validate and publish the saved rules. New connections use the published table immediately. Rollback restores the previous published table. Subscription syncs can also publish updates.
4. Point the domain to the gateway host and test a TLS connection. DNS resolution for the destination occurs inside the selected sing-box exit. No direct-route fallback is configured.

The WireGuard `services.singbox.environment` YAML and OpenVPN YAML export are application import/export formats, not standalone Docker Compose files. Each dynamic sing-box container listens on SOCKS port 1080 inside the private `vpn-net` Docker network; it is not published on the host.

See the [API reference](docs/API.md) for endpoint details and WireGuard/OpenVPN request examples. OpenVPN server hostnames use local DNS only to bootstrap the tunnel; destination lookups use VPN DNS.

## Operations

```bash
./sni-gateway.sh update --check
./sni-gateway.sh update --apply
./scripts/stop.sh
cd backend && go test ./...
cd frontend && npm run build
```

`update --apply` backs up PostgreSQL, fast-forwards the tracked branch, rebuilds images, and waits for Compose health. It does not automatically restart already-running dynamic sing-box exit containers; restart exits in the UI to use a newer runtime image. The backend requires access to the Docker socket to manage exits. Backups and `.env` contain secrets and should be kept private.

The bundled provider catalog is a pinned snapshot of [Gluetun servers data](https://github.com/qdm12/gluetun-servers) (23 providers, 27,187 servers; source commit `0c7381f`). It is MIT licensed; see `backend/internal/catalog/data/LICENSE.gluetun-servers`. Server selection fills connection endpoints but cannot replace your provider-issued WireGuard client settings or OpenVPN profile. To refresh the snapshot, clone the upstream repository and run `python3 scripts/update_provider_catalog.py /path/to/gluetun-servers`; review the result before committing. A selected endpoint is saved in the exit configuration and does not silently change when the catalog is updated.

The application currently supports HTTPS over TCP with SNI. It does not proxy QUIC/HTTP3 or TLS clients without SNI. Docker is required for full integration testing; unit tests and the sing-box configuration checker do not establish a real VPN tunnel.
