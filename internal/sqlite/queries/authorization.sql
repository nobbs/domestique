-- name: FindTargetByWahooUser :one
SELECT slot FROM targets WHERE wahoo_user_id = ? AND slot != ?;

-- name: UpdateTargetAuthorization :execresult
UPDATE targets
SET wahoo_user_id = ?, refresh_token = ?, authorization_state = ?, updated_at_unix = ?
WHERE slot = ?;

-- name: GetRefreshToken :one
SELECT refresh_token FROM targets WHERE slot = ?;

-- name: UpdateRefreshToken :execresult
UPDATE targets
SET refresh_token = ?, authorization_state = ?, updated_at_unix = ?
WHERE slot = ?;

-- name: MarkTargetNeedsReauthorization :execresult
UPDATE targets
SET refresh_token = NULL, authorization_state = ?, updated_at_unix = ?
WHERE slot = ?;

-- name: TargetExists :one
SELECT EXISTS(SELECT 1 FROM targets WHERE slot = ?);

-- name: DeletePriorOAuthTransactions :exec
DELETE FROM oauth_transactions
WHERE expires_at_unix <= ? OR (target_slot = ? AND caller_login = ? AND used_at_unix IS NULL);

-- name: InsertOAuthTransaction :exec
INSERT INTO oauth_transactions (
  id, target_slot, state_digest, code_verifier, expires_at_unix, caller_login
) VALUES (?, ?, ?, ?, ?, ?);

-- name: GetOAuthTransaction :one
SELECT target_slot, caller_login, expires_at_unix, used_at_unix
FROM oauth_transactions
WHERE state_digest = ?;

-- name: ConsumeOAuthTransaction :execresult
UPDATE oauth_transactions
SET used_at_unix = ?
WHERE state_digest = ? AND used_at_unix IS NULL;

-- name: ListPendingAuthorizations :many
SELECT DISTINCT target_slot
FROM oauth_transactions
WHERE used_at_unix IS NULL AND expires_at_unix > ?
ORDER BY target_slot;
