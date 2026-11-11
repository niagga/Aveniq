package patient

import "fmt"

// checksums maps the strategy name stored in the catalog onto its implementation.
//
// This map is the extension seam the whole country model was built around: adding
// a country's check digit is one entry here plus a catalog row, never a branch in
// the aggregate, the repository or a handler.
var checksums = map[string]func(normalized string) error{
	"ar_cuit": checkARCuit,
	"br_cpf":  checkBRCPF,
	"br_cnpj": checkBRCNPJ,
	"es_dni":  checkESDNI,
	"es_nie":  checkESNIE,
	"es_nif":  shapeOnly,
	"pt_nif":  checkPTNIF,
	"pt_cc":   shapeOnly,
}

// malformed wraps a shape problem as an invalid document.
func malformed(kind, value string) error {
	return fmt.Errorf("%w: %q is not a valid %s", ErrDocumentInvalid, value, kind)
}

// shapeOnly is an explicit "not implemented yet", never a silent skip.
//
// The catalog names a strategy for these documents and this entry says the
// strategy is shape-only; the pattern has already refused garbage. Writing it down
// is what lets someone find the gap by searching for it, instead of discovering it
// when a wrong check digit is accepted in production.
func shapeOnly(string) error { return nil }

// checkARCuit validates an Argentine CUIT: ten digits plus a mod-11 check digit
// over the weights 5,4,3,2,7,6,5,4,3,2.
func checkARCuit(normalized string) error {
	if len(normalized) != 11 {
		return malformed("CUIT", normalized)
	}

	weights := [10]int{5, 4, 3, 2, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i, weight := range weights {
		sum += int(normalized[i]-'0') * weight
	}

	check := 11 - sum%11
	switch check {
	case 11:
		check = 0
	case 10:
		// The published rule maps 10 to 9, not to a rejection.
		check = 9
	}

	if int(normalized[10]-'0') != check {
		return malformed("CUIT", normalized)
	}
	return nil
}

// checkBRCPF validates a Brazilian CPF: two mod-11 check digits.
func checkBRCPF(normalized string) error {
	if len(normalized) != 11 {
		return malformed("CPF", normalized)
	}
	// A CPF of one repeated digit is refused by the algorithm's own rule but is a
	// common fixture in test data, so it is rejected explicitly.
	if allSameDigit(normalized) {
		return malformed("CPF", normalized)
	}

	first, err := mod11Digit(normalized[:9], 10)
	if err != nil || first != int(normalized[9]-'0') {
		return malformed("CPF", normalized)
	}
	second, err := mod11Digit(normalized[:10], 11)
	if err != nil || second != int(normalized[10]-'0') {
		return malformed("CPF", normalized)
	}
	return nil
}

// checkBRCNPJ validates a Brazilian CNPJ: fourteen digits, two check digits.
func checkBRCNPJ(normalized string) error {
	if len(normalized) != 14 {
		return malformed("CNPJ", normalized)
	}
	if allSameDigit(normalized) {
		return malformed("CNPJ", normalized)
	}

	firstWeights := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	first, err := weightedMod11Digit(normalized[:12], firstWeights)
	if err != nil || first != int(normalized[12]-'0') {
		return malformed("CNPJ", normalized)
	}

	secondWeights := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	second, err := weightedMod11Digit(normalized[:13], secondWeights)
	if err != nil || second != int(normalized[13]-'0') {
		return malformed("CNPJ", normalized)
	}
	return nil
}

// mod11Digit computes a Brazilian check digit with descending weights.
func mod11Digit(digits string, startWeight int) (int, error) {
	weights := make([]int, 0, len(digits))
	for i := range digits {
		weights = append(weights, startWeight-i)
	}
	return weightedMod11Digit(digits, weights)
}

func weightedMod11Digit(digits string, weights []int) (int, error) {
	if len(digits) != len(weights) {
		return 0, fmt.Errorf("patient: %d digits and %d weights", len(digits), len(weights))
	}

	sum := 0
	for i, weight := range weights {
		digit := int(digits[i] - '0')
		if digit < 0 || digit > 9 {
			return 0, fmt.Errorf("patient: %q is not numeric", digits)
		}
		sum += digit * weight
	}

	rest := sum % 11
	if rest < 2 {
		return 0, nil
	}
	return 11 - rest, nil
}

func allSameDigit(normalized string) bool {
	if normalized == "" {
		return false
	}
	for i := 1; i < len(normalized); i++ {
		if normalized[i] != normalized[0] {
			return false
		}
	}
	return true
}

// dniLetters is the Spanish table: the letter is a function of the number mod 23.
const dniLetters = "TRWAGMYFPDXBNJZSQVHLCKE"

// checkESDNI validates a Spanish DNI: eight digits plus their letter.
func checkESDNI(normalized string) error {
	if len(normalized) != 9 {
		return malformed("DNI", normalized)
	}

	number, err := digitsOf(normalized[:8])
	if err != nil {
		return malformed("DNI", normalized)
	}

	if normalized[8] != dniLetters[number%23] {
		return malformed("DNI", normalized)
	}
	return nil
}

// checkESNIE validates a Spanish NIE: X, Y or Z stand for 0, 1 and 2, and the rest
// behaves exactly like a DNI.
func checkESNIE(normalized string) error {
	if len(normalized) != 9 {
		return malformed("NIE", normalized)
	}

	prefix := 0
	switch normalized[0] {
	case 'X':
		prefix = 0
	case 'Y':
		prefix = 1
	case 'Z':
		prefix = 2
	default:
		return malformed("NIE", normalized)
	}

	rest, err := digitsOf(normalized[1:8])
	if err != nil {
		return malformed("NIE", normalized)
	}

	number := prefix*10_000_000 + rest
	if normalized[8] != dniLetters[number%23] {
		return malformed("NIE", normalized)
	}
	return nil
}

// checkPTNIF validates a Portuguese NIF: eight digits with weights 9..2 and a
// mod-11 check digit.
func checkPTNIF(normalized string) error {
	if len(normalized) != 9 {
		return malformed("NIF", normalized)
	}

	sum := 0
	for i := 0; i < 8; i++ {
		digit := int(normalized[i] - '0')
		if digit < 0 || digit > 9 {
			return malformed("NIF", normalized)
		}
		sum += digit * (9 - i)
	}

	check := 11 - sum%11
	if check >= 10 {
		// 10 and 11 both become 0 by the published rule.
		check = 0
	}

	if int(normalized[8]-'0') != check {
		return malformed("NIF", normalized)
	}
	return nil
}

// digitsOf parses a run of ASCII digits.
func digitsOf(raw string) (int, error) {
	number := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("patient: %q is not numeric", raw)
		}
		number = number*10 + int(r-'0')
	}
	return number, nil
}
