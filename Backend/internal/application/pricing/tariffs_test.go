package pricingapp_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	pricingapp "github.com/niagga/aveniq/backend/internal/application/pricing"
	"github.com/niagga/aveniq/backend/internal/domain/insurer"
	"github.com/niagga/aveniq/backend/internal/domain/pricing"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// ─── doubles ─────────────────────────────────────────────────────────────────

const (
	tenantID     = "11111111-1111-1111-1111-111111111111"
	professional = "22222222-2222-2222-2222-222222222222"
	mutualID     = "33333333-3333-3333-3333-333333333333"
	otherMutual  = "44444444-4444-4444-4444-444444444444"
)

// fakeTariffs keeps the schedule in memory. Like the real adapter it refuses a second
// price for the same combination and start day.
type fakeTariffs struct {
	stored map[pricing.ID]pricing.Tariff
}

func newFakeTariffs() *fakeTariffs {
	return &fakeTariffs{stored: map[pricing.ID]pricing.Tariff{}}
}

func key(t pricing.Tariff) string {
	insurerID := ""
	if t.InsurerID != nil {
		insurerID = *t.InsurerID
	}
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s", t.ProfessionalUserID, t.Coverage, insurerID, t.Modality, t.Currency, t.ValidFrom.Format("2006-01-02"))
}

func (f *fakeTariffs) Create(_ context.Context, subject pricing.Tariff) error {
	for _, other := range f.stored {
		if key(other) == key(subject) {
			return pricing.ErrValidityTaken
		}
	}
	f.stored[subject.ID] = subject
	return nil
}

func (f *fakeTariffs) ByID(_ context.Context, id pricing.ID) (pricing.Tariff, error) {
	subject, ok := f.stored[id]
	if !ok {
		return pricing.Tariff{}, pricing.ErrNotFound
	}
	return subject, nil
}

func (f *fakeTariffs) List(_ context.Context) ([]port.TariffListItem, error) {
	items := []port.TariffListItem{}
	for _, subject := range f.stored {
		item := port.TariffListItem{Tariff: subject, ProfessionalName: "Demo Profesional"}
		if subject.InsurerID != nil {
			item.InsurerName = "OSDE"
		}
		items = append(items, item)
	}
	return items, nil
}

func (f *fakeTariffs) Update(_ context.Context, subject pricing.Tariff) error {
	if _, ok := f.stored[subject.ID]; !ok {
		return pricing.ErrNotFound
	}
	f.stored[subject.ID] = subject
	return nil
}

func (f *fakeTariffs) Delete(_ context.Context, id pricing.ID) error {
	if _, ok := f.stored[id]; !ok {
		return pricing.ErrNotFound
	}
	delete(f.stored, id)
	return nil
}

type fakeInsurers struct {
	stored map[insurer.ID]insurer.Insurer
}

func (f *fakeInsurers) Create(context.Context, insurer.Insurer) error { return nil }

func (f *fakeInsurers) ByID(_ context.Context, id insurer.ID) (insurer.Insurer, error) {
	subject, ok := f.stored[id]
	if !ok {
		return insurer.Insurer{}, insurer.ErrNotFound
	}
	return subject, nil
}

func (f *fakeInsurers) List(context.Context, bool) ([]insurer.Insurer, error) { return nil, nil }

func (f *fakeInsurers) Update(context.Context, insurer.Insurer) error { return nil }

// fakeScope records each opening so the tests can prove one scope per operation.
type fakeScope struct{ opened int }

func (s *fakeScope) WithTenant(ctx context.Context, _ string, fn func(ctx context.Context) error) error {
	s.opened++
	return fn(ctx)
}

type fixedClock struct{ now time.Time }

func (c *fixedClock) Now() time.Time {
	now := c.now
	c.now = c.now.Add(time.Minute)
	return now
}

type seqIDs struct{ n int }

func (s *seqIDs) NewID() string { s.n++; return fmt.Sprintf("tariff-%03d", s.n) }

type world struct {
	tariffs  *fakeTariffs
	insurers *fakeInsurers
	scope    *fakeScope
	create   *pricingapp.CreateUseCase
	update   *pricingapp.UpdateUseCase
	delete   *pricingapp.DeleteUseCase
	list     *pricingapp.ListUseCase
}

func newWorld() world {
	tariffs := newFakeTariffs()
	insurers := &fakeInsurers{stored: map[insurer.ID]insurer.Insurer{}}
	scope := &fakeScope{}
	clock := &fixedClock{now: time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)}
	ids := &seqIDs{}
	return world{
		tariffs:  tariffs,
		insurers: insurers,
		scope:    scope,
		create:   pricingapp.NewCreateUseCase(tariffs, insurers, scope, clock, ids),
		update:   pricingapp.NewUpdateUseCase(tariffs, scope, clock),
		delete:   pricingapp.NewDeleteUseCase(tariffs, scope),
		list:     pricingapp.NewListUseCase(tariffs, scope),
	}
}

// addMutual puts an active mutual of the practice in the fake catalog.
func (w world) addMutual(t *testing.T, id, name string, active bool) {
	t.Helper()
	subject, err := insurer.New(insurer.ID(id), tenant.ID(tenantID), insurer.Draft{Name: name}, time.Now())
	if err != nil {
		t.Fatalf("new mutual: %v", err)
	}
	if !active {
		subject, err = subject.Deactivate(time.Now())
		if err != nil {
			t.Fatalf("deactivate: %v", err)
		}
	}
	w.insurers.stored[subject.ID] = subject
}

var validFrom = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

func privateInput() pricingapp.CreateInput {
	return pricingapp.CreateInput{
		TenantID:           tenantID,
		ProfessionalUserID: professional,
		Coverage:           "private",
		Modality:           "presencial",
		Currency:           "ARS",
		ValidFrom:          validFrom,
		PatientMinor:       1_500_000,
	}
}

func mutualInput(insurerID string) pricingapp.CreateInput {
	return pricingapp.CreateInput{
		TenantID:           tenantID,
		ProfessionalUserID: professional,
		Coverage:           "insurer",
		InsurerID:          insurerID,
		Modality:           "online",
		Currency:           "ARS",
		ValidFrom:          validFrom,
		InsurerMinor:       1_000_000,
		PatientMinor:       200_000,
	}
}

// ─── tests ───────────────────────────────────────────────────────────────────

func TestCreateParticularPriceReturnsTheWholeFeeAsTotal(t *testing.T) {
	w := newWorld()

	view, err := w.create.Execute(context.Background(), privateInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if view.Coverage != "private" || view.InsurerID != "" || view.InsurerName != "" {
		t.Fatalf("view = %+v, want a particular price with no mutual", view)
	}
	if view.TotalMinor != 1_500_000 || view.PatientMinor != 1_500_000 || view.InsurerMinor != 0 {
		t.Fatalf("amounts = %+v, want the whole fee on the patient side", view)
	}
	if view.ValidFrom != "2026-11-01" {
		t.Fatalf("valid from = %q, want the civil day", view.ValidFrom)
	}
	if w.scope.opened != 1 {
		t.Fatalf("scope opened %d times, want one per operation", w.scope.opened)
	}
}

func TestCreateMutualPriceNamesTheMutualAndSumsTheSession(t *testing.T) {
	w := newWorld()
	w.addMutual(t, mutualID, "OSDE", true)

	view, err := w.create.Execute(context.Background(), mutualInput(mutualID))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if view.InsurerName != "OSDE" || view.InsurerID != mutualID {
		t.Fatalf("view = %+v, want the mutual by id and name", view)
	}
	if view.TotalMinor != 1_200_000 {
		t.Fatalf("total = %d, want 1200000", view.TotalMinor)
	}
}

func TestCreateRefusesAMutualOutsideTheCatalogOrDeactivated(t *testing.T) {
	w := newWorld()
	w.addMutual(t, mutualID, "OSDE", false)

	for name, id := range map[string]string{"unknown": otherMutual, "deactivated": mutualID} {
		_, err := w.create.Execute(context.Background(), mutualInput(id))
		var validation *pricing.ValidationError
		if !errors.As(err, &validation) || validation.Field != "insurer_id" {
			t.Fatalf("%s mutual: error = %v, want an insurer_id validation error", name, err)
		}
	}
	if len(w.tariffs.stored) != 0 {
		t.Fatalf("a refused price was stored: %d rows", len(w.tariffs.stored))
	}
}

func TestCreateRefusesAPriceForTheSameDayTwice(t *testing.T) {
	w := newWorld()
	if _, err := w.create.Execute(context.Background(), privateInput()); err != nil {
		t.Fatalf("first: %v", err)
	}
	_, err := w.create.Execute(context.Background(), privateInput())
	if !errors.Is(err, pricing.ErrValidityTaken) {
		t.Fatalf("second price for the same day: error = %v, want ErrValidityTaken", err)
	}
}

func TestCreateKeepsEarlierPricesOfTheSameCombination(t *testing.T) {
	w := newWorld()
	if _, err := w.create.Execute(context.Background(), privateInput()); err != nil {
		t.Fatalf("first: %v", err)
	}
	later := privateInput()
	later.ValidFrom = validFrom.AddDate(0, 1, 0)
	later.PatientMinor = 2_000_000
	if _, err := w.create.Execute(context.Background(), later); err != nil {
		t.Fatalf("a later price must coexist with the earlier one: %v", err)
	}
	if len(w.tariffs.stored) != 2 {
		t.Fatalf("stored %d tariffs, want 2 versions", len(w.tariffs.stored))
	}
}

func TestUpdateChangesOnlyTheAmounts(t *testing.T) {
	w := newWorld()
	created, err := w.create.Execute(context.Background(), privateInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	view, err := w.update.Execute(context.Background(), pricingapp.UpdateInput{
		TenantID:     tenantID,
		ID:           created.ID,
		PatientMinor: 1_800_000,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if view.TotalMinor != 1_800_000 {
		t.Fatalf("total = %d, want 1800000", view.TotalMinor)
	}
	if view.ValidFrom != created.ValidFrom || view.Coverage != created.Coverage || view.Modality != created.Modality {
		t.Fatalf("the key moved on update: before %+v after %+v", created, view)
	}
}

func TestUpdateRefusesAMutualShareOnAParticularPrice(t *testing.T) {
	w := newWorld()
	created, err := w.create.Execute(context.Background(), privateInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = w.update.Execute(context.Background(), pricingapp.UpdateInput{
		TenantID:     tenantID,
		ID:           created.ID,
		InsurerMinor: 500,
		PatientMinor: 500,
	})
	var validation *pricing.ValidationError
	if !errors.As(err, &validation) || validation.Field != "insurer_minor" {
		t.Fatalf("error = %v, want an insurer_minor validation error", err)
	}
}

func TestUpdateOfAnUnknownPriceIsNotFound(t *testing.T) {
	w := newWorld()
	_, err := w.update.Execute(context.Background(), pricingapp.UpdateInput{
		TenantID:     tenantID,
		ID:           "tariff-missing",
		PatientMinor: 1,
	})
	if !errors.Is(err, pricing.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestDeleteRemovesThePriceAndAnotherDeleteIsNotFound(t *testing.T) {
	w := newWorld()
	created, err := w.create.Execute(context.Background(), privateInput())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := w.delete.Execute(context.Background(), pricingapp.DeleteInput{TenantID: tenantID, ID: created.ID}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := w.delete.Execute(context.Background(), pricingapp.DeleteInput{TenantID: tenantID, ID: created.ID}); !errors.Is(err, pricing.ErrNotFound) {
		t.Fatalf("second delete: error = %v, want ErrNotFound", err)
	}
}

func TestListCarriesTheNamesTheScreenShows(t *testing.T) {
	w := newWorld()
	w.addMutual(t, mutualID, "OSDE", true)
	if _, err := w.create.Execute(context.Background(), privateInput()); err != nil {
		t.Fatalf("private: %v", err)
	}
	if _, err := w.create.Execute(context.Background(), mutualInput(mutualID)); err != nil {
		t.Fatalf("mutual: %v", err)
	}

	views, err := w.list.Execute(context.Background(), pricingapp.ListInput{TenantID: tenantID})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("listed %d, want 2", len(views))
	}
	for _, view := range views {
		if view.ProfessionalName != "Demo Profesional" {
			t.Fatalf("professional name = %q, want the joined name", view.ProfessionalName)
		}
	}
}
