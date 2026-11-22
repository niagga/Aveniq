package http_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/client"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

type fakeClientRepository struct {
	stored map[client.ID]client.Client
}

var _ port.ClientRepository = (*fakeClientRepository)(nil)

func newFakeClientRepository() *fakeClientRepository {
	return &fakeClientRepository{stored: map[client.ID]client.Client{}}
}

func (f *fakeClientRepository) Create(_ context.Context, subject client.Client) error {
	f.stored[subject.ID] = subject
	return nil
}

func (f *fakeClientRepository) ByID(_ context.Context, id client.ID) (client.Client, error) {
	subject, ok := f.stored[id]
	if !ok {
		return client.Client{}, client.ErrNotFound
	}
	return subject, nil
}

func (f *fakeClientRepository) List(_ context.Context, activeOnly bool) ([]client.Client, error) {
	rows := make([]client.Client, 0, len(f.stored))
	for _, subject := range f.stored {
		if activeOnly && !subject.IsActive {
			continue
		}
		rows = append(rows, subject)
	}
	return rows, nil
}

func (f *fakeClientRepository) Update(_ context.Context, subject client.Client) error {
	if _, ok := f.stored[subject.ID]; !ok {
		return client.ErrNotFound
	}
	f.stored[subject.ID] = subject
	return nil
}

func TestClientRoutesRespectTheMatrix(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")
	owner := h.token(t, tenantA, iam.RoleOwner)
	accountant := h.token(t, tenantA, iam.RoleAccountant)

	status, _ := h.do(t, http.MethodGet, "/api/v1/clients", accountant, nil)
	if status != http.StatusOK {
		t.Fatalf("accountant list clients: status %d, want 200", status)
	}

	status, _ = h.do(t, http.MethodPost, "/api/v1/clients", accountant, map[string]any{
		"displayName":     "Payer S.A.",
		"billingCurrency": "ARS",
	})
	if status != http.StatusForbidden {
		t.Fatalf("accountant create client: status %d, want 403", status)
	}

	status, body := h.do(t, http.MethodPost, "/api/v1/clients", owner, map[string]any{
		"displayName":     "E8 Client " + uuid.NewString()[:8],
		"billingCurrency": "ARS",
	})
	if status != http.StatusCreated {
		t.Fatalf("owner create client: status %d, want 201 (%s)", status, body)
	}
}