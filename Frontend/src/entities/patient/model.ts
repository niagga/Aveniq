import type { components } from "@/shared/api/schema";

/**
 * A patient as the API describes it. The wire type is the schema, not a copy:
 * duplicating it here would let the screen drift from the contract silently.
 */
export type Patient = components["schemas"]["PatientBody"];

/**
 * Labels for the document kinds seeded in the country catalogs (migration 0005).
 *
 * An unknown code renders as the raw code instead of a blank: the catalog is
 * extended by migration, and a screen that hides what it does not recognise turns a
 * new country into a patient row with no document at all.
 */
const DOCUMENT_KIND_LABEL: Record<string, string> = {
  dni: "DNI",
  lc: "Libreta cívica",
  le: "Libreta de enrolamiento",
  nie: "NIE",
  nif: "NIF",
  cpf: "CPF",
  cnpj: "CNPJ",
  rg: "RG",
  cc: "Cartão de Cidadão",
  passport: "Pasaporte",
};

const TAX_ID_KIND_LABEL: Record<string, string> = {
  cuit: "CUIT",
  nif: "NIF",
  cpf: "CPF",
  cnpj: "CNPJ",
};

const FISCAL_CONDITION_LABEL: Record<string, string> = {
  consumidor_final: "Consumidor final",
  monotributo: "Monotributo",
  responsable_inscripto: "Responsable inscripto",
  exento: "Exento",
  particular: "Particular",
  autonomo: "Autónomo",
  sociedad: "Sociedad",
  sociedade: "Sociedade",
  empresario_individual: "Empresário individual",
  mei: "MEI",
  simples_nacional: "Simples Nacional",
  lucro_presumido: "Lucro presumido",
};

export function documentKindLabel(code: string | undefined): string {
  if (!code) return "";
  return DOCUMENT_KIND_LABEL[code] ?? code;
}

export function taxIDKindLabel(code: string | undefined): string {
  if (!code) return "";
  return TAX_ID_KIND_LABEL[code] ?? code;
}

export function fiscalConditionLabel(code: string | undefined): string {
  if (!code) return "";
  return FISCAL_CONDITION_LABEL[code] ?? code;
}

/**
 * The countries the catalogs know today. Sending a code the catalog does not have
 * answers 422, so the form offers only these until a migration adds more.
 */
export const COUNTRIES = [
  { code: "AR", label: "Argentina" },
  { code: "ES", label: "España" },
  { code: "PT", label: "Portugal" },
  { code: "BR", label: "Brasil" },
] as const;

/** Document kinds offered per country, mirroring the seeded catalog rows. */
export const DOCUMENT_KINDS_BY_COUNTRY: Record<string, string[]> = {
  AR: ["dni", "lc", "le", "passport"],
  ES: ["dni", "nie", "passport"],
  PT: ["cc", "passport"],
  BR: ["cpf", "rg", "passport"],
};

/** Tax id kinds offered per country. */
export const TAX_ID_KINDS_BY_COUNTRY: Record<string, string[]> = {
  AR: ["cuit"],
  ES: ["nif"],
  PT: ["nif"],
  BR: ["cpf", "cnpj"],
};

/** Fiscal conditions offered per country. */
export const FISCAL_CONDITIONS_BY_COUNTRY: Record<string, string[]> = {
  AR: ["consumidor_final", "monotributo", "responsable_inscripto", "exento"],
  ES: ["particular", "autonomo", "sociedad"],
  PT: ["particular", "empresario_individual", "sociedade"],
  BR: ["mei", "simples_nacional", "lucro_presumido"],
};

/** Formats a date of birth for display, tolerating an empty value. */
export function formatBirthDate(value: string | undefined): string {
  if (!value) return "";
  const parsed = new Date(`${value}T00:00:00Z`);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleDateString("es-AR", { timeZone: "UTC" });
}
