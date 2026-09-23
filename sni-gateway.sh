#!/usr/bin/env bash
# Install or update a Compose deployment of SNI Gateway.
set -Eeuo pipefail

# Keep the update function parsed before Git can replace this file on disk.
install_stack() {
  command -v docker >/dev/null || { echo "Docker is required." >&2; exit 1; }
  docker compose version >/dev/null || { echo "Docker Compose v2 is required." >&2; exit 1; }

  local temp_env='' jwt_secret admin_password
  umask 077
  trap 'if [[ -n "${temp_env:-}" && -f "$temp_env" ]]; then unlink "$temp_env"; fi' EXIT

  if [[ ! -e .env ]]; then
    command -v openssl >/dev/null || { echo "OpenSSL is required to create credentials." >&2; exit 1; }
    jwt_secret="$(openssl rand -hex 32)"
    admin_password="$(openssl rand -hex 24)"
    temp_env="$(mktemp "$(pwd)/.env.XXXXXXXX")"
    printf 'JWT_SECRET=%s\nADMIN_USERNAME=admin\nADMIN_PASSWORD=%s\nEXIT_RESTART_CRON="@every 12h"\n' \
      "$jwt_secret" "$admin_password" > "$temp_env"
    chmod 600 "$temp_env"
    mv -- "$temp_env" .env
    temp_env=''
    echo "Created .env with random credentials (read ADMIN_PASSWORD there to sign in)."
  else
    echo "Using existing .env; credentials were not changed."
  fi

  # Read values as text; never execute .env as shell code.
  env_value() {
    local value
    value="$(awk -v key="$1" 'index($0, key "=") == 1 { print substr($0, length(key) + 2); exit }' .env)"
    value="${value%$'\r'}"
    if [[ "$value" == \"*\" && "$value" == *\" ]]; then value="${value:1:${#value}-2}"; fi
    if [[ "$value" == \'*\' && "$value" == *\' ]]; then value="${value:1:${#value}-2}"; fi
    printf '%s' "$value"
  }

  jwt_secret="$(env_value JWT_SECRET)"
  admin_password="$(env_value ADMIN_PASSWORD)"
  if [[ "${#jwt_secret}" -lt 32 || "$jwt_secret" == replace-with-a-long-random-secret || "$jwt_secret" == change-me-in-production ]]; then
    echo "Set a unique JWT_SECRET of at least 32 characters in .env." >&2
    exit 1
  fi
  if [[ "${#admin_password}" -lt 12 || "$admin_password" == admin || "$admin_password" == replace-this-password ]]; then
    echo "Set a unique ADMIN_PASSWORD of at least 12 characters in .env." >&2
    exit 1
  fi
  unset jwt_secret admin_password

  docker compose config --quiet
  echo "Building and starting services; this may take a few minutes..."
  trap 'echo "Installation did not complete. Check: docker compose ps; docker compose logs --tail=100" >&2' ERR
  docker image inspect ghcr.io/sagernet/sing-box:v1.14.1 >/dev/null 2>&1 || docker pull ghcr.io/sagernet/sing-box:v1.14.1
  docker pull qmcgaw/gluetun:latest
  docker compose up -d --build --remove-orphans --wait --wait-timeout 180
  stop_legacy_exits
  trap - EXIT
  echo "SNI Gateway is ready at http://127.0.0.1:8080"
  echo "Read ADMIN_PASSWORD from .env to sign in. Keep that file private."
}

update_stack() {
  local mode="${1:---apply}" tool branch remote current target state backup
  if [[ "$#" -gt 1 || ( "$mode" != --apply && "$mode" != --check ) ]]; then
    usage >&2
    exit 2
  fi
  umask 077
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
    [[ "$current" == "$target" ]] && echo "Source is up to date." || echo "Update available. Run: ./sni-gateway.sh update --apply"
    return 0
  fi
  command -v docker >/dev/null || { echo "Docker is required." >&2; exit 1; }
  docker compose version >/dev/null
  state="$(git rev-parse --git-path sni-deployed-revision)"
  if [[ "$current" == "$target" && -f "$state" && "$(cat "$state")" == "$target" ]]; then
    echo "Already deployed this revision."
    return 0
  fi
  mkdir -p backups
  backup="$(mktemp -d "$(pwd)/backups/$(date -u +%Y%m%dT%H%M%SZ)-${current:0:12}-XXXXXX")"
  trap 'echo "Update failed. Database backup (if completed): $backup/database.dump. Source may have advanced; deployment may be partially updated. Review logs before retrying. See docs/UPDATES.md." >&2' ERR
  docker compose exec -T postgres pg_dump -U sni -d sni_gateway -Fc > "$backup/database.dump.partial"
  mv "$backup/database.dump.partial" "$backup/database.dump"
  printf '%s\n' "$current" > "$backup/revision"
  git merge --ff-only "$target"
  docker compose config --quiet
  docker image inspect ghcr.io/sagernet/sing-box:v1.14.1 >/dev/null 2>&1 || docker pull ghcr.io/sagernet/sing-box:v1.14.1
  docker pull qmcgaw/gluetun:latest
  docker compose build --pull
  docker compose up -d --remove-orphans --wait --wait-timeout 180
  stop_legacy_exits
  printf '%s\n' "$target" > "$state"
  echo "Update complete: $target"
  echo "Database backup: $backup/database.dump"
}

stop_legacy_exits() {
  local name prefix legacy_names
  legacy_names="$(docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U sni -d sni_gateway -Atc "SELECT name FROM exits WHERE runtime IS DISTINCT FROM 'sing-box' AND runtime IS DISTINCT FROM 'gluetun' ORDER BY id")"
  while IFS= read -r name; do
    [[ -n "$name" ]] || continue
    for prefix in gluetun- relay-; do
      if docker container inspect "${prefix}${name}" >/dev/null 2>&1; then
        docker container stop "${prefix}${name}" >/dev/null
      fi
    done
  done <<< "$legacy_names"
  docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U sni -d sni_gateway -c "UPDATE exits SET status = 'stopped', public_ip = '', updated_at = NOW() WHERE runtime IS DISTINCT FROM 'sing-box' AND runtime IS DISTINCT FROM 'gluetun'" >/dev/null
}

usage() {
  cat <<'EOF'
Usage: ./sni-gateway.sh install
       ./sni-gateway.sh update [--check|--apply]
EOF
}

main() {
  cd "$(dirname "${BASH_SOURCE[0]}")"
  local command="${1:-}"
  if [[ "$#" -gt 0 ]]; then shift; fi
  case "$command" in
    install)
      [[ "$#" -eq 0 ]] || { usage >&2; exit 2; }
      install_stack
      ;;
    update) update_stack "$@" ;;
    --help|-h) [[ "$#" -eq 0 ]] || { usage >&2; exit 2; }; usage ;;
    *) usage >&2; exit 2 ;;
  esac
}

main "$@"
