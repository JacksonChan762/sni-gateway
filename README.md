# SNI Gateway — sing-box migration

[繁體中文](README.zh-TW.md)

This development branch replaces the Nginx edge and Gluetun/relay VPN exits with a Go SNI ingress and one sing-box container per WireGuard exit. The admin UI and API remain on `127.0.0.1:8080` by default. Ports 80 and 443 are served by the backend. Port 80 redirects matched hosts to HTTPS; port 443 passes the original TLS connection through the selected VPN without terminating TLS. Unmatched SNI names are closed.

This is a migration branch, not the v1.1 stable release. The existing [v1.1 release notes](docs/releases/v1.1.md) describe the stable Nginx/Gluetun deployment.

## Requirements

- Docker and Docker Compose v2
- Free host ports 80 and 443
- A **complete WireGuard client configuration** for each exit: interface private key and address, peer endpoint IP and port, peer public key, and optionally preshared key, DNS, and allowed IPs

Provider username/password and country selection alone cannot identify a WireGuard server and cannot be migrated automatically. OpenVPN exits are not yet supported by the sing-box runtime in this branch. These exits remain in the database, but need a full WireGuard configuration before they can start or be published.

## Install

```bash
./sni-gateway.sh install
```

The script creates `.env` with random admin credentials if needed. Open `http://127.0.0.1:8080` and read `ADMIN_PASSWORD` from `.env`. Keep that file private. The installer builds the backend and UI and pulls the pinned official sing-box image. It removes the old Nginx Compose service and stops legacy Gluetun/relay containers after the new stack is healthy. PostgreSQL data is retained.

For an existing checkout, back up the database before switching branches. The updater described in [application updates](docs/UPDATES.md) works after this branch is installed. When moving from an older Nginx/Gluetun revision, stop the old Compose stack, switch to this branch, then run `./sni-gateway.sh install`; this avoids port 80/443 conflicts during the first migration.

## Use

1. Open **Exits** and create an exit. Import a provider WireGuard `.conf` file, or enter the connection fields manually. The interface address must use CIDR notation, such as `10.2.0.2/32`; the endpoint must be an IP address. Choose **Check status** to verify the public IP through the VPN.
2. Add exact or suffix domain rules and select an exit. A rule targeting a legacy exit cannot be published until that exit is edited with a complete WireGuard configuration.
3. Open **Publish**, validate and publish the saved rules. New connections use the published table immediately. Rollback restores the previous published table. Subscription syncs can also publish updates.
4. Point the domain to the gateway host and test a TLS connection. DNS resolution for the destination occurs inside the selected sing-box exit. No direct-route fallback is configured.

The `services.singbox.environment` YAML shown in the exit editor is an import/export format for this application. It is not a standalone Docker Compose file. Each dynamic sing-box container listens on SOCKS port 1080 inside the private `vpn-net` Docker network; it is not published on the host.

See the [API reference](docs/API.md) for endpoint details and a WireGuard request example.

## Operations

```bash
./sni-gateway.sh update --check
./sni-gateway.sh update --apply
./scripts/stop.sh
cd backend && go test ./...
cd frontend && npm run build
```

`update --apply` backs up PostgreSQL, fast-forwards the tracked branch, rebuilds images, and waits for Compose health. It does not automatically restart already-running dynamic sing-box exit containers; restart exits in the UI to use a newer runtime image. The backend requires access to the Docker socket to manage exits. Backups and `.env` contain secrets and should be kept private.

The application currently supports HTTPS over TCP with SNI. It does not proxy QUIC/HTTP3 or TLS clients without SNI. Docker is required for full integration testing; unit tests and the sing-box configuration checker do not establish a real VPN tunnel.
