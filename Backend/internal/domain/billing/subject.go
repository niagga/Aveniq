package billing

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
