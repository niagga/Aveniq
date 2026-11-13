// Package careapp holds the use cases that act on the instance-level rules: which
// patient a caller may see, the clinical grants a treating professional gives, and
// the assignment of the treating professional.
//
// The rules themselves live in domain/care. This package only loads what the rules
// need, asks them, and turns a refusal into the error the API answers with.
package careapp

import (
	"context"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/care"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// Viewer builds the domain viewer from the verified identity. The identity comes
// from the access token; nothing here reads it from a request body.
func Viewer(userID, role string) care.Viewer {
	return care.Viewer{UserID: userID, Role: iam.RoleCode(role)}
}

// Resolved is a patient the viewer may see, with the one fact the rules need that the
// patient row does not carry: whether a live grant exists for this viewer.
type Resolved struct {
	Patient patient.Patient
	Granted bool
}

// Access loads patients for the rules. It must run inside the tenant scope, like every
// repository call.
type Access struct {
	patients port.PatientRepository
	grants   port.ClinicalGrantRepository
}

// NewAccess wires the access helper.
func NewAccess(patients port.PatientRepository, grants port.ClinicalGrantRepository) *Access {
	return &Access{patients: patients, grants: grants}
}

// Resolve loads the patient when the viewer may see it. A patient the viewer may not
// see is patient.ErrNotFound: from the outside it must look like a patient that does
// not exist, or the answer itself tells who is in the practice.
func (a *Access) Resolve(ctx context.Context, who care.Viewer, patientID string) (Resolved, error) {
	subject, err := a.patients.ByID(ctx, patient.ID(patientID))
	if err != nil {
		return Resolved{}, err
	}
	granted, err := a.granted(ctx, who, subject)
	if err != nil {
		return Resolved{}, err
	}
	if !care.SeesPatient(who, subject, granted) {
		return Resolved{}, patient.ErrNotFound
	}
	return Resolved{Patient: subject, Granted: granted}, nil
}

// granted asks for a live grant only when it can change the answer: a professional
// who treats the patient already sees it, and the other roles never hold a grant.
func (a *Access) granted(ctx context.Context, who care.Viewer, subject patient.Patient) (bool, error) {
	if who.Role != iam.RoleProfessional || care.Treats(who, subject) {
		return false, nil
	}
	return a.grants.HasLiveGrant(ctx, string(subject.ID), who.UserID)
}
