package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/niagga/aveniq/backend/internal/domain/clinical"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
	"github.com/niagga/aveniq/backend/internal/infrastructure/postgres"
)

// clinicalEnv builds a throwaway database with the real migrations, one patient and
// one professional per tenant, and the clinical adapter.
func clinicalEnv(t *testing.T) (*postgres.Transactor, *postgres.ClinicalRepository, map[string]string, map[string]string) {
	t.Helper()

	ownerDSN := os.Getenv("TEST_DATABASE_URL")
	appDSN := os.Getenv("TEST_APP_DATABASE_URL")
	if ownerDSN == "" || appDSN == "" {
		t.Skip("set TEST_DATABASE_URL (migrator) and TEST_APP_DATABASE_URL (application role) to run the clinical tests")
	}

	ctx := context.Background()
	tempOwnerDSN, tempAppDSN := newScratchDatabase(t, ctx, ownerDSN, appDSN)
	migrate(t, tempOwnerDSN)
	seedTenants(t, ctx, tempOwnerDSN)
	patients, professionals := seedPatientAndUserPerTenant(t, ctx, tempOwnerDSN)

	pool, err := pgxpool.New(ctx, tempAppDSN)
	if err != nil {
		t.Fatalf("cannot connect to the scratch database as the application role: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	return postgres.NewTransactor(pool), postgres.NewClinicalRepository(pool), patients, professionals
}

func TestClinicalRepository(t *testing.T) {
	transactor, repo, patients, professionals := clinicalEnv(t)
	ctx := context.Background()

	tenantA := scratchTenantA
	tenantB := scratchTenantB
	patientA := patients[tenantA]
	patientB := patients[tenantB]
	authorA := professionals[tenantA]

	newNote := func(t *testing.T, tenantID, patientID, authorID, content string, encounteredAt time.Time) clinical.Note {
		t.Helper()
		note, err := clinical.NewNote(
			clinical.NoteID(uuid.NewString()),
			tenant.ID(tenantID),
			authorID,
			clinical.NoteDraft{PatientID: patientID, EncounteredAt: encounteredAt, Content: content},
			time.Now(),
		)
		if err != nil {
			t.Fatalf("cannot build the note: %v", err)
		}
		return note
	}

	t.Run("the record is created on first save and updated after", func(t *testing.T) {
		now := time.Now()
		complaint := "Dificultad para dormir"

		record, err := clinical.NewRecord(
			clinical.RecordID(uuid.NewString()), tenant.ID(tenantA), patientA,
			clinical.RecordDraft{ChiefComplaint: &complaint}, now,
		)
		if err != nil {
			t.Fatalf("cannot build the record: %v", err)
		}

		var saved clinical.Record
		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			var inner error
			saved, inner = repo.UpsertRecord(ctx, record)
			return inner
		}); err != nil {
			t.Fatalf("cannot save the record: %v", err)
		}
		if saved.ID != record.ID {
			t.Fatalf("saved id = %q, want %q", saved.ID, record.ID)
		}

		// A second save with a new id is still the same row: one frame per patient.
		objectives := "Sostener el tratamiento"
		revised := saved.Revise(clinical.RecordDraft{Objectives: &objectives}, now.Add(time.Hour))
		revised.ID = clinical.RecordID(uuid.NewString())

		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			_, inner := repo.UpsertRecord(ctx, revised)
			return inner
		}); err != nil {
			t.Fatalf("cannot update the record: %v", err)
		}

		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			got, err := repo.RecordByPatient(ctx, patientA)
			if err != nil {
				return err
			}
			if got.ID != saved.ID {
				t.Errorf("record id = %q, want the original %q: the upsert created a second row", got.ID, saved.ID)
			}
			if got.ChiefComplaint != nil {
				t.Errorf("ChiefComplaint = %v, want it cleared by the revise", got.ChiefComplaint)
			}
			if got.Objectives == nil || *got.Objectives != objectives {
				t.Errorf("Objectives = %v, want the new value", got.Objectives)
			}
			return nil
		}); err != nil {
			t.Fatalf("cannot read the record back: %v", err)
		}
	})

	t.Run("notes are appended and listed newest first", func(t *testing.T) {
		base := time.Now().Add(-72 * time.Hour)
		for i, content := range []string{"Primera sesión", "Segunda sesión", "Tercera sesión"} {
			note := newNote(t, tenantA, patientA, authorA, content, base.Add(time.Duration(i)*24*time.Hour))
			if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
				return repo.AddNote(ctx, note)
			}); err != nil {
				t.Fatalf("cannot add %q: %v", content, err)
			}
		}

		if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			notes, err := repo.NotesByPatient(ctx, patientA, 10, 0)
			if err != nil {
				return err
			}
			if len(notes) != 3 {
				t.Fatalf("notes = %d, want 3", len(notes))
			}
			if notes[0].Content != "Tercera sesión" {
				t.Errorf("first note = %q, want the most recent encounter first", notes[0].Content)
			}
			if notes[0].AuthorID != authorA {
				t.Errorf("AuthorID = %q, want the professional who wrote it", notes[0].AuthorID)
			}
			return nil
		}); err != nil {
			t.Fatalf("cannot list the notes: %v", err)
		}
	})

	t.Run("a note cannot hang off another practice's patient", func(t *testing.T) {
		note := newNote(t, tenantA, patientB, authorA, "Nota intrusa", time.Now().Add(-time.Hour))

		err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			return repo.AddNote(ctx, note)
		})
		if !errors.Is(err, clinical.ErrPatientNotVisible) {
			t.Fatalf("error = %v, want %v: the composite key must refuse it", err, clinical.ErrPatientNotVisible)
		}
	})

	t.Run("the clinical rows are invisible from another tenant", func(t *testing.T) {
		err := transactor.WithTenant(ctx, tenantB, func(ctx context.Context) error {
			_, err := repo.RecordByPatient(ctx, patientA)
			return err
		})
		if !errors.Is(err, clinical.ErrNotFound) {
			t.Fatalf("record of another tenant: error = %v, want %v", err, clinical.ErrNotFound)
		}

		if err := transactor.WithTenant(ctx, tenantB, func(ctx context.Context) error {
			notes, err := repo.NotesByPatient(ctx, patientA, 10, 0)
			if err != nil {
				return err
			}
			if len(notes) != 0 {
				t.Fatalf("tenant B sees %d notes of tenant A", len(notes))
			}
			return nil
		}); err != nil {
			t.Fatalf("cannot list notes for B: %v", err)
		}
	})

	t.Run("a missing record is ErrNotFound", func(t *testing.T) {
		err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			_, err := repo.RecordByPatient(ctx, uuid.NewString())
			return err
		})
		if !errors.Is(err, clinical.ErrNotFound) {
			t.Fatalf("error = %v, want %v", err, clinical.ErrNotFound)
		}
	})

	t.Run("outside a tenant transaction everything is refused before the database", func(t *testing.T) {
		note := newNote(t, tenantA, patientA, authorA, "Sin scope", time.Now().Add(-time.Hour))

		if err := repo.AddNote(ctx, note); !errors.Is(err, clinical.ErrTenantScopeMissing) {
			t.Fatalf("AddNote: error = %v, want %v", err, clinical.ErrTenantScopeMissing)
		}
		if _, err := repo.RecordByPatient(ctx, patientA); !errors.Is(err, clinical.ErrTenantScopeMissing) {
			t.Fatalf("RecordByPatient: error = %v, want %v", err, clinical.ErrTenantScopeMissing)
		}
	})

	t.Run("a malformed identifier is a validation error, not a driver error", func(t *testing.T) {
		err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
			_, err := repo.RecordByPatient(ctx, "not-a-uuid")
			return err
		})

		var validation *clinical.ValidationError
		if !errors.As(err, &validation) || validation.Field != "patient_id" {
			t.Fatalf("error = %v, want a ValidationError on patient_id", err)
		}
	})
}
