package patient_test

import (
	"errors"
	"testing"

	"github.com/niagga/aveniq/backend/internal/domain/patient"
)

func kind(country, code, pattern, normalizer, checksum string) patient.DocumentKind {
	return patient.DocumentKind{
		Country:    country,
		Code:       code,
		Label:      code,
		Pattern:    pattern,
		Normalizer: normalizer,
		Checksum:   checksum,
	}
}

func TestNormalizeDocumentSeparators(t *testing.T) {
	dni := kind("AR", "dni", `^[0-9]{8}$`, "digits", "")

	for _, typed := range []string{"30111222", "30.111.222", "30-111-222", " 30 111 222 "} {
		got, err := patient.NormalizeDocument(dni, typed)
		if err != nil {
			t.Fatalf("%q: unexpected error: %v", typed, err)
		}
		if got.Normalized != "30111222" {
			t.Errorf("%q normalized to %q, want 30111222", typed, got.Normalized)
		}
		if got.Number != trim(typed) {
			t.Errorf("%q: Number = %q, want what the person typed", typed, got.Number)
		}
	}
}

// TestNormalizeDocumentIsCaseInsensitiveForAlphanumeric is the case T20b could not
// catch from the schema: the unique index compares the normalized form, so an NIE
// typed in lower case has to normalize to the same value as one typed in upper
// case, or the same person can be registered twice.
func TestNormalizeDocumentIsCaseInsensitiveForAlphanumeric(t *testing.T) {
	nie := kind("ES", "nie", `^[XYZ][0-9]{7}[A-Z]$`, "alnum_upper", "es_nie")

	upper, err := patient.NormalizeDocument(nie, "X1234567L")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lower, err := patient.NormalizeDocument(nie, "x-1234567-l")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if lower.Normalized != upper.Normalized {
		t.Fatalf("normalized values differ: %q vs %q", lower.Normalized, upper.Normalized)
	}
	if lower.Number != "x-1234567-l" {
		t.Errorf("Number = %q, want the typed form", lower.Number)
	}
}

func TestDocumentPatternRejects(t *testing.T) {
	nie := kind("ES", "nie", `^[XYZ][0-9]{7}[A-Z]$`, "alnum_upper", "es_nie")

	// A blank number is a field-level problem the API answers 422 for, not a
	// country rule. Keeping the two apart is what lets a form point at the input.
	for _, blank := range []string{"", "   "} {
		var validation *patient.ValidationError
		if _, err := patient.NormalizeDocument(nie, blank); !errors.As(err, &validation) {
			t.Fatalf("%q: error = %v, want a ValidationError", blank, err)
		}
	}

	for _, typed := range []string{"12345678Z", "X123456L", "X12345678L"} {
		if _, err := patient.NormalizeDocument(nie, typed); !errors.Is(err, patient.ErrDocumentInvalid) {
			t.Fatalf("%q: error = %v, want %v", typed, err, patient.ErrDocumentInvalid)
		}
	}
}

func TestChecksums(t *testing.T) {
	for name, tc := range map[string]struct {
		kind  patient.DocumentKind
		valid []string
		bad   []string
	}{
		"AR CUIT": {
			kind:  kind("AR", "cuit", `^[0-9]{11}$`, "digits", "ar_cuit"),
			valid: []string{"20-12345678-6", "20123456786"},
			bad:   []string{"20123456780", "20123456788"},
		},
		"BR CPF": {
			kind:  kind("BR", "cpf", `^[0-9]{11}$`, "digits", "br_cpf"),
			valid: []string{"529.982.247-25"},
			bad:   []string{"52998224724", "11111111111"},
		},
		"BR CNPJ": {
			kind:  kind("BR", "cnpj", `^[0-9]{14}$`, "digits", "br_cnpj"),
			valid: []string{"11.222.333/0001-81"},
			bad:   []string{"11222333000180", "00000000000000"},
		},
		"ES DNI": {
			kind:  kind("ES", "dni", `^[0-9]{8}[A-Z]$`, "alnum_upper", "es_dni"),
			valid: []string{"12345678Z", "12345678-z"},
			bad:   []string{"12345678A", "12345678"},
		},
		"ES NIE": {
			kind:  kind("ES", "nie", `^[XYZ][0-9]{7}[A-Z]$`, "alnum_upper", "es_nie"),
			valid: []string{"X1234567L"},
			bad:   []string{"X1234567M", "Y1234567L"},
		},
		"PT NIF": {
			kind:  kind("PT", "nif", `^[0-9]{9}$`, "digits", "pt_nif"),
			valid: []string{"123456789"},
			bad:   []string{"123456788", "123456780"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			for _, valid := range tc.valid {
				if _, err := patient.NormalizeDocument(tc.kind, valid); err != nil {
					t.Errorf("%q must be accepted: %v", valid, err)
				}
			}
			for _, bad := range tc.bad {
				if _, err := patient.NormalizeDocument(tc.kind, bad); !errors.Is(err, patient.ErrDocumentInvalid) {
					t.Errorf("%q = %v, want %v", bad, err, patient.ErrDocumentInvalid)
				}
			}
		})
	}
}

// TestUnknownChecksumStrategyFailsClosed is the guard for the seam: a catalog row
// naming a strategy nobody implemented must refuse documents, not accept them. A
// silent skip would make every document of that kind look validated.
func TestUnknownChecksumStrategyFailsClosed(t *testing.T) {
	exotic := kind("XX", "doc", `^[0-9]{4}$`, "digits", "xx_unknown")

	_, err := patient.NormalizeDocument(exotic, "1234")
	if !errors.Is(err, patient.ErrDocumentInvalid) {
		t.Fatalf("error = %v, want %v", err, patient.ErrDocumentInvalid)
	}
}

func TestShapeOnlyStrategiesAcceptThePattern(t *testing.T) {
	// es_nif and pt_cc are registered as shape-only on purpose: the pattern still
	// refuses garbage, and the missing check digit is written down instead of
	// silently assumed.
	nif := kind("ES", "nif", `^[0-9A-Z][0-9]{7}[0-9A-Z]$`, "alnum_upper", "es_nif")

	if _, err := patient.NormalizeDocument(nif, "b12345678"); err != nil {
		t.Fatalf("a shape-valid NIF must be accepted: %v", err)
	}
	if _, err := patient.NormalizeDocument(nif, "1234"); !errors.Is(err, patient.ErrDocumentInvalid) {
		t.Fatalf("a shape-invalid NIF must be refused, got %v", err)
	}
}

func TestNormalizeTaxID(t *testing.T) {
	cuit := kind("AR", "cuit", `^[0-9]{11}$`, "digits", "ar_cuit")

	taxID, err := patient.NormalizeTaxID(cuit, "20-12345678-6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if taxID.Kind != "cuit" || taxID.Normalized != "20123456786" {
		t.Fatalf("taxID = %+v, want kind cuit and the normalized digits", taxID)
	}
	if taxID.Number != "20-12345678-6" {
		t.Errorf("Number = %q, want the typed form", taxID.Number)
	}
}

func trim(value string) string {
	start, end := 0, len(value)
	for start < end && value[start] == ' ' {
		start++
	}
	for end > start && value[end-1] == ' ' {
		end--
	}
	return value[start:end]
}
