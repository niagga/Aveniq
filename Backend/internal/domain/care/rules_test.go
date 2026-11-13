package care_test

import (
	"errors"
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/care"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

const (
	treatingID = "11111111-1111-4111-8111-111111111111"
	otherID    = "22222222-2222-4222-8222-222222222222"
	ownerID    = "33333333-3333-4333-8333-333333333333"
	assistID   = "44444444-4444-4444-8444-444444444444"
)

// patientTreatedBy builds a patient whose treating professional is the given id, or
// nobody when the id is empty.
func patientTreatedBy(treating string) patient.Patient {
	subject := patient.Patient{ID: "p-1", TenantID: tenant.ID("t-1"), FullName: "Ana", IsActive: true}
	if treating != "" {
		subject = subject.WithTreating(treating)
	}
	return subject
}

func viewer(id string, role iam.RoleCode) care.Viewer {
	return care.Viewer{UserID: id, Role: role}
}

// TestTheTreatingProfessionalAloneReadsTheClinicalRecord is the rule the whole T38 turns
// on: a role may hold clinical:read:own and still not read a patient it does not treat.
func TestTheTreatingProfessionalAloneReadsTheClinicalRecord(t *testing.T) {
	subject := patientTreatedBy(treatingID)

	cases := []struct {
		name    string
		who     care.Viewer
		granted bool
		want    bool
	}{
		{"the treating professional reads", viewer(treatingID, iam.RoleProfessional), false, true},
		{"a professional with a grant reads", viewer(otherID, iam.RoleProfessional), true, true},
		{"a professional without a grant does not read", viewer(otherID, iam.RoleProfessional), false, false},
		{"the owner does not read even holding every permission", viewer(ownerID, iam.RoleOwner), false, false},
		{"the admin does not read", viewer(ownerID, iam.RoleAdmin), false, false},
		{"the assistant never reads", viewer(assistID, iam.RoleAssistant), true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := care.ReadsClinical(tc.who, subject, tc.granted); got != tc.want {
				t.Fatalf("ReadsClinical = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestOnlyTheTreatingProfessionalWritesTheRecord: a grant is read access, never write.
func TestOnlyTheTreatingProfessionalWritesTheRecord(t *testing.T) {
	subject := patientTreatedBy(treatingID)

	if !care.WritesClinical(viewer(treatingID, iam.RoleProfessional), subject) {
		t.Fatal("the treating professional cannot write its record")
	}
	if care.WritesClinical(viewer(otherID, iam.RoleProfessional), subject) {
		t.Fatal("a professional that does not treat the patient can write its record")
	}
	if care.WritesClinical(viewer(ownerID, iam.RoleOwner), subject) {
		t.Fatal("the owner can write a record it does not treat")
	}
}

// TestSeesPatientNarrowsOnlyTheProfessional: the rest of the practice sees every patient;
// a professional sees its own and the granted ones, and nothing else.
func TestSeesPatientNarrowsOnlyTheProfessional(t *testing.T) {
	unassigned := patientTreatedBy("")
	treated := patientTreatedBy(treatingID)

	cases := []struct {
		name    string
		who     care.Viewer
		subject patient.Patient
		granted bool
		want    bool
	}{
		{"the owner sees an unassigned patient", viewer(ownerID, iam.RoleOwner), unassigned, false, true},
		{"the assistant sees a patient treated by someone else", viewer(assistID, iam.RoleAssistant), treated, false, true},
		{"the treating professional sees its patient", viewer(treatingID, iam.RoleProfessional), treated, false, true},
		{"a granted professional sees the patient", viewer(otherID, iam.RoleProfessional), treated, true, true},
		{"a stranger professional does not see the patient", viewer(otherID, iam.RoleProfessional), treated, false, false},
		{"a professional does not see an unassigned patient", viewer(treatingID, iam.RoleProfessional), unassigned, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := care.SeesPatient(tc.who, tc.subject, tc.granted); got != tc.want {
				t.Fatalf("SeesPatient = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestBookingNeedsTheTreatingProfessionalForProfessionals: owner, admin and assistant
// book with anyone; a professional books only with its own patients, not a granted one.
func TestBookingNeedsTheTreatingProfessionalForProfessionals(t *testing.T) {
	subject := patientTreatedBy(treatingID)

	cases := []struct {
		name string
		who  care.Viewer
		want bool
	}{
		{"the owner books with any patient", viewer(ownerID, iam.RoleOwner), true},
		{"the assistant books with any patient", viewer(assistID, iam.RoleAssistant), true},
		{"the treating professional books", viewer(treatingID, iam.RoleProfessional), true},
		{"a professional that does not treat the patient cannot book", viewer(otherID, iam.RoleProfessional), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := care.Books(tc.who, subject); got != tc.want {
				t.Fatalf("Books = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAProfessionalWorksOnlyItsOwnAppointments: the rest of the practice works on all.
func TestAProfessionalWorksOnlyItsOwnAppointments(t *testing.T) {
	if !care.ManagesAppointment(viewer(treatingID, iam.RoleProfessional), treatingID) {
		t.Fatal("a professional cannot manage its own appointment")
	}
	if care.ManagesAppointment(viewer(treatingID, iam.RoleProfessional), otherID) {
		t.Fatal("a professional manages another professional's appointment")
	}
	if !care.ManagesAppointment(viewer(assistID, iam.RoleAssistant), otherID) {
		t.Fatal("the assistant cannot manage any appointment")
	}
}

// TestOnlyTheTreatingProfessionalGrantsAndTheOwnerRevokes: a grant is given by the treating
// professional alone; withdrawing it is also open to the owner, so a grant does not outlive
// a treating professional who was replaced.
func TestOnlyTheTreatingProfessionalGrantsAndTheOwnerRevokes(t *testing.T) {
	subject := patientTreatedBy(treatingID)

	if !care.GrantsClinical(viewer(treatingID, iam.RoleProfessional), subject) {
		t.Fatal("the treating professional cannot grant")
	}
	if care.GrantsClinical(viewer(ownerID, iam.RoleOwner), subject) {
		t.Fatal("the owner grants a record it does not treat")
	}
	if !care.ManagesGrants(viewer(ownerID, iam.RoleOwner), subject) {
		t.Fatal("the owner cannot withdraw a grant")
	}
	if care.ManagesGrants(viewer(otherID, iam.RoleProfessional), subject) {
		t.Fatal("a professional that does not treat the patient manages its grants")
	}
}

func TestEditingAPatientIsForTheTreatingProfessionalOrTheRestOfThePractice(t *testing.T) {
	subject := patientTreatedBy(treatingID)

	if !care.EditsPatient(viewer(ownerID, iam.RoleOwner), subject) {
		t.Fatal("the owner cannot edit a patient")
	}
	if !care.EditsPatient(viewer(treatingID, iam.RoleProfessional), subject) {
		t.Fatal("the treating professional cannot edit its patient")
	}
	if care.EditsPatient(viewer(otherID, iam.RoleProfessional), subject) {
		t.Fatal("a professional edited a patient it does not treat")
	}
}

func TestOnlyAProfessionalGetsTheTreatingOnCreate(t *testing.T) {
	if !care.BecomesTreatingOnCreate(viewer(treatingID, iam.RoleProfessional)) {
		t.Fatal("a professional who registers a patient does not become its treating one")
	}
	if care.BecomesTreatingOnCreate(viewer(assistID, iam.RoleAssistant)) {
		t.Fatal("an assistant who registers a patient becomes its treating one")
	}
}

func TestOwnOnlyIsTheProfessionalsListing(t *testing.T) {
	if !care.OwnOnly(viewer(treatingID, iam.RoleProfessional)) {
		t.Fatal("a professional's listing is not narrowed to its own")
	}
	if care.OwnOnly(viewer(ownerID, iam.RoleOwner)) {
		t.Fatal("the owner's listing is narrowed")
	}
}

// TestAGrantIsRefusedWithoutTreatingAndForASelfGrant covers construction: a grant is
// built only by the treating professional, and never to the professional itself.
func TestAGrantIsRefusedWithoutTreatingAndForASelfGrant(t *testing.T) {
	now := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	subject := patientTreatedBy(treatingID)

	if _, err := care.NewGrant("g-1", subject, viewer(otherID, iam.RoleProfessional), assistID, now); !errors.Is(err, care.ErrForbidden) {
		t.Fatalf("a non-treating professional granted: error = %v, want care.ErrForbidden", err)
	}
	if _, err := care.NewGrant("g-1", subject, viewer(treatingID, iam.RoleProfessional), treatingID, now); err == nil {
		t.Fatal("the treating professional granted the record to itself")
	}

	grant, err := care.NewGrant("g-1", subject, viewer(treatingID, iam.RoleProfessional), otherID, now)
	if err != nil {
		t.Fatalf("the treating professional could not grant: %v", err)
	}
	if !grant.Live() || grant.GrantedBy != treatingID || grant.ProfessionalUserID != otherID {
		t.Fatalf("grant = %+v, want a live grant from the treating professional to the other one", grant)
	}
}

// TestRevokingIsIdempotentAndKeepsTheRow: revoking twice is not an error, and the revoked
// grant stays as history with its instant.
func TestRevokingIsIdempotentAndKeepsTheRow(t *testing.T) {
	now := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	subject := patientTreatedBy(treatingID)
	grant, err := care.NewGrant("g-1", subject, viewer(treatingID, iam.RoleProfessional), otherID, now)
	if err != nil {
		t.Fatal(err)
	}

	revoked, err := grant.Revoke(viewer(treatingID, iam.RoleProfessional), subject, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if revoked.Live() || revoked.RevokedAt == nil {
		t.Fatalf("revoked grant still live: %+v", revoked)
	}

	again, err := revoked.Revoke(viewer(ownerID, iam.RoleOwner), subject, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("a second revoke failed: %v", err)
	}
	if !again.RevokedAt.Equal(*revoked.RevokedAt) {
		t.Fatalf("second revoke moved the instant to %v, want the first %v", again.RevokedAt, revoked.RevokedAt)
	}

	if _, err := grant.Revoke(viewer(otherID, iam.RoleProfessional), subject, now); !errors.Is(err, care.ErrForbidden) {
		t.Fatalf("a stranger revoked a grant: error = %v, want care.ErrForbidden", err)
	}
}
