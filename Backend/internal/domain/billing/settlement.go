package billing

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

// SettlementID is the opaque identity of a monthly settlement to a mutual.
type SettlementID string

// Settlement is what the practice bills one mutual for one calendar month: the attended sessions
// of its patients, each valued at the mutual's share of the tariff in force that day.
//
// It keeps what was true when it was issued: the mutual's name and tax id, the practice's name,
// and the amount of every session. Void is the only change; it releases the sessions, so they can
// be settled again by a new settlement.
type Settlement struct {
	ID           SettlementID
	TenantID     tenant.ID
	InsurerID    string
	InsurerName  string
	InsurerTaxID string
	PracticeName string

	// PeriodStart and PeriodEnd are the first and the last day of the month, as UTC dates.
	PeriodStart time.Time
	PeriodEnd   time.Time

	Status    Status
	CreatedBy string
	CreatedAt time.Time

	VoidedBy   string
	VoidedAt   *time.Time
	VoidReason *string

	UpdatedAt time.Time

	Items []SettlementItem
}

// SettlementItem is one session of a settlement, with the mutual's share as valued at issue.
type SettlementItem struct {
	AppointmentID string
	PatientID     string
	PatientName   string
	MemberNumber  string

	ProfessionalID   string
	ProfessionalName string
	Modality         string
	StartsAt         time.Time

	Currency    string
	AmountMinor int64

	// CopagoMinor is the patient's copago for the session. It is shown on the settlement and never
	// summed into its totals: the settlement bills the mutual for its share only.
	CopagoMinor int64

	// ReleasedAt is set when the settlement is voided; the session can be settled again then.
	ReleasedAt *time.Time
}

// SettlementInsurer is the mutual a settlement is addressed to. TaxID is nil until the practice
// records the mutual's CUIT, which a settlement needs.
type SettlementInsurer struct {
	ID     string
	Name   string
	TaxID  *string
	Active bool
}

// SettlableSession is an attended session of a mutual's patient, as the settlement reads it before
// valuing it: when, with whom, and the currency its patient is billed in.
type SettlableSession struct {
	AppointmentID    string
	PatientID        string
	PatientName      string
	MemberNumber     string
	ProfessionalID   string
	ProfessionalName string
	Modality         string
	StartsAt         time.Time
	Currency         string
}

// CurrencyTotal is the sum of a settlement's sessions in one currency. Totals are per currency:
// the practice never converts between them (no exchange rate in the MVP).
type CurrencyTotal struct {
	Currency    string
	Sessions    int
	AmountMinor int64
}

var (
	periodPattern = regexp.MustCompile(`^([0-9]{4})-(0[1-9]|1[0-2])$`)
	taxIDPattern  = regexp.MustCompile(`^[0-9]{11}$`)
)

// ParsePeriod reads a calendar month written YYYY-MM and returns its first day, as a UTC date.
func ParsePeriod(value string) (time.Time, error) {
	match := periodPattern.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return time.Time{}, &ValidationError{Field: "period", Message: "must be a month, written YYYY-MM"}
	}
	year, _ := strconv.Atoi(match[1])
	month, _ := strconv.Atoi(match[2])
	return time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC), nil
}

// MonthBounds returns the window of a month in the practice's timezone: from the first instant
// of its first day, up to (not including) the first instant of the next month.
func MonthBounds(firstDay time.Time, location *time.Location) (time.Time, time.Time) {
	from := time.Date(firstDay.Year(), firstDay.Month(), 1, 0, 0, 0, 0, location)
	return from, from.AddDate(0, 1, 0)
}

// NewSettlement validates and builds an issued settlement. The items are the sessions the
// application valued; the settlement refuses an empty list, because a settlement with nothing to
// pay is not a document.
func NewSettlement(
	id SettlementID,
	tenantID tenant.ID,
	actorID string,
	practiceName string,
	insurer SettlementInsurer,
	periodStart time.Time,
	items []SettlementItem,
	now time.Time,
) (Settlement, error) {
	if id == "" {
		return Settlement{}, &ValidationError{Field: "id", Message: "must not be empty"}
	}
	if tenantID == "" {
		return Settlement{}, &ValidationError{Field: "tenant_id", Message: "must not be empty"}
	}
	if isBlank(actorID) {
		return Settlement{}, &ValidationError{Field: "created_by", Message: "must not be empty"}
	}
	practice := strings.TrimSpace(practiceName)
	if practice == "" {
		return Settlement{}, &ValidationError{Field: "practice_name", Message: "must not be blank"}
	}
	if isBlank(insurer.Name) {
		return Settlement{}, &ValidationError{Field: "insurer_name", Message: "must not be blank"}
	}
	if insurer.TaxID == nil || !taxIDPattern.MatchString(*insurer.TaxID) {
		return Settlement{}, &ValidationError{Field: "tax_id", Message: "the mutual needs its 11-digit CUIT before it can be settled"}
	}
	if periodStart.Day() != 1 {
		return Settlement{}, &ValidationError{Field: "period", Message: "must start on the first day of a month"}
	}
	if len(items) == 0 {
		return Settlement{}, &ValidationError{Field: "period", Message: "has no attended session to settle"}
	}

	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if err := item.validate(); err != nil {
			return Settlement{}, err
		}
		if _, dup := seen[item.AppointmentID]; dup {
			return Settlement{}, &ValidationError{Field: "appointment_id", Message: "a session is settled once per settlement"}
		}
		seen[item.AppointmentID] = struct{}{}
	}

	when := stored(now)
	return Settlement{
		ID:           id,
		TenantID:     tenantID,
		InsurerID:    insurer.ID,
		InsurerName:  strings.TrimSpace(insurer.Name),
		InsurerTaxID: *insurer.TaxID,
		PracticeName: practice,
		PeriodStart:  periodStart.UTC(),
		PeriodEnd:    periodStart.AddDate(0, 1, -1).UTC(),
		Status:       StatusIssued,
		CreatedBy:    strings.TrimSpace(actorID),
		CreatedAt:    when,
		UpdatedAt:    when,
		Items:        items,
	}, nil
}

func (item SettlementItem) validate() error {
	if isBlank(item.AppointmentID) {
		return &ValidationError{Field: "appointment_id", Message: "must not be empty"}
	}
	if isBlank(item.MemberNumber) {
		return &ValidationError{Field: "member_number", Message: "must not be blank"}
	}
	if !isISOCurrency(item.Currency) {
		return &ValidationError{Field: "currency", Message: "must be a three-letter ISO code"}
	}
	if item.AmountMinor <= 0 {
		return &ValidationError{Field: "amount_minor", Message: "must be greater than zero"}
	}
	if item.CopagoMinor < 0 {
		return &ValidationError{Field: "copago_minor", Message: "must not be negative"}
	}
	return nil
}

// Void voids an issued settlement and releases its sessions. The void is final: the settlement
// keeps its items and its snapshot, and the sessions become billable by a new settlement.
func (s Settlement) Void(voidedBy, reason string, now time.Time) (Settlement, error) {
	if s.Status == StatusVoided {
		return Settlement{}, ErrAlreadyVoided
	}
	if s.Status != StatusIssued {
		return Settlement{}, ErrImmutable
	}
	if isBlank(voidedBy) {
		return Settlement{}, &ValidationError{Field: "voided_by", Message: "must not be empty"}
	}
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return Settlement{}, &ValidationError{Field: "void_reason", Message: "must not be blank"}
	}
	when := stored(now)
	if when.Before(s.CreatedAt) {
		return Settlement{}, &ValidationError{Field: "voided_at", Message: "must not be before created_at"}
	}

	s.Status = StatusVoided
	s.VoidedBy = strings.TrimSpace(voidedBy)
	s.VoidedAt = &when
	s.VoidReason = &trimmed
	s.UpdatedAt = when

	released := make([]SettlementItem, len(s.Items))
	for i, item := range s.Items {
		item.ReleasedAt = &when
		released[i] = item
	}
	s.Items = released
	return s, nil
}

// Totals sums the sessions per currency, in currency order.
func (s Settlement) Totals() []CurrencyTotal {
	byCurrency := map[string]*CurrencyTotal{}
	for _, item := range s.Items {
		total, ok := byCurrency[item.Currency]
		if !ok {
			total = &CurrencyTotal{Currency: item.Currency}
			byCurrency[item.Currency] = total
		}
		total.Sessions++
		total.AmountMinor += item.AmountMinor
	}
	out := make([]CurrencyTotal, 0, len(byCurrency))
	for _, total := range byCurrency {
		out = append(out, *total)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Currency < out[j].Currency })
	return out
}
