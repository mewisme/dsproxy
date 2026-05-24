#!/bin/sh
set -e

DATA_DIR=/home/nonroot/.dsproxy
mkdir -p "$DATA_DIR"
chown -R nonroot:nonroot "$DATA_DIR"

exec su-exec nonroot:nonroot /usr/local/bin/dsproxy
