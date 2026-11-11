-- Aveniq — every patient has a country.
--
-- 0005 made the column nullable because a patient can exist without a document.
-- But the aggregate always resolves one: the patient either chooses it or inherits
-- the practice's, so a row without a country only ever means a write that bypassed
-- the domain. Making it NOT NULL moves that invariant from a comment into the
-- schema, and it is what lets the adapter stop treating the column as optional.

-- +goose Up

ALTER TABLE patients ALTER COLUMN country SET NOT NULL;

-- +goose Down

ALTER TABLE patients ALTER COLUMN country DROP NOT NULL;
