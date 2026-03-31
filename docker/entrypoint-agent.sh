#!/bin/bash
set -e

echo "[agent] starting agent (enrolls fresh on every start) ..."
exec /usr/local/bin/agent run
