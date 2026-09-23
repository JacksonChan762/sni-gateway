#!/usr/bin/env bash
# Install the Compose stack without overwriting an existing .env.
set -Eeuo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"
umask 077

if [[ "${1:-}" == --help && "$#" -eq 1 ]]; then
  echo "Usage: ./install.sh"
  exit 0
fi
if [[ "$#" -ne 0 ]]; then
  echo "Usage: ./install.sh" >&2
  exit 2
fi

command -v docker >/dev/null || { echo "Docker is required." >&2; exit 1; }
docker compose version >/dev/null || { echo "Docker Compose v2 is required." >&2; exit 1; }

temp_env=''
trap 'if [[ -n "$temp_env" && -f "$temp_env" ]]; then unlink "$temp_env"; fi' EXIT

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

# Read these values as text; never execute .env as shell code.
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
docker compose up -d --build --wait --wait-timeout 180
echo "SNI Gateway is ready at http://127.0.0.1:8080"
echo "Read ADMIN_PASSWORD from .env to sign in. Keep that file private."
