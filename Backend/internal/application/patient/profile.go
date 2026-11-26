package patientapp

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/insurer"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// ProfileInput is the clinical and administrative data a caller chooses for a patient.
// Fiscal data and billing currency belong on the billing client (T72), not here.
type ProfileInput struct {
	FullName string
	// Country is optional: when empty, the patient inherits the practice's.
	Country string
	// BirthDate is optional; nil means absent.
	BirthDate *time.Time

	// DocumentKind and DocumentNumber are an optional pair: either both are
	// given or neither. The number is what the person typed, separators
	// included.
	DocumentKind   string
	DocumentNumber string

	Phone                 string
	Email                 string
	EmergencyContactName  string
	EmergencyContactPhone string

	AdministrativeNotes string

	// CoverageKind is required: "private" or "insurer". InsurerID and MemberNumber are
	// the mutual and the affiliate number of an insured patient, empty for a private one.
	CoverageKind string
	InsurerID    string
	MemberNumber string
}

// normalizeProfile resolves the country (the request's, or the practice's via the
// catalog), turns the typed document into canonical form under the rules of that
// country, and prepares a patient draft without fiscal data.
func normalizeProfile(ctx context.Context, catalog port.DocumentCatalog, tenantID string, in ProfileInput) (patient.Draft, error) {
	country := in.Country
	if country == "" {
		resolved, err := catalog.TenantCountry(ctx, tenantID)
		if err != nil {
			return patient.Draft{}, err
		}
		country = resolved
	}

	document, err := normalizeDocument(ctx, catalog, country, in)
	if err != nil {
		return patient.Draft{}, err
	}

	return patient.Draft{
		FullName:              in.FullName,
		Country:               country,
		BirthDate:             in.BirthDate,
		Document:              document,
		Phone:                 &in.Phone,
		Email:                 &in.Email,
		EmergencyContactName:  &in.EmergencyContactName,
		EmergencyContactPhone: &in.EmergencyContactPhone,
		AdministrativeNotes:   &in.AdministrativeNotes,
		CoverageKind:          patient.Coverage(in.CoverageKind),
		InsurerID:             &in.InsurerID,
		MemberNumber:          &in.MemberNumber,
	}, nil
}

// requireActiveInsurer confirms a mutual can be chosen for a patient's coverage: it is in
// this practice's catalog and it is active. The catalog is read inside the tenant scope,
// so another practice's mutual is simply not found, and the caller learns only that the
// field is wrong.
func requireActiveInsurer(ctx context.Context, insurers port.InsurerRepository, insurerID string) error {
	mutual, err := insurers.ByID(ctx, insurer.ID(insurerID))
	if errors.Is(err, insurer.ErrNotFound) {
		return &patient.ValidationError{Field: "insurer_id", Message: "must be a mutual of this practice"}
	}
	if err != nil {
		return err
	}
	if !mutual.IsActive {
		return &patient.ValidationError{Field: "insurer_id", Message: "is deactivated: choose an active mutual"}
	}
	return nil
}

// normalizeDocument turns a typed document pair into its canonical form, or nil
// when the caller gave neither part. A half pair is a request defect the
// aggregate cannot express, so it is refused here with the offending field.
func normalizeDocument(ctx context.Context, catalog port.DocumentCatalog, country string, in ProfileInput) (*patient.Document, error) {
	if in.DocumentKind == "" && in.DocumentNumber == "" {
		return nil, nil
	}
	if in.DocumentKind == "" {
		return nil, &patient.ValidationError{Field: "document_kind", Message: "is required when a document number is given"}
	}
	if strings.TrimSpace(in.DocumentNumber) == "" {
		return nil, &patient.ValidationError{Field: "document_number", Message: "must not be blank"}
	}

	kind, err := catalog.DocumentKind(ctx, country, in.DocumentKind)
	if err != nil {
		return nil, err
	}
	document, err := patient.NormalizeDocument(kind, in.DocumentNumber)
	if err != nil {
		return nil, err
	}
	return &document, nil
}