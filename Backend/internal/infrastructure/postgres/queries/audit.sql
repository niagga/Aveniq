-- name: CreateAuditEntry :exec
-- created_at comes from the database clock on purpose: an audit trail with one clock
-- is worth more than one with a clock per caller.
INSERT INTO audit_log (tenant_id, actor_id, action, entity, entity_id, ip, metadata, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, now());
