-- Aveniq — saved billing reports of a practice (E7/T60, L61).
--
-- A saved report is the definition of a question, not its answer: a period, a cut and the
-- filters. Opening it asks the billing report again, so the figures are always today's and always
-- the reader's own scope: a professional who opens a report someone shared still sees only their
-- sessions and vouchers. Nothing about the figures is stored here.
--
-- Reports are shared inside the practice: every role that reads reports sees every saved one.
-- Who may change or remove one is decided by the application (its author, the owner and the
-- admin), not by this table.
--
-- The period is either a relative one (this month, last month, ...), so a saved "this month"
-- keeps meaning this month, or a custom range of two civil days.

-- +goose Up

CREATE TABLE saved_reports (
    id                   uuid        NOT NULL PRIMARY KEY,
    tenant_id            uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    name                 text        NOT NULL,

    period               text        NOT NULL,
    -- Only a custom period carries days.
    from_day             date        NULL,
    to_day               date        NULL,
    group_by             text        NOT NULL,

    -- Every filter is optional; NULL does not narrow.
    professional_user_id uuid        NULL,
    coverage             text        NULL,
    insurer_id           uuid        NULL,
    modality             text        NULL,

    created_by           uuid        NOT NULL,
    created_at           timestamptz NOT NULL,
    updated_at           timestamptz NOT NULL,

    CONSTRAINT saved_reports_name_length CHECK (char_length(btrim(name)) BETWEEN 1 AND 80),
    CONSTRAINT saved_reports_period_known CHECK (
        period IN ('this-month', 'last-month', 'last-3-months', 'this-year', 'custom')
    ),
    CONSTRAINT saved_reports_period_days CHECK (
        (period = 'custom' AND from_day IS NOT NULL AND to_day IS NOT NULL AND from_day <= to_day)
     OR (period <> 'custom' AND from_day IS NULL AND to_day IS NULL)
    ),
    CONSTRAINT saved_reports_group_by_known CHECK (
        group_by IN ('none', 'professional', 'coverage', 'insurer', 'modality', 'week', 'month', 'year')
    ),
    CONSTRAINT saved_reports_coverage_known CHECK (coverage IS NULL OR coverage IN ('private', 'insurer')),
    CONSTRAINT saved_reports_modality_known CHECK (modality IS NULL OR modality IN ('online', 'presencial')),
    -- A particular patient has no mutual.
    CONSTRAINT saved_reports_insurer_needs_coverage CHECK (insurer_id IS NULL OR coverage IS DISTINCT FROM 'private'),
    CONSTRAINT saved_reports_updated_after_creation CHECK (updated_at >= created_at),

    -- Same-practice references, the composite pattern of patients, schedule and billing.
    CONSTRAINT saved_reports_creator_same_tenant
        FOREIGN KEY (tenant_id, created_by) REFERENCES users (tenant_id, id),
    CONSTRAINT saved_reports_professional_same_tenant
        FOREIGN KEY (tenant_id, professional_user_id) REFERENCES users (tenant_id, id),
    CONSTRAINT saved_reports_insurer_same_tenant
        FOREIGN KEY (tenant_id, insurer_id) REFERENCES insurers (tenant_id, id)
);

-- A name identifies a report inside the practice, whatever its case or its outer spaces.
CREATE UNIQUE INDEX saved_reports_name_unique
    ON saved_reports (tenant_id, lower(btrim(name)));

-- ─────────────────────────────────────────────────────────────────────────────
-- Row-level security, same shape as 0002, 0004, 0007, 0009, 0014, 0016 and 0019
-- ─────────────────────────────────────────────────────────────────────────────

ALTER TABLE saved_reports ENABLE ROW LEVEL SECURITY;
ALTER TABLE saved_reports FORCE ROW LEVEL SECURITY;
CREATE POLICY saved_reports_tenant_isolation ON saved_reports
    FOR ALL
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- +goose Down

DROP TABLE IF EXISTS saved_reports;
