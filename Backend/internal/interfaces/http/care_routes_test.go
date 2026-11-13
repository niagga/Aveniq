package http_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/niagga/aveniq/backend/internal/application/port"
	"github.com/niagga/aveniq/backend/internal/domain/clinical"
	"github.com/niagga/aveniq/backend/internal/domain/iam"
)

// fakeClinicalRepository keeps one frame per patient in memory. Notes are not used by
// these routes, so they are accepted and empty.
type fakeClinicalRepository struct {
	records map[string]clinical.Record
}

func newFakeClinicalRepository() *fakeClinicalRepository {
	return &fakeClinicalRepository{records: map[string]clinical.Record{}}
}

var _ port.ClinicalRepository = (*fakeClinicalRepository)(nil)

func (r *fakeClinicalRepository) UpsertRecord(_ context.Context, record clinical.Record) (clinical.Record, error) {
	r.records[record.PatientID] = record
	return record, nil
}

// RecordByPatient answers the empty frame when nothing was written, as a first visit does.
func (r *fakeClinicalRepository) RecordByPatient(_ context.Context, patientID string) (clinical.Record, error) {
	return r.records[patientID], nil
}

func (r *fakeClinicalRepository) AddNote(context.Context, clinical.Note) error { return nil }

func (r *fakeClinicalRepository) NotesByPatient(context.Context, string, int32, int32) ([]clinical.Note, error) {
	return nil, nil
}

// createPatient registers a patient through the API as the given token and returns its id.
func createPatient(t *testing.T, h *harness, token, fullName, document string) string {
	t.Helper()

	status, body := h.do(t, http.MethodPost, "/api/v1/patients", token, map[string]any{
		"fullName":       fullName,
		"documentKind":   "dni",
		"documentNumber": document,
	})
	if status != http.StatusCreated {
		t.Fatalf("create %q = %d, want 201: %v", fullName, status, body)
	}
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("create %q answered no id: %v", fullName, body)
	}
	return id
}

func listedCount(t *testing.T, body map[string]any) int {
	t.Helper()
	items, _ := body["items"].([]any)
	return len(items)
}

// TestAProfessionalRegisteringAPatientTreatsIt pins the rule that the professional who
// registers a patient is its treating one, and that the rest of the practice still sees
// it while another professional does not.
func TestAProfessionalRegisteringAPatientTreatsIt(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")

	treating, treatingID := h.member(t, tenantA, iam.RoleProfessional)
	stranger, _ := h.member(t, tenantA, iam.RoleProfessional)
	assistant, _ := h.member(t, tenantA, iam.RoleAssistant)

	status, body := h.do(t, http.MethodPost, "/api/v1/patients", treating, map[string]any{
		"fullName":       "Paciente del tratante",
		"documentKind":   "dni",
		"documentNumber": "36111222",
	})
	if status != http.StatusCreated {
		t.Fatalf("create = %d, want 201: %v", status, body)
	}
	id, _ := body["id"].(string)
	if body["treatingProfessionalId"] != treatingID {
		t.Fatalf("treatingProfessionalId = %v, want the registering professional %s", body["treatingProfessionalId"], treatingID)
	}

	if status, _ := h.do(t, http.MethodGet, "/api/v1/patients/"+id, treating, nil); status != http.StatusOK {
		t.Fatalf("treating professional reads its patient = %d, want 200", status)
	}
	if status, _ := h.do(t, http.MethodGet, "/api/v1/patients/"+id, stranger, nil); status != http.StatusNotFound {
		t.Fatalf("another professional reads the patient = %d, want 404: an invisible patient looks absent", status)
	}
	if status, _ := h.do(t, http.MethodGet, "/api/v1/patients/"+id, assistant, nil); status != http.StatusOK {
		t.Fatalf("assistant reads the patient = %d, want 200: the front desk sees the whole roster", status)
	}

	if _, body := h.do(t, http.MethodGet, "/api/v1/patients", treating, nil); listedCount(t, body) != 1 {
		t.Fatalf("treating professional lists %d patients, want its one", listedCount(t, body))
	}
	if _, body := h.do(t, http.MethodGet, "/api/v1/patients", stranger, nil); listedCount(t, body) != 0 {
		t.Fatalf("another professional lists %d patients, want none", listedCount(t, body))
	}
}

// TestOnlyTheOwnerAndAdminAssignTheTreatingProfessional covers the reassignment: the
// assistant and the professionals cannot move a patient to another professional, and the
// move is refused unless the target holds the professional role.
func TestOnlyTheOwnerAndAdminAssignTheTreatingProfessional(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")

	owner, _ := h.member(t, tenantA, iam.RoleOwner)
	admin, _ := h.member(t, tenantA, iam.RoleAdmin)
	assistant, assistantID := h.member(t, tenantA, iam.RoleAssistant)
	treating, treatingID := h.member(t, tenantA, iam.RoleProfessional)
	nextTreating, nextID := h.member(t, tenantA, iam.RoleProfessional)

	id := createPatient(t, h, owner, "Sin tratante", "37111222")
	path := "/api/v1/patients/" + id + "/treating-professional"

	t.Run("the assistant cannot assign", func(t *testing.T) {
		if status, _ := h.do(t, http.MethodPut, path, assistant, map[string]any{"professionalId": nextID}); status != http.StatusForbidden {
			t.Fatalf("assistant assigns = %d, want 403", status)
		}
	})

	t.Run("a professional cannot assign itself or another", func(t *testing.T) {
		if status, _ := h.do(t, http.MethodPut, path, treating, map[string]any{"professionalId": treatingID}); status != http.StatusForbidden {
			t.Fatalf("professional assigns = %d, want 403", status)
		}
	})

	t.Run("a target without the professional role is 422", func(t *testing.T) {
		if status, _ := h.do(t, http.MethodPut, path, owner, map[string]any{"professionalId": assistantID}); status != http.StatusUnprocessableEntity {
			t.Fatalf("assign to an assistant = %d, want 422", status)
		}
	})

	t.Run("the admin assigns", func(t *testing.T) {
		status, body := h.do(t, http.MethodPut, path, admin, map[string]any{"professionalId": nextID})
		if status != http.StatusOK {
			t.Fatalf("admin assigns = %d, want 200: %v", status, body)
		}
		if body["professionalId"] != nextID {
			t.Fatalf("professionalId = %v, want %s", body["professionalId"], nextID)
		}
	})

	t.Run("the owner reassigns and the previous professional loses sight of the patient", func(t *testing.T) {
		if status, _ := h.do(t, http.MethodPut, path, owner, map[string]any{"professionalId": treatingID}); status != http.StatusOK {
			t.Fatalf("owner reassigns = %d, want 200", status)
		}
		if status, _ := h.do(t, http.MethodGet, "/api/v1/patients/"+id, nextTreating, nil); status != http.StatusNotFound {
			t.Fatalf("previous professional reads the patient = %d, want 404", status)
		}
		if status, _ := h.do(t, http.MethodGet, "/api/v1/patients/"+id, treating, nil); status != http.StatusOK {
			t.Fatalf("new professional reads the patient = %d, want 200", status)
		}
	})
}

// TestTheClinicalRecordIsReadOnlyByItsTreatingProfessional is the instance rule over
// HTTP: the matrix lets a professional read the clinical record, the rule lets only the
// one that treats the patient, and the owner, who holds every permission, is still refused.
func TestTheClinicalRecordIsReadOnlyByItsTreatingProfessional(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")

	treating, _ := h.member(t, tenantA, iam.RoleProfessional)
	stranger, _ := h.member(t, tenantA, iam.RoleProfessional)
	assistant, _ := h.member(t, tenantA, iam.RoleAssistant)
	admin, _ := h.member(t, tenantA, iam.RoleAdmin)
	owner, _ := h.member(t, tenantA, iam.RoleOwner)

	id := createPatient(t, h, treating, "Con historia clínica", "38111222")
	record := "/api/v1/patients/" + id + "/clinical-record"

	for _, tc := range []struct {
		name  string
		token string
		want  int
	}{
		{"the treating professional reads", treating, http.StatusOK},
		{"another professional is told the patient does not exist", stranger, http.StatusNotFound},
		{"the assistant is refused by the matrix", assistant, http.StatusForbidden},
		{"the admin is refused by the matrix", admin, http.StatusForbidden},
		{"the owner holds the permission but is refused by the rule", owner, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if status, body := h.do(t, http.MethodGet, record, tc.token, nil); status != tc.want {
				t.Fatalf("read = %d, want %d: %v", status, tc.want, body)
			}
		})
	}

	t.Run("the owner cannot write the record either", func(t *testing.T) {
		if status, _ := h.do(t, http.MethodPut, record, owner, map[string]any{"chiefComplaint": "no es suyo"}); status != http.StatusForbidden {
			t.Fatalf("owner writes = %d, want 403", status)
		}
	})

	t.Run("the treating professional writes", func(t *testing.T) {
		if status, body := h.do(t, http.MethodPut, record, treating, map[string]any{"chiefComplaint": "Ansiedad"}); status != http.StatusOK {
			t.Fatalf("treating writes = %d, want 200: %v", status, body)
		}
	})
}

// TestAClinicalGrantReadsButNeverWritesOrGrantsOn walks the grant lifecycle over HTTP: the
// treating professional grants, the grantee reads and is refused every write, the owner
// keeps the list, and a revocation ends the read at once.
func TestAClinicalGrantReadsButNeverWritesOrGrantsOn(t *testing.T) {
	h := newHarness(t)
	tenantA := h.tenantID(t, "practica-a")

	treating, _ := h.member(t, tenantA, iam.RoleProfessional)
	grantee, granteeID := h.member(t, tenantA, iam.RoleProfessional)
	stranger, strangerID := h.member(t, tenantA, iam.RoleProfessional)
	owner, _ := h.member(t, tenantA, iam.RoleOwner)

	id := createPatient(t, h, treating, "Con concesión", "39111222")
	grants := "/api/v1/patients/" + id + "/clinical-grants"
	record := "/api/v1/patients/" + id + "/clinical-record"
	grantID := ""

	t.Run("a professional that does not treat the patient cannot grant on it", func(t *testing.T) {
		if status, _ := h.do(t, http.MethodPost, grants, stranger, map[string]any{"professionalId": granteeID}); status != http.StatusNotFound {
			t.Fatalf("stranger grants = %d, want 404", status)
		}
	})

	t.Run("the treating professional grants", func(t *testing.T) {
		status, body := h.do(t, http.MethodPost, grants, treating, map[string]any{"professionalId": granteeID})
		if status != http.StatusCreated {
			t.Fatalf("grant = %d, want 201: %v", status, body)
		}
		grantID, _ = body["id"].(string)
		if body["professionalId"] != granteeID || grantID == "" {
			t.Fatalf("grant body = %v, want the grantee and an id", body)
		}
	})

	t.Run("granting twice is 409", func(t *testing.T) {
		if status, _ := h.do(t, http.MethodPost, grants, treating, map[string]any{"professionalId": granteeID}); status != http.StatusConflict {
			t.Fatalf("second grant = %d, want 409", status)
		}
	})

	t.Run("the grantee reads the patient and the record", func(t *testing.T) {
		if status, _ := h.do(t, http.MethodGet, "/api/v1/patients/"+id, grantee, nil); status != http.StatusOK {
			t.Fatalf("grantee reads the patient = %d, want 200", status)
		}
		if status, _ := h.do(t, http.MethodGet, record, grantee, nil); status != http.StatusOK {
			t.Fatalf("grantee reads the record = %d, want 200", status)
		}
	})

	t.Run("the grantee cannot write the record", func(t *testing.T) {
		if status, _ := h.do(t, http.MethodPut, record, grantee, map[string]any{"chiefComplaint": "mía"}); status != http.StatusForbidden {
			t.Fatalf("grantee writes = %d, want 403: a grant is read access", status)
		}
	})

	t.Run("the grantee cannot pass the grant on", func(t *testing.T) {
		if status, _ := h.do(t, http.MethodPost, grants, grantee, map[string]any{"professionalId": strangerID}); status != http.StatusForbidden {
			t.Fatalf("grantee grants = %d, want 403", status)
		}
	})

	t.Run("the grantee cannot list the grants", func(t *testing.T) {
		if status, _ := h.do(t, http.MethodGet, grants, grantee, nil); status != http.StatusForbidden {
			t.Fatalf("grantee lists = %d, want 403", status)
		}
	})

	t.Run("the owner lists the grants", func(t *testing.T) {
		status, body := h.do(t, http.MethodGet, grants, owner, nil)
		if status != http.StatusOK {
			t.Fatalf("owner lists = %d, want 200", status)
		}
		if n := listedCount(t, body); n != 1 {
			t.Fatalf("owner sees %d grants, want the one", n)
		}
	})

	t.Run("the treating professional revokes and the grantee loses the read", func(t *testing.T) {
		revoke := grants + "/" + grantID + "/revoke"
		status, body := h.do(t, http.MethodPost, revoke, treating, nil)
		if status != http.StatusOK {
			t.Fatalf("revoke = %d, want 200: %v", status, body)
		}
		if body["revokedAt"] == nil {
			t.Fatalf("revokedAt is empty after revoke: %v", body)
		}
		if status, _ := h.do(t, http.MethodGet, "/api/v1/patients/"+id, grantee, nil); status != http.StatusNotFound {
			t.Fatalf("grantee reads the patient after revoke = %d, want 404", status)
		}
		if status, _ := h.do(t, http.MethodGet, record, grantee, nil); status != http.StatusNotFound {
			t.Fatalf("grantee reads the record after revoke = %d, want 404", status)
		}
	})

	t.Run("revoking twice is not an error", func(t *testing.T) {
		if status, _ := h.do(t, http.MethodPost, grants+"/"+grantID+"/revoke", treating, nil); status != http.StatusOK {
			t.Fatalf("second revoke = %d, want 200", status)
		}
	})
}
