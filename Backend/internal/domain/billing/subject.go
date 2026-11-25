package billing

import "strings"

// Subject is a patient seen from billing: only the identity the roster search answers
// with. It is not the patient record. The accountant reads subjects and never the
// roster (L46), so the clinical and contact fields do not travel here.
//
// The buyer no longer comes from here: since S26/T73 the payer is the client, stamped
// from Buyer. The patient is the search key; the row's tax id and currency are the
// mirror the patient write keeps until the payer data is read everywhere.
type Subject struct {
	PatientID string
	// ClientID is the payer of this patient (S26): the client whose data a receipt
	// stamps. Every patient of the practice has exactly one (0026).
	ClientID        string
	FullName        string
	DocumentNumber  *string
	TaxID           *string
	FiscalCondition *string
	// BillingCurrency is the patient row's billing currency. A voucher is issued in
	// the payer's currency, which the issue use case reads from the client.
	BillingCurrency string
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

// Buyer is who pays, as billing sees it: the client of the patient, resolved by the
// application from the patient's payer (S26, T73). A receipt is never issued in a name
// the caller chose: the only buying data a draft accepts arrives stamped from a client
// resolved inside the practice.
type Buyer struct {
	// ClientID is recorded on the voucher so old receipts keep pointing at the client
	// they were issued for. It is present exactly when the voucher has a patient.
	ClientID        string
	Name            string
	TaxID           *string
	FiscalCondition *string
	// BillingCurrency is the payer's currency: the currency the voucher is issued in.
	BillingCurrency string
}

// Stamp copies the buyer onto a draft. The buyer comes from the client record, never
// from the request body: a caller that could type the buyer could issue a receipt in
// somebody else's name.
func (b Buyer) Stamp(draft IssueDraft) IssueDraft {
	name := b.Name
	draft.ClientID = b.ClientID
	draft.BuyerName = &name
	draft.BuyerTaxID = b.TaxID
	draft.BuyerFiscalCondition = b.FiscalCondition
	return draft
}
