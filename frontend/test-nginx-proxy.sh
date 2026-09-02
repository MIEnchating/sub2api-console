#!/bin/sh
set -eu

frontend_image="${SUB2API_CONSOLE_FRONTEND_TEST_IMAGE:-sub2api-console-frontend:proxy-test}"
nginx_image="${SUB2API_CONSOLE_NGINX_TEST_IMAGE:-nginx:1.27.5-alpine}"
script_directory="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"
test_suffix="$$"
network="sub2api-console-proxy-test-${test_suffix}"
api_container="${network}-api"
api_address_holder_container="${network}-api-address-holder"
client_container="${network}-client"
frontend_container="${network}-frontend"
socket_volume="${network}-socket"

cleanup() {
  docker rm -f "$frontend_container" "$client_container" "$api_container" "$api_address_holder_container" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  docker volume rm "$socket_volume" >/dev/null 2>&1 || true
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

fail() {
  echo "frontend proxy test failed: $*" >&2
  exit 1
}

header_value() {
  header_name="$1"
  awk -v target="$header_name" '
    {
      line = $0
      sub(/\r$/, "", line)
      separator = index(line, ":")
      if (separator == 0) {
        next
      }
      name = substr(line, 1, separator - 1)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", name)
      if (tolower(name) == tolower(target)) {
        value = substr(line, separator + 1)
        sub(/^[[:space:]]+/, "", value)
        print value
        exit
      }
    }
  '
}

assert_header() {
  response_headers="$1"
  header_name="$2"
  expected="$3"
  actual="$(printf '%s\n' "$response_headers" | header_value "$header_name")"
  if [ "$actual" != "$expected" ]; then
    echo "$response_headers" >&2
    fail "$header_name was '$actual', expected '$expected'"
  fi
}

start_frontend() {
  trusted_cidrs="$1"
  api_upstream="${2:-api:8080}"
  docker run -d \
    --name "$frontend_container" \
    --network "$network" \
    -v "$socket_volume:/run/sub2api-console:ro" \
    -e "SUB2API_CONSOLE_API_UPSTREAM=$api_upstream" \
    -e "SUB2API_CONSOLE_FRONTEND_TRUSTED_PROXY_CIDRS=$trusted_cidrs" \
    "$frontend_image" >/dev/null

  attempts=0
  until docker exec "$frontend_container" wget -q -O /dev/null http://127.0.0.1/; do
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 20 ]; then
      docker logs "$frontend_container" >&2 || true
      fail "frontend did not become ready"
    fi
    sleep 1
  done
}

stop_frontend() {
  docker rm -f "$frontend_container" >/dev/null
}

start_api() {
  docker run -d \
    --name "$api_container" \
    --network "$network" \
    --network-alias api \
    --network-alias 1234 \
    -v "$socket_volume:/run/sub2api-console" \
    -v "${script_directory}/testdata/echo-api.nginx.conf:/etc/nginx/nginx.conf:ro" \
    "$nginx_image" >/dev/null

  attempts=0
  until docker exec "$api_container" wget -q -O /dev/null http://127.0.0.1:8080/api/proxy-test; do
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 20 ]; then
      docker logs "$api_container" >&2 || true
      fail "API did not become ready"
    fi
    sleep 1
  done
}

stop_api() {
  docker rm -f "$api_container" >/dev/null
}

remove_stale_test_socket() {
  docker run --rm \
    -v "$socket_volume:/run/sub2api-console" \
    "$nginx_image" rm -f /run/sub2api-console/echo-api.sock
}

proxy_request() {
  forwarded_for="$1"
  forwarded_proto="$2"
  docker exec "$client_container" wget -q -S -O /dev/null \
    --header "X-Forwarded-For: $forwarded_for" \
    --header "X-Forwarded-Proto: $forwarded_proto" \
    "http://${frontend_container}/api/proxy-test" 2>&1
}

wait_for_proxy() {
  forwarded_for="$1"
  forwarded_proto="$2"
  attempts=0
  while :; do
    if response="$(proxy_request "$forwarded_for" "$forwarded_proto")"; then
      printf '%s\n' "$response"
      return
    fi
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 30 ]; then
      docker logs "$frontend_container" >&2 || true
      fail "frontend did not connect to the API"
    fi
    sleep 1
  done
}

docker network create "$network" >/dev/null
docker volume create "$socket_volume" >/dev/null
trusted_subnet="$(docker network inspect --format '{{(index .IPAM.Config 0).Subnet}}' "$network")"
[ -n "$trusted_subnet" ] || fail "test network has no IPv4 subnet"

docker run -d \
  --name "$client_container" \
  --network "$network" \
  "$nginx_image" sh -c 'sleep 300' >/dev/null

client_ip="$(docker inspect --format "{{with index .NetworkSettings.Networks \"$network\"}}{{.IPAddress}}{{end}}" "$client_container")"
[ -n "$client_ip" ] || fail "could not resolve the test client address"

# A hostname upstream must not prevent Nginx from starting while the API is
# absent, and it must become usable without restarting the frontend.
start_frontend ""
start_api
untrusted_response="$(wait_for_proxy '203.0.113.9' 'https')"
assert_header "$untrusted_response" X-Test-Forwarded-For "$client_ip"
assert_header "$untrusted_response" X-Test-Real-IP "$client_ip"
assert_header "$untrusted_response" X-Test-Forwarded-Proto http

old_api_ip="$(docker inspect --format "{{with index .NetworkSettings.Networks \"$network\"}}{{.IPAddress}}{{end}}" "$api_container")"
[ -n "$old_api_ip" ] || fail "could not resolve the original API address"
stop_api
remove_stale_test_socket
docker run -d \
  --name "$api_address_holder_container" \
  --network "$network" \
  --ip "$old_api_ip" \
  "$nginx_image" sh -c 'sleep 300' >/dev/null
start_api
new_api_ip="$(docker inspect --format "{{with index .NetworkSettings.Networks \"$network\"}}{{.IPAddress}}{{end}}" "$api_container")"
[ -n "$new_api_ip" ] || fail "could not resolve the recreated API address"
[ "$new_api_ip" != "$old_api_ip" ] || fail "recreated API unexpectedly reused its old address"
recreated_api_response="$(wait_for_proxy '203.0.113.9' 'https')"
assert_header "$recreated_api_response" X-Test-Forwarded-For "$client_ip"
assert_header "$recreated_api_response" X-Test-Forwarded-Proto http
stop_frontend

start_frontend "$trusted_subnet"
trusted_response="$(proxy_request '203.0.113.9' 'https')"
assert_header "$trusted_response" X-Test-Forwarded-For 203.0.113.9
assert_header "$trusted_response" X-Test-Real-IP 203.0.113.9
assert_header "$trusted_response" X-Test-Forwarded-Proto https

ambiguous_proto_response="$(proxy_request '198.51.100.7' 'https, http')"
assert_header "$ambiguous_proto_response" X-Test-Forwarded-For 198.51.100.7
assert_header "$ambiguous_proto_response" X-Test-Forwarded-Proto http
stop_frontend

start_frontend "" "1234:8080"
numeric_hostname_response="$(wait_for_proxy '203.0.113.9' 'https')"
assert_header "$numeric_hostname_response" X-Test-Forwarded-For "$client_ip"
assert_header "$numeric_hostname_response" X-Test-Forwarded-Proto http
stop_frontend

api_ip="$(docker inspect --format "{{with index .NetworkSettings.Networks \"$network\"}}{{.IPAddress}}{{end}}" "$api_container")"
[ -n "$api_ip" ] || fail "could not resolve the API address"
start_frontend "" "$api_ip:8080"
ipv4_response="$(proxy_request '203.0.113.9' 'https')"
assert_header "$ipv4_response" X-Test-Forwarded-For "$client_ip"
assert_header "$ipv4_response" X-Test-Forwarded-Proto http
stop_frontend

start_frontend "" "unix:/run/sub2api-console/echo-api.sock"
unix_response="$(proxy_request '203.0.113.9' 'https')"
assert_header "$unix_response" X-Test-Forwarded-For "$client_ip"
assert_header "$unix_response" X-Test-Forwarded-Proto http
stop_frontend

invalid_output=""
if invalid_output="$(docker run --rm \
  -e 'SUB2API_CONSOLE_FRONTEND_TRUSTED_PROXY_CIDRS=10.0.0.0/8; include /tmp/forged.conf' \
  "$frontend_image" nginx -t 2>&1)"; then
  fail "an injectable trusted proxy value was accepted"
fi
case "$invalid_output" in
  *"must be a comma-separated list of valid IPv4 or IPv6 CIDRs"*) ;;
  *) fail "invalid trusted proxy value did not produce a clear validation error" ;;
esac

invalid_output=""
if invalid_output="$(docker run --rm \
  -e 'SUB2API_CONSOLE_API_UPSTREAM=api:8080; include /tmp/forged.conf' \
  "$frontend_image" nginx -t 2>&1)"; then
  fail "an injectable API upstream value was accepted"
fi
case "$invalid_output" in
  *"must be unix:/absolute/path.sock or a hostname/IPv4 address followed by a port from 1 to 65535"*) ;;
  *) fail "invalid API upstream did not produce a clear validation error" ;;
esac

for invalid_port in 0 65536 12345678901234567890; do
  invalid_output=""
  if invalid_output="$(docker run --rm \
    -e "SUB2API_CONSOLE_API_UPSTREAM=api:$invalid_port" \
    "$frontend_image" nginx -t 2>&1)"; then
    fail "out-of-range API upstream port $invalid_port was accepted"
  fi
  case "$invalid_output" in
    *"must be unix:/absolute/path.sock or a hostname/IPv4 address followed by a port from 1 to 65535"*) ;;
    *) fail "invalid API upstream port $invalid_port did not produce a clear validation error" ;;
  esac
done

docker run --rm \
  -e 'SUB2API_CONSOLE_FRONTEND_TRUSTED_PROXY_CIDRS=10.20.0.0/16, 2001:db8::/32' \
  -e 'SUB2API_CONSOLE_API_UPSTREAM=127.0.0.1:8080' \
  "$frontend_image" nginx -t >/dev/null

ipv4_config="$(docker run --rm \
  -e 'SUB2API_CONSOLE_API_UPSTREAM=127.0.0.1:8080' \
  "$frontend_image" nginx -T 2>&1)"
case "$ipv4_config" in
  *"server 127.0.0.1:8080;"*) ;;
  *) fail "standard IPv4 upstream was not rendered as a static server" ;;
esac
case "$ipv4_config" in
  *"server 127.0.0.1:8080 resolve;"*) fail "standard IPv4 upstream unexpectedly enabled DNS resolution" ;;
esac

numeric_hostname_config="$(docker run --rm \
  -e 'SUB2API_CONSOLE_API_UPSTREAM=1234:8080' \
  "$frontend_image" nginx -T 2>&1)"
case "$numeric_hostname_config" in
  *"server 1234:8080 resolve;"*) ;;
  *) fail "numeric hostname upstream did not enable DNS resolution" ;;
esac

if docker run --rm \
  -e 'SUB2API_CONSOLE_FRONTEND_TRUSTED_PROXY_CIDRS=999.0.0.0/8' \
  "$frontend_image" nginx -t >/dev/null 2>&1; then
  fail "a semantically invalid CIDR was accepted"
fi

echo "frontend proxy trust tests passed"
