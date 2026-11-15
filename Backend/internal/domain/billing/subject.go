package billing

import "strings"

// Subject is a patient seen from billing: only the identity the receipt prints and
// the fiscal data the buyer needs. It is not the patient record. The accountant
// reads subjects and never the roster (L46), so the clinical and contact fields do
// not travel here.
type Subject struct {
	PatientID       string
	FullName        string
	DocumentNumber  *string
	TaxID           *string
	FiscalCondition *string
}

// MinIdentifierLength is how many letters or digits a fragment needs before it is also
// matched against documents and tax ids. Below it the fragment would match a large
// share of the roster, and the search is not a roster.
const MinIdentifierLength = 3

// IdentifierFragment reduces what the person typed to the form documents are compared
// in: letters and digits, upper case, nothing else. "20-12345678" and "20 12345678"
// both become "2012345678", so a search finds a document however it was written when
// it was saved. It returns "" when the fragment is too short to search identifiers by.
func IdentifierFragment(fragment string) string {
	var reduced strings.Builder
	reduced.Grow(len(fragment))
	for _, r := range strings.ToUpper(fragment) {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'Z':
			reduced.WriteRune(r)
		}
	}
	if reduced.Len() < MinIdentifierLength {
		return ""
	}
	return reduced.String()
}

// ApplyBuyer stamps the buyer snapshot onto a draft. The buyer comes from the
// patient record, never from the request body: a caller that could type the buyer
// could issue a receipt in somebody else's name.
func (s Subject) ApplyBuyer(draft IssueDraft) IssueDraft {
	name := s.FullName
	draft.PatientID = s.PatientID
	draft.BuyerName = &name
	draft.BuyerTaxID = s.TaxID
	draft.BuyerFiscalCondition = s.FiscalCondition
	return draft
}
