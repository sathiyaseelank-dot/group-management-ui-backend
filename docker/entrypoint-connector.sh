#!/bin/bash
set -e

STATE_DIRECTORY="${STATE_DIRECTORY:-/var/lib/connector}"
mkdir -p "$STATE_DIRECTORY"

if [ ! -f "$STATE_DIRECTORY/cert.pem" ]; then
    echo "[connector] enrolling with controller at $CONTROLLER_ADDR ..."
    /usr/local/bin/connector enroll
    echo "[connector] enrollment complete"
else
    echo "[connector] using existing certificate (delete volume to re-enroll)"
fi

echo "[connector] starting connector service ..."
exec /usr/local/bin/connector run
