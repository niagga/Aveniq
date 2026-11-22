package landing_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/landing"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

var (
	practice  = tenant.ID("11111111-1111-4111-8111-111111111111")
	writtenAt = time.Date(2026, 11, 22, 18, 0, 0, 0, time.UTC)
)

func validDraft() landing.Draft {
	return landing.Draft{
		DisplayName: "Consultorio Sur",
		Tagline:     "Psicología clínica para adultos",
		About:       "Atendemos con turnos presenciales y online.",
		Address:     "Av. Siempre Viva 742",
		Phone:       "+54 11 4444-5555",
		Email:       "hola@sur.test",
		Schedule:    "Lunes a viernes, 9 a 18 h",
	}
}

func validationField(t *testing.T, err error) string {
	t.Helper()
	var validation *landing.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want a ValidationError", err)
	}
	return validation.Field
}

func TestANewLandingTrimsEveryFieldAndKeepsItsPractice(t *testing.T) {
	draft := validDraft()
	draft.DisplayName = "  Consultorio Sur  "
	draft.Tagline = "   "

	subject, err := landing.New(practice, draft, writtenAt)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if subject.TenantID != practice || subject.DisplayName != "Consultorio Sur" || subject.Tagline != "" {
		t.Fatalf("New did not trim or did not keep the practice: %+v", subject)
	}
	if !subject.UpdatedAt.Equal(writtenAt) {
		t.Fatalf("UpdatedAt = %v, want %v", subject.UpdatedAt, writtenAt)
	}
}

func TestOnlyTheNameIsRequired(t *testing.T) {
	_, err := landing.New(practice, landing.Draft{DisplayName: "Consultorio Sur"}, writtenAt)
	if err != nil {
		t.Fatalf("a landing with only a name must be valid: %v", err)
	}

	_, err = landing.New(practice, landing.Draft{DisplayName: "   "}, writtenAt)
	if got := validationField(t, err); got != "displayName" {
		t.Fatalf("blank name refused on %q, want displayName", got)
	}
}

func TestEveryLimitIsCountedInCharactersNotBytes(t *testing.T) {
	// "ñ" is two bytes: a limit counted in bytes would refuse 60 of them, a limit in characters
	// accepts exactly 60.
	// The phone takes digits: it has its own character rule, which a letter would trip first.
	cases := []struct {
		name   string
		field  string
		filler string
		draft  landing.Draft
		limit  int
		set    func(*landing.Draft, string)
	}{
		{"name", "displayName", "ñ", landing.Draft{}, 120, func(d *landing.Draft, v string) { d.DisplayName = v }},
		{"tagline", "tagline", "ñ", landing.Draft{DisplayName: "x"}, 160, func(d *landing.Draft, v string) { d.Tagline = v }},
		{"about", "about", "ñ", landing.Draft{DisplayName: "x"}, 2000, func(d *landing.Draft, v string) { d.About = v }},
		{"address", "address", "ñ", landing.Draft{DisplayName: "x"}, 200, func(d *landing.Draft, v string) { d.Address = v }},
		{"phone", "phone", "1", landing.Draft{DisplayName: "x"}, 40, func(d *landing.Draft, v string) { d.Phone = v }},
		{"schedule", "schedule", "ñ", landing.Draft{DisplayName: "x"}, 300, func(d *landing.Draft, v string) { d.Schedule = v }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			atLimit := tc.draft
			tc.set(&atLimit, strings.Repeat(tc.filler, tc.limit))
			if _, err := landing.New(practice, atLimit, writtenAt); err != nil {
				t.Fatalf("a value of exactly %d characters must be accepted: %v", tc.limit, err)
			}

			overLimit := tc.draft
			tc.set(&overLimit, strings.Repeat(tc.filler, tc.limit+1))
			_, err := landing.New(practice, overLimit, writtenAt)
			if got := validationField(t, err); got != tc.field {
				t.Fatalf("over the limit refused on %q, want %q", got, tc.field)
			}
		})
	}
}

func TestTheEmailIsABareAddressOrNothing(t *testing.T) {
	for _, email := range []string{"Ana <ana@sur.test>", "no-es-un-correo", "a@b@c"} {
		draft := validDraft()
		draft.Email = email
		_, err := landing.New(practice, draft, writtenAt)
		if got := validationField(t, err); got != "email" {
			t.Fatalf("email %q refused on %q, want email", email, got)
		}
	}

	draft := validDraft()
	draft.Email = ""
	if _, err := landing.New(practice, draft, writtenAt); err != nil {
		t.Fatalf("an empty email is allowed: %v", err)
	}
}

func TestThePhoneHoldsANumberNotANote(t *testing.T) {
	draft := validDraft()
	draft.Phone = "llamar a la secretaria"
	_, err := landing.New(practice, draft, writtenAt)
	if got := validationField(t, err); got != "phone" {
		t.Fatalf("a phone with letters refused on %q, want phone", got)
	}

	draft.Phone = "+54 (11) 4444.5555"
	if _, err := landing.New(practice, draft, writtenAt); err != nil {
		t.Fatalf("a number with punctuation is a phone: %v", err)
	}
}

func TestChangeReplacesTheTextAndKeepsThePractice(t *testing.T) {
	subject, err := landing.New(practice, validDraft(), writtenAt)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	later := writtenAt.Add(time.Hour)

	changed, err := subject.Change(landing.Draft{DisplayName: "Consultorio Norte"}, later)
	if err != nil {
		t.Fatalf("Change: %v", err)
	}
	if changed.TenantID != practice || changed.DisplayName != "Consultorio Norte" || changed.Email != "" {
		t.Fatalf("Change must replace every field from the new draft: %+v", changed)
	}
	if !changed.UpdatedAt.Equal(later) {
		t.Fatalf("Change must move UpdatedAt to the moment of the change")
	}
	if subject.DisplayName != "Consultorio Sur" {
		t.Fatalf("Change must not touch the value it was called on")
	}
}

func TestTheDefaultLandingIsThePracticeNameAndNothingElse(t *testing.T) {
	subject := landing.Default(practice, "  Consultorio Sur  ")
	if subject.TenantID != practice || subject.DisplayName != "Consultorio Sur" {
		t.Fatalf("Default = %+v", subject)
	}
	if subject.Tagline != "" || subject.Email != "" || subject.Phone != "" {
		t.Fatalf("Default must not invent any text: %+v", subject)
	}
}

func TestReconstituteRefusesStoredTextThatBreaksTheRules(t *testing.T) {
	_, err := landing.Reconstitute(practice, landing.Draft{DisplayName: ""}, writtenAt)
	if got := validationField(t, err); got != "displayName" {
		t.Fatalf("stored blank name accepted or refused on %q", got)
	}

	subject, err := landing.Reconstitute(practice, validDraft(), writtenAt)
	if err != nil || subject.Email != "hola@sur.test" {
		t.Fatalf("a stored valid landing must rebuild: %v", err)
	}
}

func TestANewLandingNeedsItsPractice(t *testing.T) {
	_, err := landing.New("", validDraft(), writtenAt)
	if got := validationField(t, err); got != "tenantId" {
		t.Fatalf("empty practice refused on %q, want tenantId", got)
	}
}
