#!/usr/bin/env bash
#
# Moves this host onto a published Domestique image, or back to the previous
# one. It is what the CI `deploy` job runs over Tailscale SSH after a merge to
# the default branch publishes an image, and it is equally the manual path:
# an operator with a digest runs exactly the same command.
#
# The deployment contract this preserves:
#
#   * The host runs an immutable digest. The caller supplies the digest alone,
#     never a full reference, and the repository name is composed here. A
#     caller that is not trusted therefore cannot point this host at another
#     registry, another repository, or a mutable tag.
#   * A deploy that does not come up healthy is undone. The previous digest is
#     recorded before the switch and restored if the health gate fails, so the
#     failure mode of an automated deployment is the service that was already
#     running rather than a service that is down.
#   * The state volume is never touched. Every path here is `up -d` on one
#     service; nothing removes a volume, and nothing runs `down`.
#
# Usage:
#   domestique-deploy.sh sha256:<64 hex>   deploy that published digest
#   domestique-deploy.sh --rollback        return to the previously deployed one
#
set -euo pipefail

DOMESTIQUE_DIR="${DOMESTIQUE_DIR:-/srv/domestique}"
IMAGE_REPO="${IMAGE_REPO:-ghcr.io/nobbs/domestique}"
STATE_DIR="${STATE_DIR:-/var/lib/domestique-deploy}"
COMPOSE_PROJECT="${COMPOSE_PROJECT:-domestique}"
COMPOSE_SERVICE="${COMPOSE_SERVICE:-domestique}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:8080/healthz}"
READY_URL="${READY_URL:-http://127.0.0.1:8081/readyz}"
CONTAINER_PORT="${CONTAINER_PORT:-8080}"
READINESS_PORT="${READINESS_PORT:-8081}"
HEALTH_TIMEOUT="${HEALTH_TIMEOUT:-60}"
LOCK_FILE="${LOCK_FILE:-/run/lock/domestique-deploy.lock}"

ENV_FILE="${DOMESTIQUE_DIR}/.env"
COMPOSE_FILE="${DOMESTIQUE_DIR}/compose.yml"
PREVIOUS_FILE="${STATE_DIR}/previous"
HISTORY_FILE="${STATE_DIR}/history"

log() {
  printf '%s domestique-deploy: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

die() {
  log "error: $*" >&2
  exit 1
}

usage() {
  cat <<'USAGE'
usage: domestique-deploy.sh sha256:<64 hex>   deploy that published digest
       domestique-deploy.sh --rollback        return to the previously deployed one
USAGE
}

# Answered before the lock, so asking how to use this never waits on a deploy.
case "${1:-}" in
  -h | --help)
    usage
    exit 0
    ;;
esac

# Serialise against a second CI deploy and against an operator running this by
# hand. Held for the whole run rather than only the compose call, because the
# .env rewrite and the health gate are one transaction.
if [[ "${DOMESTIQUE_DEPLOY_LOCKED:-}" != "1" ]]; then
  export DOMESTIQUE_DEPLOY_LOCKED=1
  status=0
  flock --nonblock --conflict-exit-code 75 "${LOCK_FILE}" "${BASH_SOURCE[0]}" "$@" || status=$?
  if [[ "${status}" -eq 75 ]]; then
    echo "error: another deploy holds ${LOCK_FILE}" >&2
  fi
  exit "${status}"
fi

[[ "$(id -u)" -eq 0 ]] || die "must run as root (the CI account reaches it through sudo)"

mode=""
requested=""
case "${1:-}" in
  --rollback) mode="rollback" ;;
  sha256:*)
    mode="digest"
    requested="$1"
    ;;
  *)
    usage >&2
    exit 64
    ;;
esac
[[ "$#" -le 1 ]] || die "too many arguments"

for path in "${ENV_FILE}" "${COMPOSE_FILE}"; do
  [[ -f "${path}" ]] || die "${path} is missing; this host is not a deployment"
done
mkdir -p "${STATE_DIR}"

# The digest is the only thing an untrusted caller controls, so it is checked
# before it is ever concatenated into a reference.
valid_digest() {
  [[ "$1" =~ ^sha256:[0-9a-f]{64}$ ]]
}

deployed_digest() {
  sed -n "s|^DOMESTIQUE_IMAGE=${IMAGE_REPO}@\(sha256:[0-9a-f]\{64\}\)[[:space:]]*$|\1|p" \
    "${ENV_FILE}" | head -n 1
}

# Rewrites only the DOMESTIQUE_IMAGE line: .env also carries the tunnel's
# Tailscale auth key, which must survive untouched. The temporary file is in
# the same directory so the move is atomic.
pin_digest() {
  local digest="$1" tmp
  tmp="$(mktemp "${ENV_FILE}.XXXXXX")"
  awk -v line="DOMESTIQUE_IMAGE=${IMAGE_REPO}@${digest}" '
    /^DOMESTIQUE_IMAGE=/ { print line; found = 1; next }
    { print }
    END { exit found ? 0 : 1 }
  ' "${ENV_FILE}" > "${tmp}" || {
    rm -f "${tmp}"
    die "${ENV_FILE} has no DOMESTIQUE_IMAGE line to replace"
  }
  chown root:root "${tmp}"
  chmod 600 "${tmp}"
  mv "${tmp}" "${ENV_FILE}"
}

compose() {
  docker compose \
    --project-name "${COMPOSE_PROJECT}" \
    --project-directory "${DOMESTIQUE_DIR}" \
    --env-file "${ENV_FILE}" \
    --file "${COMPOSE_FILE}" \
    "$@"
}

wait_healthy() {
  local deadline
  deadline=$(($(date +%s) + HEALTH_TIMEOUT))
  while true; do
    # Quiet: a connection refused while the container is still starting is
    # expected, and only the timeout that follows is worth reporting.
    if curl -fs --max-time 3 -o /dev/null "${HEALTH_URL}"; then
      return 0
    fi
    if [[ "$(date +%s)" -ge "${deadline}" ]]; then
      return 1
    fi
    sleep 2
  done
}

# Liveness says the process answers HTTP; readiness says it can read the state
# it was configured with. A deploy that comes up but cannot reach its database is
# exactly the failure this gate exists for, so it is checked before the deploy is
# called good.
#
# Skipped, with a line saying so, when this host's compose file does not publish
# the readiness port. The script deploys images, not compose files, so it has to
# work against a host that has not added that publication yet — and a missing
# publication must not roll back an image that is otherwise healthy.
wait_ready() {
  local published deadline
  published="$(compose port "${COMPOSE_SERVICE}" "${READINESS_PORT}" 2>/dev/null || true)"
  if [[ -z "${published}" ]]; then
    log "readiness port ${READINESS_PORT} is not published; readiness gate skipped"
    return 0
  fi
  if [[ "${published}" != 127.0.0.1:* ]]; then
    log "readiness port is published on ${published}, which is not loopback"
    return 1
  fi
  deadline=$(($(date +%s) + HEALTH_TIMEOUT))
  while true; do
    if curl -fs --max-time 3 -o /dev/null "${READY_URL}"; then
      return 0
    fi
    if [[ "$(date +%s)" -ge "${deadline}" ]]; then
      return 1
    fi
    sleep 2
  done
}

# A rollback onto a binary older than the schema the failed deploy migrated is a
# different failure from a rollback that is merely unhealthy: the image is fine,
# the state is intact, and no amount of restarting fixes it. The store refuses
# with a fixed message (internal/sqlite/store.go), so the log says which one this
# is, and the operator gets told rather than left to read the crash loop.
schema_ahead_of_binary() {
  compose logs --tail 50 --no-color "${COMPOSE_SERVICE}" 2>/dev/null |
    grep -q 'state schema version is newer than this service'
}

# The listener must stay loopback-only: this host has a public address, and a
# published 0.0.0.0:8080 would hand the API to the internet.
loopback_only() {
  local published
  published="$(compose port "${COMPOSE_SERVICE}" "${CONTAINER_PORT}" 2>/dev/null || true)"
  [[ "${published}" == 127.0.0.1:* ]]
}

# Pushover is reserved for the cases an operator has to know about: the app
# already notifies on every sync, and a routine deployment does not belong in
# that stream.
notify() {
  local title="$1" message="$2"
  local token_file="${DOMESTIQUE_DIR}/secrets/pushover_application_token"
  local user_file="${DOMESTIQUE_DIR}/secrets/pushover_user_key"
  if [[ ! -r "${token_file}" || ! -r "${user_file}" ]]; then
    log "no Pushover credentials on this host; notification skipped"
    return 0
  fi
  curl -fsS --max-time 10 -o /dev/null \
    --form-string "token=$(cat "${token_file}")" \
    --form-string "user=$(cat "${user_file}")" \
    --form-string "title=${title}" \
    --form-string "message=${message}" \
    --form-string "priority=1" \
    https://api.pushover.net/1/messages.json ||
    log "warning: Pushover notification failed"
}

# Keeps the running digest and the rollback target. A digest stays pullable
# after its tag has moved on, so this is disk hygiene rather than the rollback
# mechanism itself.
#
# The digest is read from each image rather than from the listing, because
# `docker images` fills its digest column only for an image that still holds the
# tag it was pulled under. Every image this prunes has lost that tag to a later
# deployment, so a prune reading the column saw an empty digest for all of them
# and removed nothing at all.
#
# What is removed is the superseded reference, never the image by its ID: an
# image may carry more than one, and this owns only the ones under IMAGE_REPO.
# Docker deletes the image once its last reference goes, so the disk is still
# reclaimed. A reference outside this repository is somebody else's, and an
# image with no repository digest at all was built on this host rather than
# deployed to it.
prune_images() {
  local keep_a="$1" keep_b="$2" id reference
  while read -r id; do
    while read -r reference; do
      [[ "${reference}" == "${IMAGE_REPO}@"* ]] || continue
      if [[ "${reference}" == "${IMAGE_REPO}@${keep_a}" || "${reference}" == "${IMAGE_REPO}@${keep_b}" ]]; then
        continue
      fi
      docker image rm "${reference}" > /dev/null 2>&1 ||
        log "kept ${reference} (still in use)"
    done < <(docker image inspect "${id}" --format '{{range .RepoDigests}}{{println .}}{{end}}' 2> /dev/null)
  done < <(docker images --quiet "${IMAGE_REPO}" | sort -u)
}

record() {
  printf '%s\t%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$1" >> "${HISTORY_FILE}"
}

current="$(deployed_digest)"
[[ -n "${current}" ]] || die "${ENV_FILE} does not pin a ${IMAGE_REPO} digest"

if [[ "${mode}" == "rollback" ]]; then
  [[ -s "${PREVIOUS_FILE}" ]] || die "no previous digest recorded in ${PREVIOUS_FILE}"
  requested="$(cat "${PREVIOUS_FILE}")"
fi

valid_digest "${requested}" || die "not a sha256 digest: ${requested}"

if [[ "${requested}" == "${current}" ]]; then
  log "already running ${IMAGE_REPO}@${current}; nothing to do"
  exit 0
fi

log "deploying ${IMAGE_REPO}@${requested} (from ${current})"
docker pull "${IMAGE_REPO}@${requested}"

printf '%s\n' "${current}" > "${PREVIOUS_FILE}"
pin_digest "${requested}"
record "${requested}"

started=0
if compose up -d "${COMPOSE_SERVICE}"; then
  started=1
else
  log "compose up failed"
fi

if [[ "${started}" -eq 1 ]] && wait_healthy && wait_ready && loopback_only; then
  log "healthy on ${IMAGE_REPO}@${requested}"
  prune_images "${requested}" "${current}"
  exit 0
fi

log "health gate failed; rolling back to ${IMAGE_REPO}@${current}"
compose logs --tail 30 --no-color "${COMPOSE_SERVICE}" || true

pin_digest "${current}"
record "${current} (rollback)"
compose up -d "${COMPOSE_SERVICE}" || log "compose up failed while rolling back"

if wait_healthy; then
  notify "Domestique: deploy rolled back" \
    "${requested} did not pass the health gate. The host is back on ${current}."
  die "deploy of ${requested} rolled back to ${current}"
fi

if schema_ahead_of_binary; then
  notify "Domestique: DOWN after rollback (state schema)" \
    "${requested} failed, and ${current} cannot open the state ${requested} migrated. The state is intact; recovery needs a binary at or past that schema."
  die "rollback to ${current} cannot open the migrated state; manual recovery needed"
fi

notify "Domestique: DOWN after rollback" \
  "${requested} failed and the rollback to ${current} is also unhealthy. Manual recovery needed."
die "rollback to ${current} is unhealthy; manual recovery needed"
