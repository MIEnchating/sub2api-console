#!/bin/sh
set -eu

data_dir="${SUB2API_CONSOLE_DATA_DIR:-/app/data}"
mkdir -p "$data_dir"
data_dir="$(realpath "$data_dir")"
if [ "$data_dir" = / ]; then
  echo "SUB2API_CONSOLE_DATA_DIR must not resolve to the filesystem root" >&2
  exit 1
fi
chown -R console:console "$data_dir"

trusted_proxy_socket="${SUB2API_CONSOLE_TRUSTED_PROXY_SOCKET:-}"
if [ -n "$trusted_proxy_socket" ]; then
  case "$trusted_proxy_socket" in
    /*/*) ;;
    *)
      echo "SUB2API_CONSOLE_TRUSTED_PROXY_SOCKET must be an absolute file path with a non-root parent directory" >&2
      exit 1
      ;;
  esac
  trusted_proxy_directory="${trusted_proxy_socket%/*}"
  mkdir -p "$trusted_proxy_directory"
  trusted_proxy_directory="$(realpath "$trusted_proxy_directory")"
  if [ "$trusted_proxy_directory" = / ]; then
    echo "SUB2API_CONSOLE_TRUSTED_PROXY_SOCKET parent directory must not resolve to the filesystem root" >&2
    exit 1
  fi
  chown console:console "$trusted_proxy_directory"
  chmod 0750 "$trusted_proxy_directory"
fi

exec su-exec console "$@"
