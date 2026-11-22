package patientapp_test

import (
	"context"
	"errors"
	"testing"
	"time"

	clientapp "github.com/niagga/aveniq/backend/internal/application/client"
	patientapp "github.com/niagga/aveniq/backend/internal/application/patient"
	"github.com/niagga/aveniq/backend/internal/domain/client"
	"github.com/niagga/aveniq/backend/internal/domain/insurer"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// addMutual puts a mutual of tenant-1 into the catalog the use cases read.
func addMutual(t *testing.T, w *world, id string, active bool) {
	t.Helper()
	subject, err := insurer.New(insurer.ID(id), tenant.ID("tenant-1"), insurer.Draft{Name: "Mutual " + id}, time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("new mutual: %v", err)
	}
	if !active {
		subject, err = subject.Deactivate(time.Date(2026, 11, 20, 9, 30, 0, 0, time.UTC))
		if err != nil {
			t.Fatalf("deactivate mutual: %v", err)
		}
	}
	w.insurers.stored[subject.ID] = subject
}

func insuredInput(insurerID string) patientapp.CreateInput {
	return patientapp.CreateInput{
		TenantID: "tenant-1",
		ProfileInput: patientapp.ProfileInput{
			FullName:     "Ana García",
			CoverageKind: "insurer",
			InsurerID:    insurerID,
			MemberNumber: "12345-01",
		},
		ClientProfile: clientapp.ProfileInput{BillingCurrency: "USD"},
	}
}

func expectInsurerField(t *testing.T, err error) {
	t.Helper()
	var validation *patient.ValidationError
	if !errors.As(err, &validation) || validation.Field != "insurer_id" {
		t.Fatalf("error = %v, want a ValidationError on insurer_id", err)
	}
}

func TestCreateInsuredPatientWithActiveMutual(t *testing.T) {
	w := newWorld(t)
	addMutual(t, w, "mutual-osde", true)

	view, err := w.create.Execute(context.Background(), insuredInput("mutual-osde"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if view.CoverageKind != "insurer" || view.InsurerID != "mutual-osde" || view.MemberNumber != "12345-01" {
		t.Fatalf("coverage = (%q, %q, %q), want the mutual and its member number", view.CoverageKind, view.InsurerID, view.MemberNumber)
	}
	clientID := view.ClientID
	if clientID == "" {
		t.Fatalf("clientId must be set on create")
	}
	payer, err := w.clients.ByID(context.Background(), client.ID(clientID))
	if err != nil {
		t.Fatalf("load client: %v", err)
	}
	if payer.BillingCurrency != patient.CurrencyUSD {
		t.Fatalf("billing currency = %q, want USD on the client", payer.BillingCurrency)
	}
}

func TestCreateRefusesMutualOfAnotherPractice(t *testing.T) {
	w := newWorld(t)
	// The adapter reads the catalog inside the tenant scope, so another practice's mutual
	// is simply absent from this practice's catalog.
	_, err := w.create.Execute(context.Background(), insuredInput("mutual-de-otro-consultorio"))
	expectInsurerField(t, err)
	if len(w.repo.stored) != 0 {
		t.Fatalf("a refused create must not persist anything")
	}
}

func TestCreateRefusesDeactivatedMutual(t *testing.T) {
	w := newWorld(t)
	addMutual(t, w, "mutual-inactiva", false)

	_, err := w.create.Execute(context.Background(), insuredInput("mutual-inactiva"))
	expectInsurerField(t, err)
	if len(w.repo.stored) != 0 {
		t.Fatalf("a refused create must not persist anything")
	}
}

// A patient already covered by a mutual that is later deactivated keeps its coverage and
// can still be edited. Only naming a different, deactivated mutual is refused.
func TestUpdateKeepsDeactivatedMutualUntilChanged(t *testing.T) {
	w := newWorld(t)
	addMutual(t, w, "mutual-vieja", true)
	addMutual(t, w, "mutual-nueva", false)
	view, err := w.create.Execute(context.Background(), insuredInput("mutual-vieja"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// The mutual goes inactive after the patient was covered by it.
	stored := w.insurers.stored["mutual-vieja"]
	stored.IsActive = false
	w.insurers.stored["mutual-vieja"] = stored

	// Editing the phone keeps the same mutual: allowed.
	if _, err := w.update.Execute(context.Background(), patientapp.UpdateInput{
		TenantID: "tenant-1",
		ID:       view.ID,
		ProfileInput: patientapp.ProfileInput{
			FullName:        "Ana García",
			Phone:           "11 4444 1111",
			CoverageKind:    "insurer",
			InsurerID:       "mutual-vieja",
			MemberNumber:    "12345-01",
		},
	}); err != nil {
		t.Fatalf("editing the phone of a patient with a deactivated mutual: %v", err)
	}

	// Moving to a different, deactivated mutual is refused.
	_, err = w.update.Execute(context.Background(), patientapp.UpdateInput{
		TenantID: "tenant-1",
		ID:       view.ID,
		ProfileInput: patientapp.ProfileInput{
			FullName:        "Ana García",
			CoverageKind:    "insurer",
			InsurerID:       "mutual-nueva",
			MemberNumber:    "12345-01",
		},
	})
	expectInsurerField(t, err)
}

func TestUpdateToPrivateDropsTheMutual(t *testing.T) {
	w := newWorld(t)
	addMutual(t, w, "mutual-osde", true)
	view, err := w.create.Execute(context.Background(), insuredInput("mutual-osde"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := w.update.Execute(context.Background(), patientapp.UpdateInput{
		TenantID: "tenant-1",
		ID:       view.ID,
		ProfileInput: patientapp.ProfileInput{
			FullName:        "Ana García",
			CoverageKind: "private",
		},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.CoverageKind != "private" || updated.InsurerID != "" || updated.MemberNumber != "" {
		t.Fatalf("coverage = (%q, %q, %q), want private with no mutual", updated.CoverageKind, updated.InsurerID, updated.MemberNumber)
	}
}
