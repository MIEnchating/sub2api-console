#!/bin/sh
set -eu

data_dir="${SUB2API_CONSOLE_DATA_DIR:-/app/data}"
mkdir -p "$data_dir"
chown -R console:console "$data_dir"
exec su-exec console "$@"
