#!/usr/bin/env bash
# Stop the stack and its dynamically created VPN/relay containers, keeping data.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

# Stop API writes and schedulers before taking the runtime inventory.
docker compose stop backend
exit_names=$(docker compose exec -T postgres psql -U sni -d sni_gateway -Atc 'SELECT name FROM exits ORDER BY id')
existing=$(docker container ls -a --format '{{.Names}}')

containers=()
while IFS= read -r name; do
  [[ -n "$name" ]] || continue
  for prefix in singbox- relay- gluetun-; do
    target="${prefix}${name}"
    while IFS= read -r candidate; do
      if [[ "$candidate" == "$target" ]]; then
        containers+=("$target")
        break
      fi
    done <<< "$existing"
  done
done <<< "$exit_names"

failed=0
if (( ${#containers[@]} > 0 )); then
  docker container stop --time 30 "${containers[@]}" || failed=1
fi
if (( failed == 0 )); then
  docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U sni -d sni_gateway -c "UPDATE exits SET status = 'stopped', public_ip = '', updated_at = NOW()" || failed=1
fi
# Finish stopping the stack even if a runtime container failed to stop.
docker compose stop || failed=1
if (( failed != 0 )); then
  echo 'Some shutdown operations failed; review the errors above.' >&2
  exit 1
fi
echo 'Stack and VPN exits stopped. Settings and volumes retained.'
