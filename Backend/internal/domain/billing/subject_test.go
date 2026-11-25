package billing_test

import (
	"testing"

	"github.com/niagga/aveniq/backend/internal/domain/billing"
)

// The buyer of a voucher is the client that pays (S26, T73): Stamp copies the client's
// data and its identity onto the draft, never the patient's and never the request's.
func TestTheBuyerStampsItsData(t *testing.T) {
	name := "Estudio Pérez S.A."
	tax := "30-61234567-2"
	condition := "responsable_inscripto"
	currency := "EUR"

	stamped := billing.Buyer{
		ClientID:        "3e5f6a7b-0000-4000-8000-000000000003",
		Name:            name,
		TaxID:           &tax,
		FiscalCondition: &condition,
		BillingCurrency: currency,
	}.Stamp(billing.IssueDraft{Currency: "ARS"})

	if stamped.ClientID != "3e5f6a7b-0000-4000-8000-000000000003" {
		t.Errorf("ClientID = %q, want the payer's", stamped.ClientID)
	}
	if stamped.BuyerName == nil || *stamped.BuyerName != name {
		t.Errorf("BuyerName = %v, want the client's name", stamped.BuyerName)
	}
	if stamped.BuyerTaxID == nil || *stamped.BuyerTaxID != tax {
		t.Errorf("BuyerTaxID = %v, want the client's", stamped.BuyerTaxID)
	}
	if stamped.BuyerFiscalCondition == nil || *stamped.BuyerFiscalCondition != condition {
		t.Errorf("BuyerFiscalCondition = %v, want the client's", stamped.BuyerFiscalCondition)
	}
}

// A document is compared without its separators and without its case, and a fragment
// too short to be a document search is not used for one.
func TestIdentifierFragment(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"digits as typed", "30111222", "30111222"},
		{"dots are dropped", "30.111.222", "30111222"},
		{"a tax id with dashes", "20-12345678-6", "20123456786"},
		{"spaces are dropped", "20 12345678", "2012345678"},
		{"letters go to upper case", "x-1234567-l", "X1234567L"},
		{"a partial fragment keeps what it has", "111.2", "1112"},
		{"two characters are too short", "30", ""},
		{"separators alone are nothing", "..-", ""},
		{"a wildcard is not a character", "%%%", ""},
		{"a wildcard between digits is dropped", "30%11", "3011"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := billing.IdentifierFragment(tc.in); got != tc.want {
				t.Fatalf("IdentifierFragment(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
