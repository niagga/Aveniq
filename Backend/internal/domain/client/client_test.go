package client_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/domain/client"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

var now = time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)

func newClient(t *testing.T, draft client.Draft) client.Client {
	t.Helper()
	subject, err := client.New(client.ID(uuid.NewString()), tenant.ID(uuid.NewString()), draft, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return subject
}

func TestNewClientRequiresDisplayName(t *testing.T) {
	_, err := client.New(client.ID(uuid.NewString()), tenant.ID(uuid.NewString()), client.Draft{
		Country:         "AR",
		BillingCurrency: patient.CurrencyARS,
	}, now)
	if err == nil {
		t.Fatal("want validation error for blank display name")
	}
}

func TestNewClientTrimsDisplayName(t *testing.T) {
	subject := newClient(t, client.Draft{
		DisplayName:     "  María López  ",
		Country:         "AR",
		BillingCurrency: patient.CurrencyARS,
	})
	if subject.DisplayName != "María López" {
		t.Errorf("DisplayName = %q", subject.DisplayName)
	}
}