import type { Patient } from "@/entities/patient/model";
import type { components } from "@/shared/api/schema";

/** A clinical grant as the API describes it. `revokedAt` is set once it is withdrawn. */
export type ClinicalGrant = components["schemas"]["GrantBody"];

/*
 * Who may do what with a patient's care, seen from the viewer.
 *
 * These mirror domain/care/rules.go and the permission matrix (domain/iam). The API is
 * the authority: a helper here only decides what the screen offers, so nobody is shown
 * a control the server would refuse. If a rule changes on the server, change it here too.
 */

/** Whether the viewer is the treating professional of the patient (T38). */
export function isTreating(patient: Patient, userId: string): boolean {
  return userId !== "" && patient.treatingProfessionalId === userId;
}

/** Only professionals open the clinical record. Owner, admin and assistant never do (S19, S23). */
export function canOpenClinicalRecord(role: string | undefined): boolean {
  return role === "professional";
}

/** Writing the record and its notes belongs to the treating professional. A grant only reads. */
export function canWriteClinicalRecord(role: string | undefined, patient: Patient, userId: string): boolean {
  return canOpenClinicalRecord(role) && isTreating(patient, userId);
}

/** Only the treating professional gives a grant over the clinical record (S23). */
export function canGrantClinicalRecord(role: string | undefined, patient: Patient, userId: string): boolean {
  return canWriteClinicalRecord(role, patient, userId);
}

/** The treating professional and the owner see and withdraw the grants of a patient. */
export function canManageGrants(role: string | undefined, patient: Patient, userId: string): boolean {
  return role === "owner" || canGrantClinicalRecord(role, patient, userId);
}

/** Owner and admin reassign the treating professional (patients:assign, T33). */
export function canAssignTreating(role: string | undefined): boolean {
  return role === "owner" || role === "admin";
}

/** A professional books only with the patients it treats. The rest of the practice books with anyone. */
export function canBookWith(role: string | undefined, patient: Patient, userId: string): boolean {
  return role !== "professional" || isTreating(patient, userId);
}

/**
 * The sentence an operator can act on for a refused care action. The API answers in
 * English; the screens speak Spanish, so the status decides the words.
 */
export function careFailureMessage(status: number): string {
  switch (status) {
    case 403:
      return "Tu rol no puede hacer esto con este paciente.";
    case 404:
      return "Este paciente no está disponible para tu usuario.";
    case 409:
      return "Ese profesional ya tiene acceso a la historia clínica.";
    case 422:
      return "Ese usuario no es un profesional activo del consultorio.";
    default:
      return "No pudimos completar la acción. Intentá de nuevo.";
  }
}
