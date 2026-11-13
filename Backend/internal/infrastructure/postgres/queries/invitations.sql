-- Invitations: pending offers of an account. Same rule as the other tenant tables:
-- the tenant comes from app.tenant_id (set by Transactor.WithTenant), never from a
-- WHERE clause, and a statement without tenant context sees and writes nothing.

-- name: CreateInvitation :one
INSERT INTO invitations (id, tenant_id, email, role_code, token_hash, invited_by, expires_at, accepted_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, NULL, $8)
RETURNING *;

-- name: GetInvitationByID :one
SELECT *
FROM invitations
WHERE id = $1;

-- name: GetInvitationByTokenHash :one
SELECT *
FROM invitations
WHERE token_hash = $1;

-- name: ListPendingInvitations :many
SELECT *
FROM invitations
WHERE accepted_at IS NULL
  AND expires_at > $1
ORDER BY created_at DESC, id DESC
LIMIT $2 OFFSET $3;

-- name: MarkInvitationAccepted :execrows
-- The guard is in the statement, not in Go: two requests racing on one link both
-- pass a read, and only one UPDATE matches.
UPDATE invitations
SET accepted_at = $2
WHERE id = $1
  AND accepted_at IS NULL
  AND expires_at > $2;

-- name: DeleteInvitation :execrows
-- An accepted invitation is history and stays. Only a pending one can be revoked.
DELETE FROM invitations
WHERE id = $1
  AND accepted_at IS NULL;
