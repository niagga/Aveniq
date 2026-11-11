package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// TestUserRepositoryRefusesToRunOutsideTenantScope pins the guard that replaced
// the silent pool fallback.
//
// The earlier adapter fell back to the pool when the context carried no tenant
// transaction, leaving the answer to row-level security: reads came back as zero
// rows and the caller saw iam.ErrNotFound. That made a scope bug indistinguishable
// from a deleted user — a 404 with no signal anywhere. The adapter now refuses
// before it reaches the database, and this test keeps it that way for all six
// methods, including the read paths that used to look like ordinary "not found".
func TestUserRepositoryRefusesToRunOutsideTenantScope(t *testing.T) {
	_, transactor, repo, tenantA, _ := iamEnv(t)
	ctx := context.Background()

	subject := newUser(t, tenantA, "scoped@iam.test")
	if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
		return repo.Create(ctx, subject)
	}); err != nil {
		t.Fatalf("cannot create the user inside WithTenant: %v", err)
	}

	assertScopeMissing := func(t *testing.T, operation string, err error) {
		t.Helper()
		if !errors.Is(err, iam.ErrTenantScopeMissing) {
			t.Errorf("%s outside a tenant transaction = %v, want %v", operation, err, iam.ErrTenantScopeMissing)
		}
	}

	assertScopeMissing(t, "Create", repo.Create(ctx, newUser(t, tenantA, "unscoped@iam.test")))
	_, byIDErr := repo.ByID(ctx, subject.ID)
	assertScopeMissing(t, "ByID", byIDErr)
	_, byEmailErr := repo.ByEmail(ctx, subject.Email)
	assertScopeMissing(t, "ByEmail", byEmailErr)
	_, listErr := repo.List(ctx, 10, 0)
	assertScopeMissing(t, "List", listErr)
	assertScopeMissing(t, "Update", repo.Update(ctx, subject))
	assertScopeMissing(t, "Delete", repo.Delete(ctx, subject.ID))

	// None of the calls above may have reached the database: the user must still
	// be there and the refused insert must not have created a second one.
	listErr = transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
		users, err := repo.List(ctx, 100, 0)
		if err != nil {
			return err
		}
		if len(users) != 1 {
			t.Errorf("tenant A holds %d users after the unscoped attempts, want 1", len(users))
		}
		if len(users) == 1 && users[0].ID != subject.ID {
			t.Errorf("tenant A holds user %s, want %s", users[0].ID, subject.ID)
		}
		return nil
	})
	if listErr != nil {
		t.Fatalf("cannot list inside WithTenant: %v", listErr)
	}
}

// TestUserRepositoryRejectsCrossTenantAccess pins the asymmetry between INSERT
// and UPDATE/DELETE under FORCE row-level security, because assuming symmetry is
// how a tenant boundary gets misread.
//
// INSERT is rejected by the WITH CHECK policy and raises SQLSTATE 42501. UPDATE
// and DELETE are filtered by USING instead: they raise nothing at all, affect
// zero rows, and sqlc reports pgx.ErrNoRows. Neither outcome may be read as "the
// scope check passed", and neither may be reported as a missing tenant context —
// the scope was set, the row simply is not ours.
func TestUserRepositoryRejectsCrossTenantAccess(t *testing.T) {
	_, transactor, repo, tenantA, tenantB := iamEnv(t)
	ctx := context.Background()

	userB := newUser(t, tenantB, "b-only@iam.test")
	if err := transactor.WithTenant(ctx, tenantB, func(ctx context.Context) error {
		return repo.Create(ctx, userB)
	}); err != nil {
		t.Fatalf("cannot create the user in tenant B: %v", err)
	}

	userA := newUser(t, tenantA, "a-only@iam.test")
	if err := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
		return repo.Create(ctx, userA)
	}); err != nil {
		t.Fatalf("cannot create the user in tenant A: %v", err)
	}

	hijacked := userB
	hijacked.FullName = "hijacked from another tenant"
	intruder := newUser(t, tenantB, "intruder@iam.test")

	// Every attempt gets its own transaction. PostgreSQL aborts the transaction
	// on the first failed statement, so a second call inside the same scope would
	// report "current transaction is aborted" and measure the wrong thing. That
	// is also why a use case must not attempt a recovery inside WithTenant: once
	// a statement fails, the unit of work is over.
	attempts := []struct {
		name string
		want error
		run  func(ctx context.Context) error
	}{
		{"ByID", iam.ErrNotFound, func(ctx context.Context) error {
			_, err := repo.ByID(ctx, userB.ID)
			return err
		}},
		{"ByEmail", iam.ErrNotFound, func(ctx context.Context) error {
			_, err := repo.ByEmail(ctx, userB.Email)
			return err
		}},
		{"Update", iam.ErrNotFound, func(ctx context.Context) error {
			return repo.Update(ctx, hijacked)
		}},
		{"Delete", iam.ErrNotFound, func(ctx context.Context) error {
			return repo.Delete(ctx, userB.ID)
		}},
		{"Create", iam.ErrInsufficientPrivilege, func(ctx context.Context) error {
			return repo.Create(ctx, intruder)
		}},
	}

	for _, attempt := range attempts {
		t.Run(attempt.name, func(t *testing.T) {
			err := transactor.WithTenant(ctx, tenantA, attempt.run)
			if !errors.Is(err, attempt.want) {
				t.Errorf("%s against another tenant's data = %v, want %v", attempt.name, err, attempt.want)
			}
			if errors.Is(err, iam.ErrTenantScopeMissing) {
				t.Errorf("%s reported a missing scope, which it is not: the scope was set and the row is simply not ours", attempt.name)
			}
		})
	}

	// Tenant B's user must be untouched and tenant A must still hold exactly one.
	intact := transactor.WithTenant(ctx, tenantB, func(ctx context.Context) error {
		got, err := repo.ByID(ctx, userB.ID)
		if err != nil {
			return err
		}
		if got.FullName != userB.FullName {
			t.Errorf("tenant B's user full name = %q, want %q", got.FullName, userB.FullName)
		}
		users, err := repo.List(ctx, 100, 0)
		if err != nil {
			return err
		}
		if len(users) != 1 {
			t.Errorf("tenant B holds %d users after the cross-tenant attempts, want 1", len(users))
		}
		return nil
	})
	if intact != nil {
		t.Fatalf("cannot verify tenant B after the cross-tenant attempts: %v", intact)
	}

	stillOne := transactor.WithTenant(ctx, tenantA, func(ctx context.Context) error {
		users, err := repo.List(ctx, 100, 0)
		if err != nil {
			return err
		}
		if len(users) != 1 {
			t.Errorf("tenant A holds %d users after the cross-tenant attempts, want 1", len(users))
		}
		return nil
	})
	if stillOne != nil {
		t.Fatalf("cannot verify tenant A after the cross-tenant attempts: %v", stillOne)
	}
}
