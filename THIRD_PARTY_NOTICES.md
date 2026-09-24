# Third-party components

The root [MIT License](LICENSE) covers the original SNI Gateway code contributed
under that license. Third-party code and data retain their own licenses and
copyright notices.

## Frontend

The complete license texts for the current browser build are in
[backend/public/THIRD_PARTY_NOTICES.txt](backend/public/THIRD_PARTY_NOTICES.txt).
The application serves the same file at `/THIRD_PARTY_NOTICES.txt`.

`npm run build` generates this file from the installed versions of dependencies
included in the output chunks, plus Tailwind CSS and Vite's runtime helpers.
Upstream LICENSE and NOTICE files are preserved. Some upstream license files
also describe build dependencies that are not shipped in the browser bundle.
The build fails if a bundled package has no license text. Keep this file together
with the generated JavaScript and CSS when distributing the frontend.

Lucide's ISC license identifies portions derived from Feather under MIT. The
additional [Feather license](frontend/licenses/LICENSE.feather) is retained from
[Feather v4.29.2](https://github.com/feathericons/feather/blob/v4.29.2/LICENSE)
and included in the generated notices alongside Lucide's original statement.

## Provider catalog and OpenVPN templates

- The server catalog is derived from
  [qdm12/gluetun-servers](https://github.com/qdm12/gluetun-servers), under MIT.
  Preserve [its license](backend/internal/catalog/data/LICENSE.gluetun-servers).
  The bundled snapshot records its upstream source commit.
- Public OpenVPN provider templates are derived from
  [qdm12/gluetun](https://github.com/qdm12/gluetun), under MIT.
  Preserve [its license](backend/internal/provider/data/LICENSE.gluetun).
  The source revision is recorded in `backend/internal/provider/openvpn.go`.

These notices remain applicable to the derived data even though VPN traffic is
handled by sing-box. The backend image includes these two notices under
`/app/licenses/`.

## sing-box

The gateway starts the upstream `ghcr.io/sagernet/sing-box` image as a separate
container and connects through SOCKS5. The configured version is declared in
`backend/internal/docker/singbox.go`; installation downloads that upstream image.

sing-box retains its own
[GPL-3.0-or-later license](https://github.com/SagerNet/sing-box/blob/v1.14.1/LICENSE),
including its statement concerning names and implied association. SNI Gateway
is an independent project and does not claim endorsement by the sing-box authors.

If you distribute sing-box binaries, images, or modified versions yourself, you
must comply with its license, including applicable corresponding-source and
notice requirements. The MIT license on the gateway does not replace those
requirements.

## Other dependencies and release artifacts

Backend dependencies are recorded in `backend/go.mod` and `backend/go.sum`;
frontend dependencies are recorded in `frontend/package-lock.json`. Dependency
references do not relicense the upstream projects.

The frontend notices above cover the frontend output, not every component in a
backend executable or container image. If distributing backend binaries or
images, also retain the applicable Go runtime and dependency licenses and NOTICE
files. The container operating-system packages, PostgreSQL, and separately
managed VPN images have their own licenses and distribution obligations.
