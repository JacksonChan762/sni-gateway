#!/usr/bin/env bash
# Convenient entrypoint for the backed-up, health-checked updater.
set -Eeuo pipefail
exec bash "$(dirname "${BASH_SOURCE[0]}")/scripts/update.sh" "$@"
