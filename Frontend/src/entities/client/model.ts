import type { components } from "@/shared/api/schema";
import { taxIDKindLabel } from "@/entities/patient/model";

/**
 * A billing client: who pays, which is not always who is treated (S26, T71).
 *
 * The wire types are the schema, not copies, so a screen cannot drift from the contract
 * silently. Since T72 the fiscal data and the billing currency live here and no longer on
 * the patient.
 */
export type Client = components["schemas"]["ClientBody"];
export type ClientProfile = components["schemas"]["ClientProfileBody"];

/**
 * Who may read and write the client catalog, mirroring the grants in migration 0026.
 *
 * The owner holds every permission. The accountant reads clients because it invoices against
 * them, but never writes them: the fiscal data a receipt freezes is not theirs to change.
 * These helpers only keep a role off a screen that can only fail; the API enforces the matrix.
 */
const READ_ROLES = ["owner", "admin", "professional", "assistant", "accountant"] as const;
const WRITE_ROLES = ["owner", "admin", "professional", "assistant"] as const;

export function canReadClients(role: string | undefined): boolean {
  return role !== undefined && (READ_ROLES as readonly string[]).includes(role);
}

export function canManageClients(role: string | undefined): boolean {
  return role !== undefined && (WRITE_ROLES as readonly string[]).includes(role);
}

/**
 * The fiscal document as one string: the kind says what the number is, so one without the
 * other is noise. Shared by the client list and the patient record, which shows who pays.
 */
export function fiscalDocumentText(client: Client): string {
  if (!client.taxIdNumber) return "—";
  const kind = taxIDKindLabel(client.taxIdKind);
  return kind ? `${kind} ${client.taxIdNumber}` : client.taxIdNumber;
}
