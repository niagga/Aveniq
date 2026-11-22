package http_test

// newPatientCreateBody merges fields into a valid POST /patients payload (T72: fiscal on client).
func newPatientCreateBody(extra map[string]any) map[string]any {
	body := map[string]any{
		"coverageKind": "private",
		"client":       map[string]any{"billingCurrency": "ARS"},
	}
	for key, value := range extra {
		body[key] = value
	}
	return body
}