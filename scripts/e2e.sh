#!/usr/bin/env bash
# End-to-end test of the SLD image: runs `sld serve` against fixture projects
# (test/e2e/fixtures) and drives it through the same /api/v2 calls the
# dashboard makes: set up → start → request the domain → destroy.
#
# It uses its own router, network and data volume, so it can run next to a
# real SLD installation without touching it.
#
#   SLD_IMAGE=sld:local scripts/e2e.sh          # test an image
#   E2E_PHP=1 scripts/e2e.sh                     # also test PHP (needs sld-php/sld-nginx images)
#   E2E_KEEP=1 scripts/e2e.sh                    # keep everything running for inspection
set -euo pipefail
export MSYS_NO_PATHCONV=1 # Git Bash on Windows: don't rewrite /paths in docker args

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SLD_IMAGE="${SLD_IMAGE:-sld:local}"
HTTP_PORT="${E2E_HTTP_PORT:-18080}"
HTTPS_PORT="${E2E_HTTPS_PORT:-18443}"
API_PORT="${E2E_API_PORT:-12025}"
PREFIX="sld-e2e"
TOKEN="e2e-$(date +%s)-$RANDOM$RANDOM"
TIMEOUT="${E2E_TIMEOUT:-600}"

CONTROL="$PREFIX-control"
ROUTER="$PREFIX-router"
NETWORK="$PREFIX-global"
DATA="$PREFIX-data"

pass=0
fail=0
declare -a PROJECT_IDS=()

log() { printf '\n\033[1m▶ %s\033[0m\n' "$*"; }
ok() { printf '  \033[32m✔ %s\033[0m\n' "$*"; pass=$((pass + 1)); }
bad() { printf '  \033[31m✘ %s\033[0m\n' "$*"; fail=$((fail + 1)); }

# Docker on Windows needs C:/... paths; elsewhere the path is used as-is.
host_path() { (cd "$1" && (pwd -W 2>/dev/null || pwd)); }

cleanup() {
  if [ "${E2E_KEEP:-}" = "1" ]; then
    echo "E2E_KEEP=1: leaving $CONTROL, $ROUTER and projects running (API http://127.0.0.1:$API_PORT, token $TOKEN)"
    return
  fi
  log "Cleaning up"
  for id in "${PROJECT_IDS[@]}"; do
    ids=$(docker ps -aq --filter "label=com.supreme.sld.project=$id")
    [ -n "$ids" ] && docker rm -f $ids >/dev/null 2>&1 || true
    docker volume ls -q --filter "label=com.supreme.sld.project=$id" | xargs -r docker volume rm -f >/dev/null 2>&1 || true
    docker network rm "sld-$id-net" >/dev/null 2>&1 || true
  done
  docker rm -f "$CONTROL" "$ROUTER" >/dev/null 2>&1 || true
  docker network rm "$NETWORK" >/dev/null 2>&1 || true
  docker volume rm -f "$DATA" >/dev/null 2>&1 || true
  docker volume rm -f e2e-compose-backend_default >/dev/null 2>&1 || true
  [ -n "${WORKSPACE:-}" ] && rm -rf "$WORKSPACE"
}
trap cleanup EXIT

api() { # api METHOD PATH [JSON]
  curl -sS --max-time 30 -X "$1" -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
    ${3:+--data "$3"} "http://127.0.0.1:$API_PORT/api/v2$2"
}

json_field() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | cut -d'"' -f4; }

# wait_job JOB_ID → succeeds if the job succeeded; prints its output on failure.
wait_job() {
  local job=$1 deadline=$((SECONDS + TIMEOUT)) view status
  while :; do
    view=$(api GET "/jobs/$job")
    status=$(echo "$view" | json_field status)
    [ "$status" != "running" ] && break
    [ $SECONDS -gt $deadline ] && { echo "  job $job timed out"; return 1; }
    sleep 2
  done
  if [ "$status" != "succeeded" ]; then
    echo "  job $job $status: $(echo "$view" | json_field error)"
    echo "$view" | grep -o '"output":"[^"]*' | cut -d'"' -f4 | sed 's/\\n/\n/g' | tail -25 | sed 's/^/    | /'
    return 1
  fi
}

# fetch DOMAIN MARKER → waits up to 60s for the router to serve MARKER.
fetch() {
  local domain=$1 marker=$2 body=""
  for _ in $(seq 1 30); do
    body=$(curl -sS --max-time 5 --resolve "$domain:$HTTP_PORT:127.0.0.1" "http://$domain:$HTTP_PORT/" 2>/dev/null || true)
    if echo "$body" | grep -q "$marker"; then
      echo "$body" | head -1
      return 0
    fi
    sleep 2
  done
  echo "  last response from $domain: ${body:0:300}" >&2
  return 1
}

# ---------------------------------------------------------------------------

log "Preparing workspace"
WORKSPACE="$(mktemp -d)"
cp -r "$ROOT/test/e2e/fixtures/." "$WORKSPACE/"
for dir in "$WORKSPACE"/*/; do mkdir -p "$dir.git" && echo "ref: refs/heads/main" >"$dir.git/HEAD"; done
CASES=(
  "e2e-static|e2e-static.localhost|E2E-STATIC-OK"
  "e2e-node|e2e-node.localhost|E2E-NODE-OK"
  "e2e-python|e2e-python.localhost|E2E-PYTHON-OK"
  "e2e-dockerfile|e2e-dockerfile.localhost|E2E-DOCKERFILE-OK"
  "e2e-compose|e2e-compose.localhost|E2E-COMPOSE-OK"
)
if [ "${E2E_PHP:-}" = "1" ]; then
  CASES+=("e2e-php|e2e-php.localhost|E2E-PHP-OK")
else
  rm -rf "$WORKSPACE/e2e-php"
fi
echo "  $(ls "$WORKSPACE" | tr '\n' ' ')"

log "Starting the control plane ($SLD_IMAGE)"
socket_gid=0
if [ -S /var/run/docker.sock ] && stat -c %g /var/run/docker.sock >/dev/null 2>&1; then
  socket_gid=$(stat -c %g /var/run/docker.sock)
fi
docker rm -f "$CONTROL" >/dev/null 2>&1 || true
docker run -d --name "$CONTROL" \
  -p "127.0.0.1:$API_PORT:2025" \
  -v /var/run/docker.sock:/var/run/docker.sock --group-add "$socket_gid" \
  -v "$DATA:/home/sld/.sld" \
  -v "$(host_path "$WORKSPACE"):/workspace" \
  -e SLD_ADMIN_TOKEN="$TOKEN" \
  -e SLD_HTTP_PORT="$HTTP_PORT" -e SLD_HTTPS_PORT="$HTTPS_PORT" \
  -e SLD_GLOBAL_NETWORK="$NETWORK" -e SLD_ROUTER_CONTAINER="$ROUTER" \
  "$SLD_IMAGE" serve >/dev/null

for _ in $(seq 1 30); do
  curl -fsS "http://127.0.0.1:$API_PORT/api/health" >/dev/null 2>&1 && break
  sleep 1
done
if curl -fsS "http://127.0.0.1:$API_PORT/api/health" >/dev/null 2>&1; then ok "control plane is up"; else
  bad "control plane did not start"
  docker logs "$CONTROL" | tail -30
  exit 1
fi

log "Security checks"
code=$(curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:$API_PORT/api/v2/projects" || true)
[ "$code" = "401" ] && ok "API refuses unauthenticated requests" || bad "unauthenticated request returned $code"
code=$(curl -s -o /dev/null -w '%{http_code}' -H 'Host: attacker.example' "http://127.0.0.1:$API_PORT/" || true)
[ "$code" = "403" ] && ok "foreign Host header refused (DNS rebinding)" || bad "foreign Host returned $code"

log "Project discovery"
projects=$(api GET "/projects?refresh=1")
for case in "${CASES[@]}"; do
  IFS='|' read -r name domain marker <<<"$case"
  if echo "$projects" | grep -q "\"name\":\"$name\""; then ok "found $name"; else bad "$name not discovered"; fi
done

for case in "${CASES[@]}"; do
  IFS='|' read -r name domain marker <<<"$case"
  log "$name"
  id=$(echo "$projects" | grep -o "\"id\":\"[^\"]*\",\"name\":\"$name\"" | cut -d'"' -f4)
  if [ -z "$id" ]; then bad "$name: no project id"; continue; fi
  PROJECT_IDS+=("$id")

  job=$(api POST "/projects/$id/setup" | json_field id)
  if wait_job "$job"; then ok "set up"; else bad "$name: setup failed"; continue; fi

  job=$(api POST "/projects/$id/start" | json_field id)
  if wait_job "$job"; then ok "started"; else bad "$name: start failed"; continue; fi

  if line=$(fetch "$domain" "$marker"); then ok "http://$domain → $line"; else bad "$name: $domain did not serve $marker"; fi

  if [ "$name" = "e2e-compose" ]; then
    published=$(docker port e2e-compose-backend-web-1 2>/dev/null || true)
    [ -z "$published" ] && ok "compose host port removed" || bad "compose stack still publishes: $published"
  fi
  if [ "$name" = "e2e-static" ]; then
    leak=$(curl -sS --max-time 5 --resolve "$domain:$HTTP_PORT:127.0.0.1" "http://$domain:$HTTP_PORT/.git/HEAD" || true)
    echo "$leak" | grep -q "ref:" && bad "static site exposes .git" || ok "hidden files (.git) are not served"
  fi
  if [ "$name" = "e2e-node" ] || [ "$name" = "e2e-python" ]; then
    body=$(fetch "$domain" "$marker" || true)
    echo "$body" | grep -q "uid=0" && bad "$name runs as root" || ok "runs as an unprivileged user"
  fi

  job=$(api POST "/projects/$id/destroy" '{"remove_volumes":true}' | json_field id)
  if wait_job "$job"; then ok "destroyed"; else bad "$name: destroy failed"; continue; fi
  left=$(docker ps -aq --filter "label=com.supreme.sld.project=$id" | wc -l | tr -d ' ')
  [ "$left" = "0" ] && ok "no containers left" || bad "$name: $left container(s) left after destroy"
done

log "Result: $pass passed, $fail failed"
if [ "$fail" -gt 0 ]; then
  echo "--- control plane log (tail) ---"
  docker logs "$CONTROL" 2>&1 | tail -40
  exit 1
fi
