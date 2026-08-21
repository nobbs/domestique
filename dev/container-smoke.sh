#!/usr/bin/env bash
#
# Runs the production image the way a deployment runs it, and asserts the
# runtime contract that arrangement depends on.
#
# `build-check` proves the binary compiles for the published release target and
# the image build proves the Dockerfile still builds. Neither starts the result.
# This does: it gives the image exactly what docs/compose.example.yml gives it —
# an unprivileged user, a read-only root filesystem, no capabilities, one tmpfs,
# one writable state mount, a read-only configuration and read-only secret files
# — and then asks whether the service actually comes up under it.
#
# What is asserted, in the order it is asserted:
#
#   * the image declares the unprivileged user, the two ports, the state volume
#     and the entrypoint a deployment relies on;
#   * the liveness probe answers on the served listener, with the response
#     headers every answer on that listener carries;
#   * the readiness probe answers ready on its own listener, with no-store, and
#     serves nothing else;
#   * an unauthenticated request to the gated surface is refused;
#   * the process really runs as 65532, not as root;
#   * the root filesystem took no writes and the state landed in its mount;
#   * no synthetic secret value reached the container log;
#   * SIGTERM stops the service cleanly.
#
# A failure prints the container log with every placeholder of this run replaced,
# so a failure that happens before that assertion cannot print one either.
#
# Safety model — this reaches no provider and uses no production secret:
#
#   * Every credential is written here, by this script, as an obvious
#     placeholder. Nothing in .local is read and no deployed secret is mounted.
#   * VeloPlanner, Wahoo and Pushover point at an unroutable address, the
#     surface index is switched off, and the first scheduled synchronisation
#     is a year away, so no code path has anywhere to send anything.
#   * Cloudflare's signing keys are fetched lazily, on the first request that
#     presents an assertion. No request here presents one, so the identity gate
#     is exercised by the answer it gives an anonymous caller and Cloudflare is
#     never contacted.
#   * The state directory and the published ports are this script's own. The
#     deployed container's volume and ports are never touched, which is why the
#     host ports default off 8080 and 8081.
#
# The image is not built here. A local build needs a `docker login dhi.io` for
# the hardened base images, and CI builds the published platform in a job that
# already holds those credentials, so the image is an input: either
# build `domestique:smoke` first, or point DOMESTIQUE_SMOKE_IMAGE at a
# reference that is already in the local image store. Nothing here pulls.
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Everything is configured through the environment, so an argument is a
# misunderstanding rather than a request.
if [[ "$#" -gt 0 ]]; then
  echo "usage: $(basename "$0")" >&2
  echo "       DOMESTIQUE_SMOKE_IMAGE=<reference> $(basename "$0")" >&2
  exit 2
fi

DOCKER="${DOCKER:-docker}"
IMAGE="${DOMESTIQUE_SMOKE_IMAGE:-domestique:smoke}"
# Off the deployment's ports on purpose: this may well run on a host that is
# already serving the real thing on 8080 and 8081.
SERVED_PORT="${DOMESTIQUE_SMOKE_PORT:-18080}"
READINESS_PORT="${DOMESTIQUE_SMOKE_READINESS_PORT:-18081}"

# The user and paths the image ships. Restated here rather than read out of the
# Dockerfile, because the point of this script is to check the running image
# against the contract, not against its own build file.
IMAGE_USER="65532:65532"
CONFIG_PATH="/etc/domestique/config.toml"
SECRETS_PATH="/run/secrets"
STATE_PATH="/var/lib/domestique"

SMOKE_DIR="${ROOT}/.local/container-smoke"
SECRETS_DIR="${SMOKE_DIR}/secrets"
STATE_DIR="${SMOKE_DIR}/state"
CONFIG_FILE="${SMOKE_DIR}/config.toml"
HEADERS_FILE="${SMOKE_DIR}/headers"
BODY_FILE="${SMOKE_DIR}/body"

SERVED_URL="http://127.0.0.1:${SERVED_PORT}"
READINESS_URL="http://127.0.0.1:${READINESS_PORT}"
# Unroutable, and only ever named, never fetched. It is also an absolute HTTPS
# URL, which the origin check requires of the browser origin it derives.
UNROUTABLE="https://127.0.0.1:9"

CONTAINER="domestique-container-smoke-$$"
# Long enough for a cold start on a loaded runner, short enough that a service
# that will never answer does not hold a CI job for minutes.
READY_ATTEMPTS=60

# Every credential below is written by this script and is a placeholder. The
# encryption key has to be 32 bytes of base64url to load at all; the rest are
# never sent anywhere, because nothing this container runs has a destination.
SECRET_PLACEHOLDER="smoke-placeholder-not-a-credential"
ENCRYPTION_KEY="$(printf 'A%.0s' $(seq 43))"

log() {
  printf 'container-smoke: %s\n' "$*"
}

die() {
  printf 'container-smoke: error: %s\n' "$*" >&2
  exit 1
}

# The container log is dumped on failure and only on failure, and every
# placeholder this run mounted is replaced on the way out. The service is what
# should keep a secret value out of a log line, and that is asserted below over
# the log as it really is — but a run can fail long before it reaches that
# assertion, including while starting, so what a failure prints is filtered
# rather than trusted.
redacted() {
  sed -e "s|${SECRET_PLACEHOLDER}|[redacted]|g" -e "s|${ENCRYPTION_KEY}|[redacted]|g"
}

cleanup() {
  local status=$?
  if [[ "${status}" -ne 0 ]] && "${DOCKER}" inspect "${CONTAINER}" >/dev/null 2>&1; then
    printf 'container-smoke: the container log follows, with every placeholder redacted\n' >&2
    "${DOCKER}" logs "${CONTAINER}" 2>&1 | redacted >&2 || true
  fi
  "${DOCKER}" rm --force --volumes "${CONTAINER}" >/dev/null 2>&1 || true
}
# Cleanup hangs off EXIT alone, so it runs once however this ends. A signal
# turns into an exit rather than a handler that returns: a trap that only ran
# cleanup would leave the script carrying on from wherever the signal landed,
# against a container it had just removed.
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

image_field() {
  "${DOCKER}" image inspect --format "$1" "${IMAGE}"
}

running() {
  [[ "$("${DOCKER}" inspect --format '{{.State.Running}}' "${CONTAINER}" 2>/dev/null)" == "true" ]]
}

# One request, and the status, headers and body all come out of it, so a header
# assertion cannot pass against a different response than the status did.
request() {
  STATUS="$(curl --silent --show-error --max-time 5 \
    --dump-header "${HEADERS_FILE}" --output "${BODY_FILE}" \
    --write-out '%{http_code}' "$1")"
  LAST_URL="$1"
}

expect_status() {
  [[ "${STATUS}" == "$1" ]] || die "${LAST_URL} answered ${STATUS}, expected $1"
}

expect_header() {
  grep -qiF "$1" "${HEADERS_FILE}" || die "${LAST_URL} did not answer with '$1'"
}

expect_body() {
  grep -qF "$1" "${BODY_FILE}" || die "${LAST_URL} did not answer with '$1'"
}

command -v "${DOCKER}" >/dev/null 2>&1 || die "${DOCKER} is not on the path"
command -v curl >/dev/null 2>&1 || die "curl is not on the path"
"${DOCKER}" image inspect "${IMAGE}" >/dev/null 2>&1 ||
  die "${IMAGE} is not in the local image store; build it or set DOMESTIQUE_SMOKE_IMAGE"

log "checking what ${IMAGE} declares"

[[ "$(image_field '{{.Config.User}}')" == "${IMAGE_USER}" ]] ||
  die "the image must run as ${IMAGE_USER}, not as $(image_field '{{.Config.User}}')"
for port in 8080 8081; do
  [[ "$(image_field '{{json .Config.ExposedPorts}}')" == *"\"${port}/tcp\""* ]] ||
    die "the image must expose ${port}"
done
[[ "$(image_field '{{json .Config.Volumes}}')" == *"\"${STATE_PATH}\""* ]] ||
  die "the image must declare ${STATE_PATH} as its state volume"
[[ "$(image_field '{{json .Config.Entrypoint}}')" == *"/usr/local/bin/domestique"* ]] ||
  die "the image must start the service itself, without a shell in front of it"

# A fresh state directory every run: this asserts what a first start does with an
# empty one, which is also the case an operator meets on a new host.
rm -rf "${SMOKE_DIR}"
mkdir -p "${SECRETS_DIR}" "${STATE_DIR}"
# The container user owns nothing on this host, so the directory it writes its
# database into has to be writable by anyone, and the files it reads have to be
# readable by anyone. Everything here is synthetic and lives under .local, which
# is untracked.
chmod 0777 "${STATE_DIR}"

printf '%s' "${ENCRYPTION_KEY}" > "${SECRETS_DIR}/state_encryption_key"
for placeholder in veloplanner_email veloplanner_password wahoo_client_secret \
  pushover_application_token pushover_user_key; do
  printf '%s' "${SECRET_PLACEHOLDER}" > "${SECRETS_DIR}/${placeholder}"
done
chmod 0644 "${SECRETS_DIR}"/*

cat > "${CONFIG_FILE}" <<EOF
# Generated by dev/container-smoke.sh. Every provider below is unroutable and
# every credential is a placeholder.

[http]
listen_address = ":8080"
readiness_address = ":8081"

[access.cloudflare]
team_domain = "smoke"
application_aud = "smoke-application"
allowed_email = "rider@example.test"

[state]
database_path = "${STATE_PATH}/state.db"
encryption_key_file = "${SECRETS_PATH}/state_encryption_key"

[veloplanner]
base_url = "${UNROUTABLE}"
email_file = "${SECRETS_PATH}/veloplanner_email"
password_file = "${SECRETS_PATH}/veloplanner_password"

[wahoo]
api_base_url = "${UNROUTABLE}"
oauth_base_url = "${UNROUTABLE}"
client_id = "smoke-placeholder"
client_secret_file = "${SECRETS_PATH}/wahoo_client_secret"
redirect_url = "${UNROUTABLE}/oauth/wahoo/callback"

[[wahoo.targets]]
id = "rider-a"

[sync]
# A year out, so the scheduler never runs while this does. The interval is
# fixed at an hour by the configuration layer, and never elapses here.
initial_delay = "8760h"
interval = "1h"
max_deletions_per_target = 5
empty_source_deletion = "deny"

[surface]
# Off: this container downloads no map extract and builds no surface index.
regions = []

[notifications.pushover]
base_url = "${UNROUTABLE}"
application_token_file = "${SECRETS_PATH}/pushover_application_token"
user_key_file = "${SECRETS_PATH}/pushover_user_key"
EOF
chmod 0644 "${CONFIG_FILE}"

log "starting ${IMAGE} as ${CONTAINER}"

# The hardening docs/compose.example.yml documents, less the restart policy.
# There is deliberately no --user: the image's own declaration is what has to
# put this process on an unprivileged uid, and a flag here would hide an image
# that had lost it.
"${DOCKER}" run --detach --name "${CONTAINER}" \
  --read-only \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --tmpfs /tmp:mode=1777,nosuid,nodev,noexec \
  --publish "127.0.0.1:${SERVED_PORT}:8080" \
  --publish "127.0.0.1:${READINESS_PORT}:8081" \
  --volume "${STATE_DIR}:${STATE_PATH}" \
  --volume "${CONFIG_FILE}:${CONFIG_PATH}:ro" \
  --volume "${SECRETS_DIR}:${SECRETS_PATH}:ro" \
  "${IMAGE}" > /dev/null

for attempt in $(seq "${READY_ATTEMPTS}"); do
  if curl --silent --output /dev/null --max-time 2 "${SERVED_URL}/healthz"; then
    break
  fi
  running || die "the container exited before it answered the liveness probe"
  if [[ "${attempt}" -eq "${READY_ATTEMPTS}" ]]; then
    die "the container never answered the liveness probe"
  fi
  sleep 0.5
done

log "probing the served listener"

request "${SERVED_URL}/healthz"
expect_status 200
expect_body '"status":"ok"'
expect_header 'Cache-Control: no-store'
expect_header 'X-Content-Type-Options: nosniff'
expect_header 'Referrer-Policy: no-referrer'
expect_header 'Content-Security-Policy: '

log "probing the readiness listener"

# Ready on a database this run created: the targets are ensured at startup, so a
# first start on an empty state directory is a ready service rather than one
# waiting for something to write to it.
request "${READINESS_URL}/readyz"
expect_status 200
expect_body '"status":"ready"'
expect_header 'Cache-Control: no-store'
expect_header 'X-Content-Type-Options: nosniff'

# The probe listener is not a second door into the service. It answers /readyz
# and nothing else, so an operator can publish it to loopback without publishing
# anything that reads state.
request "${READINESS_URL}/v1/routes"
expect_status 404
expect_body '"status":"not_found"'

log "checking that the gated surface refuses an anonymous caller"

# No assertion header, so this is refused before any verification is attempted
# and Cloudflare is never asked for a key. The refusal still carries the
# response headers, and still says nothing about whether the path exists.
request "${SERVED_URL}/v1/routes"
expect_status 401
expect_body '"code":"unauthorized"'
expect_header 'X-Content-Type-Options: nosniff'
expect_header 'Referrer-Policy: no-referrer'

log "checking how the process runs"

# The daemon reports the process from outside the container, which is the only
# way to ask: the runtime image ships no shell to ask from inside. Nothing on a
# host names uid 65532, so the id itself is what `docker top` prints.
process_user="$("${DOCKER}" top "${CONTAINER}" | awk 'NR == 2 { print $1 }')"
[[ "${process_user}" == "65532" ]] ||
  die "the service runs as '${process_user}', expected 65532"

# Nothing was written into the image's own filesystem. `docker diff` reports the
# container's own layer, and what a mount holds is not part of it, so a write
# that had landed anywhere but the state mount and the tmpfs would show up here.
#
# The two read-only mounts do show up. The image ships /etc/domestique but no
# configuration file in it, and no /run/secrets at all, so the runtime creates
# those two mount points in that layer to mount over, and reports the
# directories above them as changed because it did. That is the mounts arriving
# rather than the service writing, so those paths and their parents are the only
# entries allowed.
expected_changes=()
for target in "${CONFIG_PATH}" "${SECRETS_PATH}"; do
  while [[ "${target}" != "/" ]]; do
    expected_changes+=("${target}")
    target="$(dirname "${target}")"
  done
done

unexpected=()
while read -r _ path; do
  [[ -n "${path}" ]] || continue
  for expected in "${expected_changes[@]}"; do
    if [[ "${path}" == "${expected}" ]]; then
      continue 2
    fi
  done
  unexpected+=("${path}")
done <<< "$("${DOCKER}" diff "${CONTAINER}")"

[[ "${#unexpected[@]}" -eq 0 ]] ||
  die "the read-only root filesystem took writes: ${unexpected[*]}"

# So the state has to be on the mount, and it is.
[[ -f "${STATE_DIR}/state.db" ]] ||
  die "no state database appeared in the writable state mount"

log "checking that no secret value reached the log"

# The log as it really is, not the filtered form a failure prints: the point is
# that the service keeps these out, not that this script can hide them.
container_log="$("${DOCKER}" logs "${CONTAINER}" 2>&1)"
for secret in "${SECRET_PLACEHOLDER}" "${ENCRYPTION_KEY}"; do
  [[ "${container_log}" != *"${secret}"* ]] ||
    die "a secret value appears in the container log"
done

log "stopping the service"

# SIGTERM straight to the service, because it is pid 1 with no shell in front of
# it. A clean exit is what makes a deployment's restart a restart rather than a
# recovery from a killed process.
"${DOCKER}" stop --timeout 20 "${CONTAINER}" > /dev/null
exit_code="$("${DOCKER}" inspect --format '{{.State.ExitCode}}' "${CONTAINER}")"
[[ "${exit_code}" == "0" ]] ||
  die "the service exited ${exit_code} on SIGTERM, expected a clean shutdown"

log "the production image satisfies the runtime contract"
