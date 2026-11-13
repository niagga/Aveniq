-- Aveniq — trial period on the practice (T36).
--
-- A self-registered practice starts in trial and the trial has an end: the date is
-- stored, not derived, so a later change to the trial length does not move the
-- end of practices already registered.
--
-- The default covers rows written by the local seed script, which inserts tenants
-- directly. The domain always sets the column explicitly for new practices.

-- +goose Up
ALTER TABLE tenants
    ADD COLUMN trial_ends_at timestamptz NOT NULL DEFAULT (now() + interval '14 days');

-- Existing practices get the same 14 days counted from their creation, not from
-- the migration, so a practice created last month is not handed a fresh trial.
UPDATE tenants
SET trial_ends_at = created_at + interval '14 days';

ALTER TABLE tenants
    ADD CONSTRAINT tenants_trial_after_creation CHECK (trial_ends_at > created_at);

COMMENT ON COLUMN tenants.trial_ends_at IS 'End of the trial period. Set at registration to 14 days after creation.';

-- +goose Down
ALTER TABLE tenants DROP CONSTRAINT IF EXISTS tenants_trial_after_creation;
ALTER TABLE tenants DROP COLUMN IF EXISTS trial_ends_at;
