package patient

import (
	"fmt"
	"regexp"
	"strings"
)

// Country codes are ISO 3166-1 alpha-2: two uppercase letters.
var countryPattern = regexp.MustCompile(`^[A-Z]{2}$`)

// DocumentKind is one row of the country's document catalog.
//
// It travels as a value so the domain can validate without reading a table and
// without knowing which countries exist. The catalog is the source of truth; this
// struct is only its shape.
type DocumentKind struct {
	Country    string
	Code       string
	Label      string
	Pattern    string
	Normalizer string
	// Checksum names a strategy from the registry below. Empty means the document
	// has no check digit worth computing (a passport, for instance).
	Checksum string
}

// Document is a personal identity document in canonical form.
//
// Number keeps what the person typed, because that is what a receptionist reads
// back over the phone; Normalized is what the database compares, so the same
// document typed with or without separators is the same person.
type Document struct {
	Country    string
	Kind       string
	Number     string
	Normalized string
}

// TaxID is a fiscal identifier in canonical form. Its country is the patient's:
// one country per patient keeps the rules of the document and of the invoice
// pointing at the same place.
type TaxID struct {
	Kind       string
	Number     string
	Normalized string
}

// NormalizeDocument converts a typed document into its canonical form and
// validates it against the rules of its country.
func NormalizeDocument(kind DocumentKind, raw string) (Document, error) {
	normalized, err := normalizeAndValidate(kind, raw)
	if err != nil {
		return Document{}, err
	}

	return Document{
		Country:    kind.Country,
		Kind:       kind.Code,
		Number:     strings.TrimSpace(raw),
		Normalized: normalized,
	}, nil
}

// NormalizeTaxID is the same pipeline for a fiscal identifier.
func NormalizeTaxID(kind DocumentKind, raw string) (TaxID, error) {
	normalized, err := normalizeAndValidate(kind, raw)
	if err != nil {
		return TaxID{}, err
	}

	return TaxID{
		Kind:       kind.Code,
		Number:     strings.TrimSpace(raw),
		Normalized: normalized,
	}, nil
}

// normalizeAndValidate applies the normalizer, the shape pattern and the check
// digit, in that order. Normalizing first is what makes a document typed with
// dots match one typed without them.
func normalizeAndValidate(kind DocumentKind, raw string) (string, error) {
	if kind.Country == "" || kind.Code == "" {
		return "", fmt.Errorf("%w: the document kind is incomplete", ErrDocumentInvalid)
	}

	normalized, err := normalize(kind, raw)
	if err != nil {
		return "", err
	}
	if normalized == "" {
		return "", &ValidationError{Field: "document_number", Message: "must not be blank"}
	}

	pattern, err := regexp.Compile(kind.Pattern)
	if err != nil {
		// A broken pattern in the catalog is a deployment defect, not a request
		// problem: fail closed rather than accept anything.
		return "", fmt.Errorf("%w: the pattern for %s/%s does not compile: %v", ErrDocumentInvalid, kind.Country, kind.Code, err)
	}
	if !pattern.MatchString(normalized) {
		return "", fmt.Errorf("%w: %q is not a valid %s for %s", ErrDocumentInvalid, raw, kind.Code, kind.Country)
	}

	if kind.Checksum == "" {
		return normalized, nil
	}

	check, known := checksums[kind.Checksum]
	if !known {
		// An unknown strategy is a catalog defect. Silently skipping it would make
		// every document of that kind pass while looking validated.
		return "", fmt.Errorf("%w: the checksum %q has no implementation", ErrDocumentInvalid, kind.Checksum)
	}
	if err := check(normalized); err != nil {
		return "", err
	}

	return normalized, nil
}

// normalize applies the catalog's normalizer.
func normalize(kind DocumentKind, raw string) (string, error) {
	switch kind.Normalizer {
	case "digits":
		return onlyDigits(raw), nil
	case "alnum_upper":
		return onlyAlphanumeric(strings.ToUpper(raw)), nil
	case "as_is":
		return strings.TrimSpace(raw), nil
	default:
		return "", fmt.Errorf("%w: unknown normalizer %q", ErrDocumentInvalid, kind.Normalizer)
	}
}

func onlyDigits(raw string) string {
	var builder strings.Builder
	builder.Grow(len(raw))
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func onlyAlphanumeric(raw string) string {
	var builder strings.Builder
	builder.Grow(len(raw))
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

// ValidCountry reports whether a country code is well formed. Whether the country
// exists in the catalog is a storage question, not a domain one.
func ValidCountry(code string) bool {
	return countryPattern.MatchString(code)
}
