package http_test

import (
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/niagga/aveniq/backend/internal/domain/iam"
	"github.com/niagga/aveniq/backend/internal/domain/tenant"
)

const landingPath = "/api/v1/practice/landing"

// publicPath asks the public landing for a host, the way the public site does.
func publicPath(host string) string {
	return "/api/v1/public/landing?host=" + url.QueryEscape(host)
}

// publishedDraft is a full landing the owner writes. Its values are distinct from the defaults
// so a test can tell the saved text from the practice name.
func publishedDraft() map[string]any {
	return map[string]any{
		"displayName": "Consultorio Sur",
		"tagline":     "Psicología clínica para adultos",
		"about":       "Atendemos con turnos presenciales y online.",
		"address":     "Av. Siempre Viva 742",
		"phone":       "+54 11 4444-5555",
		"email":       "hola@sur.test",
		"schedule":    "Lunes a viernes, 9 a 18 h",
	}
}

// keysOf lists the data keys of a body. huma adds "$schema", a link to the document of the
// answer, to every object it writes; it is not part of the landing and is left out.
func keysOf(body map[string]any) []string {
	keys := make([]string, 0, len(body))
	for key := range body {
		if key != "$schema" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// TestThePublicLandingAnswersOnlyThePublishedFields is the contract of the public door: the
// visitor reads the text the owner wrote and nothing that identifies the practice internally.
func TestThePublicLandingAnswersOnlyThePublishedFields(t *testing.T) {
	h := newHarness(t)
	practice := h.tenantID(t, "practica-a")
	owner, _ := h.member(t, practice, iam.RoleOwner)

	if status, body := h.do(t, http.MethodPut, landingPath, owner, publishedDraft()); status != http.StatusOK {
		t.Fatalf("save = %d %v, want 200", status, body)
	}

	status, body := h.do(t, http.MethodGet, publicPath("practica-a."+testBaseDomain), "", nil)
	if status != http.StatusOK {
		t.Fatalf("public landing = %d %v, want 200", status, body)
	}

	want := []string{"about", "address", "displayName", "email", "phone", "schedule", "slug", "tagline"}
	if got := keysOf(body); !reflect.DeepEqual(got, want) {
		t.Fatalf("public keys = %v, want exactly %v: an internal field reached the public door", got, want)
	}
	if body["displayName"] != "Consultorio Sur" || body["email"] != "hola@sur.test" || body["slug"] != "practica-a" {
		t.Fatalf("public landing = %v, want the saved text and the slug", body)
	}
}

// TestAPracticeWithoutALandingShowsItsName: before the first save the public site has the
// practice's name and nothing else to show, so it never fails for a practice that has not written.
func TestAPracticeWithoutALandingShowsItsName(t *testing.T) {
	h := newHarness(t)

	status, body := h.do(t, http.MethodGet, publicPath("practica-a."+testBaseDomain), "", nil)
	if status != http.StatusOK {
		t.Fatalf("public landing = %d %v, want 200", status, body)
	}
	if body["displayName"] != "practica-a" || body["tagline"] != "" || body["email"] != "" {
		t.Fatalf("default landing = %v, want the practice name and empty text", body)
	}
}

func TestACustomDomainAnswersTheLandingOfItsPractice(t *testing.T) {
	h := newHarness(t)
	practice := h.tenantID(t, "practica-b")
	owner, _ := h.member(t, practice, iam.RoleOwner)
	h.domains.hosts["sur.paciente.test"] = tenant.ID(practice)

	if status, body := h.do(t, http.MethodPut, landingPath, owner, publishedDraft()); status != http.StatusOK {
		t.Fatalf("save = %d %v, want 200", status, body)
	}

	status, body := h.do(t, http.MethodGet, publicPath("SUR.paciente.test:443"), "", nil)
	if status != http.StatusOK {
		t.Fatalf("custom domain = %d %v, want 200: the host is normalized before the lookup", status, body)
	}
	if body["slug"] != "practica-b" || body["displayName"] != "Consultorio Sur" {
		t.Fatalf("custom domain answered %v, want practica-b's landing", body)
	}
}

// TestAHostThatPublishesNothingAnswersOneAnswer: unknown, foreign and suspended are the same
// answer, so the public cannot learn which practices exist or which are suspended.
func TestAHostThatPublishesNothingAnswersOneAnswer(t *testing.T) {
	h := newHarness(t)
	suspended := h.tenantID(t, "practica-b")
	h.suspend(t, suspended)

	cases := map[string]string{
		"unknown subdomain": "nadie." + testBaseDomain,
		"foreign domain":    "practica-a.otro.test",
		"suspended":         "practica-b." + testBaseDomain,
		"the bare base":     testBaseDomain,
	}
	var answers []map[string]any
	for name, host := range cases {
		status, body := h.do(t, http.MethodGet, publicPath(host), "", nil)
		if status != http.StatusNotFound {
			t.Fatalf("%s = %d %v, want 404", name, status, body)
		}
		answers = append(answers, body)
	}
	for _, body := range answers[1:] {
		if !reflect.DeepEqual(body, answers[0]) {
			t.Fatalf("answers differ: %v and %v: a visitor could tell the cases apart", answers[0], body)
		}
	}
}

func TestThePublicLandingNeedsAHost(t *testing.T) {
	h := newHarness(t)

	if status, _ := h.do(t, http.MethodGet, "/api/v1/public/landing", "", nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("no host = %d, want 422", status)
	}
	if status, _ := h.do(t, http.MethodGet, publicPath("   "), "", nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("blank host = %d, want 422", status)
	}
	if status, _ := h.do(t, http.MethodGet, publicPath("[::1]:8080"), "", nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("an address instead of a host = %d, want 422", status)
	}
}

// TestThePublicLandingIsRateLimitedPerClient: sixty reads a minute, then 429; and a store that
// cannot count refuses the read instead of letting every request through.
func TestThePublicLandingIsRateLimitedPerClient(t *testing.T) {
	h := newHarness(t)
	path := publicPath("practica-a." + testBaseDomain)

	for i := 1; i <= 60; i++ {
		if status, _ := h.do(t, http.MethodGet, path, "", nil); status != http.StatusOK {
			t.Fatalf("read %d = %d, want 200", i, status)
		}
	}
	if status, _ := h.do(t, http.MethodGet, path, "", nil); status != http.StatusTooManyRequests {
		t.Fatalf("read 61 = %d, want 429", status)
	}

	broken := newHarness(t)
	broken.limiter.fail = true
	if status, _ := broken.do(t, http.MethodGet, path, "", nil); status != http.StatusServiceUnavailable {
		t.Fatalf("with the counter down = %d, want 503: the public door fails closed", status)
	}
}

// TestOnlyOwnerAndAdminReadAndEditTheLanding: the landing is what the practice tells the world,
// the same kind of configuration as the mutual catalog, so it follows tenant:settings.
func TestOnlyOwnerAndAdminReadAndEditTheLanding(t *testing.T) {
	h := newHarness(t)
	practice := h.tenantID(t, "practica-a")
	owner, _ := h.member(t, practice, iam.RoleOwner)
	admin, _ := h.member(t, practice, iam.RoleAdmin)
	accountant, _ := h.member(t, practice, iam.RoleAccountant)
	assistant, _ := h.member(t, practice, iam.RoleAssistant)

	for name, token := range map[string]string{"accountant": accountant, "assistant": assistant} {
		if status, _ := h.do(t, http.MethodPut, landingPath, token, publishedDraft()); status != http.StatusForbidden {
			t.Fatalf("%s save = %d, want 403", name, status)
		}
		if status, _ := h.do(t, http.MethodGet, landingPath, token, nil); status != http.StatusForbidden {
			t.Fatalf("%s read = %d, want 403", name, status)
		}
	}

	for name, token := range map[string]string{"owner": owner, "admin": admin} {
		if status, body := h.do(t, http.MethodPut, landingPath, token, publishedDraft()); status != http.StatusOK {
			t.Fatalf("%s save = %d %v, want 200", name, status, body)
		}
	}
}

func TestTheLandingRefusesTextTheDomainRefuses(t *testing.T) {
	h := newHarness(t)
	practice := h.tenantID(t, "practica-a")
	owner, _ := h.member(t, practice, iam.RoleOwner)

	for name, field := range map[string]struct{ key, value string }{
		"a name that is blank": {"displayName", "   "},
		"an email with a name": {"email", "Ana <ana@sur.test>"},
		"a phone with letters": {"phone", "llamar a la secretaria"},
	} {
		draft := publishedDraft()
		draft[field.key] = field.value
		status, body := h.do(t, http.MethodPut, landingPath, owner, draft)
		if status != http.StatusUnprocessableEntity {
			t.Fatalf("%s = %d %v, want 422", name, status, body)
		}
		if detail, _ := body["detail"].(string); !strings.Contains(detail, field.key) {
			t.Fatalf("%s: detail = %q, want it to name %s", name, detail, field.key)
		}
	}
}

// TestEveryLandingSaveIsRecordedInTheTrail: the trail is written in the same transaction as
// the save, so a change without its record cannot exist. The event names who changed it.
func TestEveryLandingSaveIsRecordedInTheTrail(t *testing.T) {
	h := newHarness(t)
	practice := h.tenantID(t, "practica-a")
	owner, ownerID := h.member(t, practice, iam.RoleOwner)

	if status, body := h.do(t, http.MethodPut, landingPath, owner, publishedDraft()); status != http.StatusOK {
		t.Fatalf("save = %d %v, want 200", status, body)
	}

	if len(h.audit.events) == 0 {
		t.Fatalf("the save left no trail")
	}
	event := h.audit.events[len(h.audit.events)-1]
	if event.Action != "landing.updated" || event.Entity != "landing" || event.TenantID != practice || event.ActorID != ownerID {
		t.Fatalf("trail = %+v, want landing.updated by the owner in the practice", event)
	}
}

// TestAnEditOfOnePracticeNeverShowsInAnother: practice B's owner reads the default landing of B,
// not the text of A, and the public door of B does not show A's text either.
func TestAnEditOfOnePracticeNeverShowsInAnother(t *testing.T) {
	h := newHarness(t)
	practiceA := h.tenantID(t, "practica-a")
	ownerA, _ := h.member(t, practiceA, iam.RoleOwner)
	ownerB, _ := h.member(t, h.tenantID(t, "practica-b"), iam.RoleOwner)

	if status, body := h.do(t, http.MethodPut, landingPath, ownerA, publishedDraft()); status != http.StatusOK {
		t.Fatalf("save = %d %v, want 200", status, body)
	}

	status, body := h.do(t, http.MethodGet, landingPath, ownerB, nil)
	if status != http.StatusOK || body["displayName"] != "practica-b" {
		t.Fatalf("B's editor = %d %v, want its own default landing", status, body)
	}
	status, body = h.do(t, http.MethodGet, publicPath("practica-b."+testBaseDomain), "", nil)
	if status != http.StatusOK || body["displayName"] != "practica-b" {
		t.Fatalf("B's public landing = %d %v, want its own default", status, body)
	}
}

// TestAnExpiredTrialStopsTheEditButNotThePublicLanding: the write guard refuses the save with
// trial_expired like every other write, while the public landing keeps answering, because the
// visitors of a practice are not its subscription.
func TestAnExpiredTrialStopsTheEditButNotThePublicLanding(t *testing.T) {
	h := newHarness(t)
	practice := h.tenantID(t, "practica-a")
	owner, _ := h.member(t, practice, iam.RoleOwner)
	if status, body := h.do(t, http.MethodPut, landingPath, owner, publishedDraft()); status != http.StatusOK {
		t.Fatalf("save while live = %d %v, want 200", status, body)
	}

	h.endTrial(t, practice)

	status, body := h.do(t, http.MethodPut, landingPath, owner, publishedDraft())
	if status != http.StatusForbidden || body["code"] != "trial_expired" {
		t.Fatalf("save after the trial = %d %v, want 403 trial_expired", status, body)
	}

	status, body = h.do(t, http.MethodGet, publicPath("practica-a."+testBaseDomain), "", nil)
	if status != http.StatusOK || body["displayName"] != "Consultorio Sur" {
		t.Fatalf("public landing after the trial = %d %v, want the published text", status, body)
	}
}

// suspend moves a practice to suspended, the state that closes its public door.
func (h *harness) suspend(t *testing.T, id string) {
	t.Helper()
	original, ok := h.tenants.byID[tenant.ID(id)]
	if !ok {
		t.Fatalf("unknown practice %s", id)
	}
	suspended, err := tenant.Reconstitute(original.ID, original.Slug, original.Name, tenant.StatusSuspended, "",
		harnessNow.Add(-time.Hour), harnessNow.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("cannot suspend: %v", err)
	}
	h.tenants.byID[original.ID] = suspended
}
