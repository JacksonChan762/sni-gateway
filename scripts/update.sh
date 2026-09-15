#!/usr/bin/env bash
# Run from a dedicated deployment checkout. Never discards local changes.
set -Eeuo pipefail

# Parse the complete function before Git can update this script on disk.
main() {
umask 077
cd "$(dirname "${BASH_SOURCE[0]}")/.."
mode="${1:---apply}"
if [[ "$#" -gt 1 || ( "$mode" != --apply && "$mode" != --check ) ]]; then
  echo "Usage: bash scripts/update.sh [--check|--apply]" >&2
  exit 2
fi
for tool in git flock; do
  command -v "$tool" >/dev/null || { echo "Missing prerequisite: $tool" >&2; exit 1; }
done
exec 9>"$(git rev-parse --git-path sni-update.lock)"
flock -n 9 || { echo "Another update is already running." >&2; exit 1; }
if [[ -n "$(git status --porcelain)" ]]; then
  echo "Update stopped: commit or move local changes first (including untracked files)." >&2
  exit 1
fi
branch="$(git symbolic-ref --quiet --short HEAD)" || { echo "Check out a tracking branch first." >&2; exit 1; }
remote="$(git config --get "branch.$branch.remote")" || { echo "Configure a remote tracking branch first." >&2; exit 1; }
[[ "$remote" != . ]] || { echo "A remote tracking branch is required." >&2; exit 1; }
git fetch --prune "$remote"
current="$(git rev-parse HEAD)"
target="$(git rev-parse '@{upstream}')"
git merge-base --is-ancestor "$current" "$target" || {
  echo "Update stopped: local branch has commits absent from upstream. Reconcile it manually." >&2
  exit 1
}
echo "Installed source: $current"
echo "Available source: $target"
if [[ "$mode" == --check ]]; then
  [[ "$current" == "$target" ]] && echo "Source is up to date." || echo "Update available. Run: bash scripts/update.sh --apply"
  exit 0
fi
command -v docker >/dev/null || { echo "Docker is required." >&2; exit 1; }
docker compose version >/dev/null
state="$(git rev-parse --git-path sni-deployed-revision)"
if [[ "$current" == "$target" && -f "$state" && "$(cat "$state")" == "$target" ]]; then
  echo "Already deployed this revision."
  exit 0
fi
mkdir -p backups
backup="$(mktemp -d "$(pwd)/backups/$(date -u +%Y%m%dT%H%M%SZ)-${current:0:12}-XXXXXX")"
trap 'echo "Update failed. Database backup (if completed): $backup/database.dump. Source may have advanced; deployment may be partially updated. Review logs before retrying. See docs/UPDATES.md." >&2' ERR
docker compose exec -T postgres pg_dump -U sni -d sni_gateway -Fc > "$backup/database.dump.partial"
mv "$backup/database.dump.partial" "$backup/database.dump"
printf '%s\n' "$current" > "$backup/revision"
# Update only by fast-forward. Build failures leave existing containers running.
git merge --ff-only "$target"
docker compose config --quiet
docker compose build --pull
# Compose waits for health checks and returns failure if readiness is not reached.
docker compose up -d --wait --wait-timeout 180
printf '%s\n' "$target" > "$state"
echo "Update complete: $target"
echo "Database backup: $backup/database.dump"
}

main "$@"
