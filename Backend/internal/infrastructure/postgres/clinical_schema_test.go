package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestClinicalSchema is the schema half of T24. The property it exists for is the
// one L10 recorded and T14 had to close in Go: a foreign key does not consult
// row-level security, so `clinical_notes.patient_id` alone would let a note hang off
// another practice's patient if someone knew the UUID.
//
// 0007 answers that with composite keys — (tenant_id, patient_id) — so the refusal is
// a database property instead of a code path somebody can forget.
func TestClinicalSchema(t *testing.T) {
	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL (migrator) and TEST_APP_DATABASE_URL (application role) to run the schema tests")
	}

	ctx := context.Background()
	tempOwnerDSN, tempAppDSN := newScratchDatabase(t, ctx, ownerDSN, appDSN)
	migrate(t, tempOwnerDSN)
	seedTenants(t, ctx, tempOwnerDSN)

	patients, users := seedPatientAndUserPerTenant(t, ctx, tempOwnerDSN)

	app, err := pgx.Connect(ctx, tempAppDSN)
	if err != nil {
		t.Fatalf("cannot connect as the application role: %v", err)
	}
	defer func() { _ = app.Close(ctx) }()

	expectCode := func(t *testing.T, err error, code, what string) {
		t.Helper()
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != code {
			t.Fatalf("%s: error = %v, want SQLSTATE %s", what, err, code)
		}
	}

	insertRecord := func(ctx context.Context, tenantID, patientID string) error {
		return withTenantSetting(ctx, app, tenantID, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO clinical_records (id, tenant_id, patient_id, chief_complaint, created_at, updated_at)
				VALUES ($1, $2, $3, 'Motivo', now(), now())`,
				uuid.NewString(), tenantID, patientID)
			return err
		})
	}

	insertNote := func(ctx context.Context, tenantID, patientID, authorID, content string) error {
		return withTenantSetting(ctx, app, tenantID, func(ctx context.Context) error {
			_, err := app.Exec(ctx, `
				INSERT INTO clinical_notes (id, tenant_id, patient_id, author_user_id, encountered_at, content, created_at, updated_at)
				VALUES ($1, $2, $3, $4, now() - interval '1 day', $5, now(), now())`,
				uuid.NewString(), tenantID, patientID, authorID, content)
			return err
		})
	}

	t.Run("the application role can write both tables without an explicit grant", func(t *testing.T) {
		if err := insertRecord(ctx, scratchTenantA, patients[scratchTenantA]); err != nil {
			t.Fatalf("cannot insert a clinical record: %v — the default privileges did not reach 0007", err)
		}
		if err := insertNote(ctx, scratchTenantA, patients[scratchTenantA], users[scratchTenantA], "Primera sesión"); err != nil {
			t.Fatalf("cannot insert a clinical note: %v", err)
		}
	})

	t.Run("one record per patient", func(t *testing.T) {
		err := insertRecord(ctx, scratchTenantA, patients[scratchTenantA])
		expectCode(t, err, "23505", "second clinical record for the same patient")
	})

	t.Run("a note cannot hang off another practice's patient", func(t *testing.T) {
		// The author belongs to A and the scope is A; only the patient is foreign,
		// which is the case the composite key has to refuse.
		err := insertNote(ctx, scratchTenantA, patients[scratchTenantB], users[scratchTenantA], "Nota intrusa")
		expectCode(t, err, "23503", "note pointing at a foreign patient")

		// And nothing was written.
		var total int
		if err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			return app.QueryRow(ctx, "SELECT count(*) FROM clinical_notes WHERE content = 'Nota intrusa'").Scan(&total)
		}); err != nil {
			t.Fatalf("cannot count: %v", err)
		}
		if total != 0 {
			t.Fatalf("the refused note was written anyway (%d rows)", total)
		}
	})

	t.Run("a note cannot be signed by a user of another practice", func(t *testing.T) {
		err := insertNote(ctx, scratchTenantA, patients[scratchTenantA], users[scratchTenantB], "Firmada por otro")
		expectCode(t, err, "23503", "author from a foreign tenant")
	})

	t.Run("a blank note is refused", func(t *testing.T) {
		err := insertNote(ctx, scratchTenantA, patients[scratchTenantA], users[scratchTenantA], "   ")
		expectCode(t, err, "23514", "blank content")
	})

	t.Run("a session sees only its own clinical rows", func(t *testing.T) {
		var records, notes int
		if err := withTenantSetting(ctx, app, scratchTenantA, func(ctx context.Context) error {
			if err := app.QueryRow(ctx, "SELECT count(*) FROM clinical_records").Scan(&records); err != nil {
				return err
			}
			return app.QueryRow(ctx, "SELECT count(*) FROM clinical_notes").Scan(&notes)
		}); err != nil {
			t.Fatalf("cannot count for A: %v", err)
		}
		if records == 0 || notes == 0 {
			t.Fatalf("tenant A cannot read its own rows: %d records, %d notes", records, notes)
		}

		var foreignRecords, foreignNotes int
		if err := withTenantSetting(ctx, app, scratchTenantB, func(ctx context.Context) error {
			if err := app.QueryRow(ctx, "SELECT count(*) FROM clinical_records").Scan(&foreignRecords); err != nil {
				return err
			}
			return app.QueryRow(ctx, "SELECT count(*) FROM clinical_notes").Scan(&foreignNotes)
		}); err != nil {
			t.Fatalf("cannot count for B: %v", err)
		}
		if foreignRecords != 0 || foreignNotes != 0 {
			t.Fatalf("tenant B sees A's clinical data: %d records, %d notes", foreignRecords, foreignNotes)
		}
	})

	t.Run("without a tenant setting the policies fail closed", func(t *testing.T) {
		for _, table := range []string{"clinical_records", "clinical_notes"} {
			var total int
			if err := app.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&total); err != nil {
				t.Fatalf("cannot count %s: %v", table, err)
			}
			if total != 0 {
				t.Fatalf("%s returns %d rows without app.tenant_id, want 0", table, total)
			}
		}
	})
}

// seedPatientAndUserPerTenant creates one patient and one user in each scratch
// tenant, as the owner, and returns their ids by tenant.
func seedPatientAndUserPerTenant(t *testing.T, ctx context.Context, ownerDSN string) (patients, users map[string]string) {
	t.Helper()

	conn, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatalf("cannot connect to the scratch database: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	patients = map[string]string{}
	users = map[string]string{}

	for _, tenantID := range []string{scratchTenantA, scratchTenantB} {
		patientID := uuid.NewString()
		if _, err := conn.Exec(ctx, `
			INSERT INTO patients (id, tenant_id, country, full_name, created_at, updated_at)
			VALUES ($1, $2, 'AR', 'Paciente de prueba', now(), now())`,
			patientID, tenantID); err != nil {
			t.Fatalf("cannot insert the patient for %s: %v", tenantID, err)
		}
		patients[tenantID] = patientID

		userID := uuid.NewString()
		if _, err := conn.Exec(ctx, `
			INSERT INTO users (id, tenant_id, email, password_hash, full_name, role_code, is_active, created_at, updated_at)
			VALUES ($1, $2, $3, 'hash', 'Profesional de prueba', 'professional', true, now(), now())`,
			userID, tenantID, "profesional-"+tenantID[:8]+"@clinica.test"); err != nil {
			t.Fatalf("cannot insert the user for %s: %v", tenantID, err)
		}
		users[tenantID] = userID
	}

	return patients, users
}
