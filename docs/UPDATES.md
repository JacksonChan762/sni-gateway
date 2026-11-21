# Application updates

SNI Gateway supports optional unattended updates on Linux hosts using Docker Compose
and a systemd timer. Subscription domain lists already sync and publish automatically;
application updates are a separate operation.

## Prepare a deployment

Use a dedicated, clean Git checkout on a branch tracking a trusted upstream. The updater
deploys **every commit on that branch**, not just tagged releases. Choose a stable branch
appropriate for your deployment. It refuses local changes, untracked files, detached HEAD,
and diverged or locally ahead branches. Keep `.env` in the checkout (it is Git-ignored).

Requirements: Bash, Git, `flock`, Docker and Docker Compose v2 supporting `up --wait`.
The stack must already be running, including PostgreSQL (start it with `./sni-gateway.sh install`). The account running the update
needs Docker access and noninteractive Git read access. Docker now builds the frontend
from source; Node.js is not required on the host.

Check for new source without rebuilding or restarting services:

```bash
./sni-gateway.sh update --check
```

Apply an update:

```bash
./sni-gateway.sh update --apply
```

The updater locks out concurrent runs, fetches the configured upstream, saves a
PostgreSQL custom-format dump under `backups/`, fast-forwards the source, builds images,
and starts the stack with a 180-second readiness wait. It records the deployed commit
only after success. A failed attempt is retried on the next run even if source already
advanced. The first run rebuilds once to establish that record.

Compose builds include the current frontend and backend. Installation and updates use the sing-box image configured by the backend source at that revision. Existing dynamically managed VPN exits keep their running images until restarted in the UI; this updater does not roll those containers. Edit older Gluetun provider exits in the UI to migrate them to sing-box. Published routes and database volumes are retained. The readiness check
covers Compose service health, not end-to-end VPN routing; test a routed domain afterward.
Backups contain private configuration and have restrictive permissions. Copy them to
secure storage and periodically remove old backups according to your retention policy.

## Switch an existing deployment to another branch

This procedure assumes an existing sing-box deployment with the `update` command
in `sni-gateway.sh`. Start in the original checkout at the version currently deployed,
with PostgreSQL running. Keep the same `.env` and Compose project name so Docker uses
the existing volumes. This example targets `feature/ddns-tls-rule-folders`.

Create the local tracking branch at the currently deployed commit. That gives the
updater the correct old revision to record alongside the database backup.

```bash
upgrade_branch=feature/ddns-tls-rule-folders
test -z "$(git status --porcelain)" &&
git remote set-branches --add origin "$upgrade_branch" &&
git fetch origin &&
git merge-base --is-ancestor HEAD "origin/$upgrade_branch" &&
git switch -c "$upgrade_branch" &&
git branch --set-upstream-to="origin/$upgrade_branch" &&
./sni-gateway.sh update --check
```

Confirm that `Available source` is the intended commit, then apply the update and
check the Compose services.

```bash
./sni-gateway.sh update --apply && docker compose ps
```

Future manual and scheduled updates use the new tracking branch.

For older deployments with Nginx ingress or a separate `update.sh`, record the
deployed Git revision and make a PostgreSQL backup first. Fast-forward the original
checkout to the target version and run that version's `./sni-gateway.sh install`.
Publish the saved rules if the new gateway has no published routes. If scheduled
updates were enabled, refresh the service unit using the steps below.

## Enable daily updates (optional)

First run a manual update successfully. Edit **both paths** in
`deploy/systemd/sni-gateway-update.service` to match your deployment checkout. The supplied
unit runs as root; configure `User=` if you use a dedicated deployment account with Docker
access, and ensure that account owns the checkout and can authenticate to Git.

```bash
sudo cp deploy/systemd/sni-gateway-update.{service,timer} /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now sni-gateway-update.timer
systemctl list-timers sni-gateway-update.timer
```

The timer runs daily at 04:00 in the host timezone with up to 30 minutes of random delay.
View results or disable it:

```bash
journalctl -u sni-gateway-update.service -n 100
sudo systemctl disable --now sni-gateway-update.timer
```

## Failed updates and recovery

Build failures leave running containers in place. Startup failures can leave a partially
updated deployment. Updates can briefly interrupt the admin UI or traffic. There is no
automatic database rollback: newer application code may have migrated the schema, and
restoring a dump would discard later changes.

1. Disable the timer and inspect the service logs.
2. Locate the last backup's `revision` and completed `database.dump`; a `.partial` file
   is not a completed backup. Preserve `.env` and the named Docker volumes.
3. For a transient network/build failure, rerun `./sni-gateway.sh update --apply`.
4. For a code regression, review schema compatibility before deploying the recorded
   previous revision from a separate checkout. Use the same Compose project name and
   `.env` to address the existing volumes. Do not run `docker compose down -v`.
5. If database restoration is required, stop writers and have an operator restore the
   custom-format dump with `pg_restore` after reviewing the data-loss implications.

The Publish page's rollback restores the previous routing table only; it does not roll back
application code or database migrations.

## Development verification

Run `python3 scripts/tests/test_update.py` to check updater behavior with temporary Git
repositories and a simulated Docker CLI. These tests do not contact your upstream or
change a running deployment. They cover check-only mode, local-change protection,
backups, successful deployment, repeat runs, and backup/build/health failures.
