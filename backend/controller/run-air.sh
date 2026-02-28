#!/usr/bin/env bash
set -euo pipefail

# load .env from current directory
set -a
source ./.env
set +a

# your program expects PEM content, but we want file-fallback, so ensure these are empty
unset INTERNAL_CA_CERT INTERNAL_CA_KEY

exec air
