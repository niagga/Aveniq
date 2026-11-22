// Package patientapp holds the patient use cases: the application layer between
// the HTTP handlers and the patient aggregate. It orchestrates domain rules and
// port contracts, and owns transaction-level decisions. It must never import
// infrastructure.
package patientapp

import (
	"context"
	"errors"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/client"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// timeLayout matches tenantapp.TenantView: RFC 3339, always UTC.
const timeLayout = time.RFC3339

// DocumentView is the read model of an identity document: the number as the
// person typed it, plus the kind and country it was validated against.
type DocumentView struct {
	Kind    string
	Number  string
	Country string
}

// PatientView is the read model the application layer exposes to delivery. It is
// intentionally a flat, string-based shape: transports should not have to know
// domain types, and the domain should not have to know JSON. Optional data is nil
// or the empty string; "absent" and "empty" are one state in the aggregate.
//
// Fiscal data and billing currency live on the billing client (T72), not here.
type PatientView struct {
	ID                     string
	TenantID               string
	ClientID               string
	ClientDisplayName      string
	Country                string
	FullName               string
	BirthDate              string // RFC 3339, empty when absent
	Document               *DocumentView
	Phone                  string
	Email                  string
	EmergencyContactName   string
	EmergencyContactPhone  string
	AdministrativeNotes    string
	TreatingProfessionalID string // empty until an owner or admin assigns one
	CoverageKind           string // "private" or "insurer"
	InsurerID              string // empty for a private patient
	MemberNumber           string // empty for a private patient
	IsActive               bool
	CreatedAt              string // RFC 3339, always UTC
	UpdatedAt              string // RFC 3339, always UTC
}

func toView(subject patient.Patient, payer *client.Client) PatientView {
	view := PatientView{
		ID:                     string(subject.ID),
		TenantID:               string(subject.TenantID),
		ClientID:               subject.ClientID,
		Country:                subject.Country,
		FullName:               subject.FullName,
		BirthDate:              formatTime(subject.BirthDate),
		Document:               toDocumentView(subject.Document),
		Phone:                  deref(subject.Phone),
		Email:                  deref(subject.Email),
		EmergencyContactName:   deref(subject.EmergencyContactName),
		EmergencyContactPhone:  deref(subject.EmergencyContactPhone),
		AdministrativeNotes:    deref(subject.AdministrativeNotes),
		TreatingProfessionalID: deref(subject.TreatingProfessionalID),
		CoverageKind:           string(subject.CoverageKind),
		InsurerID:              deref(subject.InsurerID),
		MemberNumber:           deref(subject.MemberNumber),
		IsActive:               subject.IsActive,
		CreatedAt:              subject.CreatedAt.UTC().Format(timeLayout),
		UpdatedAt:              subject.UpdatedAt.UTC().Format(timeLayout),
	}
	if payer != nil {
		view.ClientDisplayName = payer.DisplayName
	}
	return view
}

func toDocumentView(document *patient.Document) *DocumentView {
	if document == nil {
		return nil
	}
	return &DocumentView{
		Kind:    document.Kind,
		Number:  document.Number,
		Country: document.Country,
	}
}

func formatTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(timeLayout)
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func viewWithClient(ctx context.Context, clients port.ClientRepository, subject patient.Patient) (PatientView, error) {
	if subject.ClientID == "" {
		return toView(subject, nil), nil
	}
	payer, err := clients.ByID(ctx, client.ID(subject.ClientID))
	if err != nil && !errors.Is(err, client.ErrNotFound) {
		return PatientView{}, err
	}
	if errors.Is(err, client.ErrNotFound) {
		return toView(subject, nil), nil
	}
	return toView(subject, &payer), nil
}