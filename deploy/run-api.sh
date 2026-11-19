#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../backend"
# Keep secrets in the protected local file, outside the PM2 process definition.
set -a
source .env
set +a
exec ./bin/api
