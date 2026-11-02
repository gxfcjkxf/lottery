#!/usr/bin/env bash
# Initial deployment from the development/build machine.
set -Eeuo pipefail
[[ $# == 2 ]] || { echo "Usage: $0 SSH_HOST CONFIG.json" >&2; exit 1; }
exec "$(cd "$(dirname "$0")" && pwd)/03-update.sh" "$1" --initial "$2"
