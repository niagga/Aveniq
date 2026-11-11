package patientapp

import (
	"context"
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

// ProfileInput is the personal and fiscal data a caller chooses for a patient.
// It is shared by create and update on purpose: an edit cannot offer fields the
// registration does not, or the other way around. Every optional field is a
// plain string where empty means absent — the aggregate turns blank optional
// text into a real "absent".
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

	// TaxIDKind and TaxIDNumber are an optional pair, like the document.
	TaxIDKind   string
	TaxIDNumber string

	FiscalCondition     string
	FiscalAddress       string
	AdministrativeNotes string
}

// normalizeProfile resolves the country (the request's, or the practice's via the
// catalog), turns the typed document and tax id into canonical form under the
// rules of that country, and confirms the fiscal condition exists for it.
//
// The order is deliberate and matters:
//
//   - the country is resolved first, because every later lookup is keyed by it;
//   - documents and tax ids are normalized through the catalog, which is global
//     reference data with no row-level security, so this must run BEFORE the
//     tenant transaction opens;
//   - the aggregate must already be valid when the write begins: a request the
//     domain would refuse never reaches the database.
//
// Catalog errors pass through untouched: the adapter reports an unknown kind or
// condition as patient.ErrDocumentInvalid, which is exactly what the caller did
// wrong, and a domain *patient.ValidationError arrives unswallowed so the HTTP
// layer can answer 422 with a field.
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

	taxID, err := normalizeTaxID(ctx, catalog, country, in)
	if err != nil {
		return patient.Draft{}, err
	}

	if in.FiscalCondition != "" {
		if _, err := catalog.FiscalCondition(ctx, country, in.FiscalCondition); err != nil {
			return patient.Draft{}, err
		}
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
		TaxID:                 taxID,
		FiscalCondition:       &in.FiscalCondition,
		FiscalAddress:         &in.FiscalAddress,
		AdministrativeNotes:   &in.AdministrativeNotes,
	}, nil
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

// normalizeTaxID is the same pipeline for the fiscal identifier.
func normalizeTaxID(ctx context.Context, catalog port.DocumentCatalog, country string, in ProfileInput) (*patient.TaxID, error) {
	if in.TaxIDKind == "" && in.TaxIDNumber == "" {
		return nil, nil
	}
	if in.TaxIDKind == "" {
		return nil, &patient.ValidationError{Field: "tax_id_kind", Message: "is required when a tax id number is given"}
	}
	if strings.TrimSpace(in.TaxIDNumber) == "" {
		return nil, &patient.ValidationError{Field: "tax_id_number", Message: "must not be blank"}
	}

	kind, err := catalog.TaxIDKind(ctx, country, in.TaxIDKind)
	if err != nil {
		return nil, err
	}
	taxID, err := patient.NormalizeTaxID(kind, in.TaxIDNumber)
	if err != nil {
		return nil, err
	}
	return &taxID, nil
}
