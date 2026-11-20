package patient

// Coverage is who pays for the sessions of a patient (L61).
type Coverage string

const (
	// CoveragePrivate is a particular patient: the patient pays the practice.
	CoveragePrivate Coverage = "private"
	// CoverageInsurer is a patient covered by a mutual of the practice's catalog.
	CoverageInsurer Coverage = "insurer"
)

// Currency is the currency a patient's sessions are billed in. The MVP bills in these
// three and no others; there is no exchange rate (L61).
type Currency string

const (
	CurrencyARS Currency = "ARS"
	CurrencyEUR Currency = "EUR"
	CurrencyUSD Currency = "USD"
)

// ValidCurrency reports whether the currency is one the MVP bills in. The empty
// currency is not valid: a patient is never saved without a choice.
func ValidCurrency(currency Currency) bool {
	switch currency {
	case CurrencyARS, CurrencyEUR, CurrencyUSD:
		return true
	default:
		return false
	}
}
