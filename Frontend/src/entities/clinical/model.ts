import type { components } from "@/shared/api/schema";

/**
 * Wire types for the clinical record. Same rule as patients: the schema is the
 * type, so the screen cannot drift from the contract without a type error.
 */
export type ClinicalRecord = components["schemas"]["ClinicalRecordBody"];
export type ClinicalNote = components["schemas"]["ClinicalNoteBody"];

export const CLINICAL_NOTES_PAGE_SIZE = 20;

/** Formats an encounter timestamp for the Spanish product surface. */
export function formatEncounteredAt(value: string | undefined): string {
  if (!value) return "";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleString("es-AR", {
    dateStyle: "short",
    timeStyle: "short",
  });
}

/** Value for `<input type="datetime-local">` from a Date in the browser's zone. */
export function toDateTimeLocalValue(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

/** RFC 3339 for the API from a datetime-local value (interpreted as local time). */
export function dateTimeLocalToRFC3339(value: string): string | null {
  if (value.trim() === "") return null;
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return null;
  return parsed.toISOString();
}
