// Package patientapp holds the patient use cases: the application layer between
// the HTTP handlers and the patient aggregate. It orchestrates domain rules and
// port contracts, and owns transaction-level decisions. It must never import
// infrastructure.
package patientapp

import (
	"time"

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

// TaxIDView is the read model of a fiscal identifier, in the same spirit.
type TaxIDView struct {
	Kind   string
	Number string
}

// PatientView is the read model the application layer exposes to delivery. It is
// intentionally a flat, string-based shape: transports should not have to know
// domain types, and the domain should not have to know JSON. Optional data is nil
// or the empty string; "absent" and "empty" are one state in the aggregate.
type PatientView struct {
	ID                    string
	TenantID              string
	Country               string
	FullName              string
	BirthDate             string // RFC 3339, empty when absent
	Document              *DocumentView
	Phone                 string
	Email                 string
	EmergencyContactName  string
	EmergencyContactPhone string
	TaxID                 *TaxIDView
	FiscalCondition       string
	FiscalAddress         string
	AdministrativeNotes   string
	IsActive              bool
	CreatedAt             string // RFC 3339, always UTC
	UpdatedAt             string // RFC 3339, always UTC
}

func toView(subject patient.Patient) PatientView {
	return PatientView{
		ID:                    string(subject.ID),
		TenantID:              string(subject.TenantID),
		Country:               subject.Country,
		FullName:              subject.FullName,
		BirthDate:             formatTime(subject.BirthDate),
		Document:              toDocumentView(subject.Document),
		Phone:                 deref(subject.Phone),
		Email:                 deref(subject.Email),
		EmergencyContactName:  deref(subject.EmergencyContactName),
		EmergencyContactPhone: deref(subject.EmergencyContactPhone),
		TaxID:                 toTaxIDView(subject.TaxID),
		FiscalCondition:       deref(subject.FiscalCondition),
		FiscalAddress:         deref(subject.FiscalAddress),
		AdministrativeNotes:   deref(subject.AdministrativeNotes),
		IsActive:              subject.IsActive,
		CreatedAt:             subject.CreatedAt.UTC().Format(timeLayout),
		UpdatedAt:             subject.UpdatedAt.UTC().Format(timeLayout),
	}
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

func toTaxIDView(taxID *patient.TaxID) *TaxIDView {
	if taxID == nil {
		return nil
	}
	return &TaxIDView{Kind: taxID.Kind, Number: taxID.Number}
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
