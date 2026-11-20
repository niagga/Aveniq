import { canReadBilling, parseAmount, type AmountResult } from "@/entities/billing/model";
import type { components } from "@/shared/api/schema";

/**
 * The fee schedule (T53) as the API describes it. The wire types are the schema, not copies,
 * so the screens cannot drift from the contract silently.
 */
export type Tariff = components["schemas"]["TariffBody"];
export type TariffCreate = components["schemas"]["TariffCreateBody"];
export type TariffAmounts = components["schemas"]["TariffAmountsBody"];
export type Coverage = Tariff["coverage"];
export type Modality = Tariff["modality"];
export type Currency = Tariff["billingCurrency"];

export const COVERAGES: readonly { value: Coverage; label: string }[] = [
  { value: "private", label: "Particular" },
  { value: "insurer", label: "Mutual" },
];

export const MODALITIES: readonly { value: Modality; label: string }[] = [
  { value: "online", label: "Online" },
  { value: "presencial", label: "Presencial" },
];

export const CURRENCIES: readonly Currency[] = ["ARS", "EUR", "USD"];

/**
 * Owner and admin hold tenant:settings, which writes the fee schedule. The accountant reads
 * it with billing:read, and so does the admin; the API enforces both, and these helpers only
 * keep a role off a control that can only fail.
 */
export function canEditTariffs(role: string | undefined): boolean {
  return role === "owner" || role === "admin";
}

/** Who may open the fee schedule at all: the billing readers. */
export function canReadTariffs(role: string | undefined): boolean {
  return canReadBilling(role);
}

export function coverageLabel(tariff: Pick<Tariff, "coverage" | "insurerName">): string {
  if (tariff.coverage === "private") return "Particular";
  return tariff.insurerName ?? "Mutual";
}

export function modalityLabel(modality: string): string {
  return MODALITIES.find((option) => option.value === modality)?.label ?? modality;
}

/**
 * Renders the day a price starts on, as the practice reads dates. The API sends a civil day
 * (YYYY-MM-DD); anything else is shown as it came.
 */
export function formatValidFrom(validFrom: string): string {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(validFrom);
  if (match === null) return validFrom;
  return `${match[3]}/${match[2]}/${match[1]}`;
}

/** Minor units as the amount inputs show them: "15000,00" for 1500000. */
export function minorToInput(minor: number): string {
  const whole = Math.floor(minor / 100);
  const cents = minor % 100;
  return `${whole},${String(cents).padStart(2, "0")}`;
}

/**
 * The copago of a mutual price may be zero: the mutual covers the whole session. Every other
 * amount must be above zero, which parseAmount already enforces.
 */
export function parsePatientShare(text: string): AmountResult {
  if (/^0([.,]0{1,2})?$/.test(text.trim())) return { ok: true, minor: 0 };
  return parseAmount(text);
}

/** Today's civil day in the viewer's zone, as a date input wants it. */
export function todayIso(now: Date = new Date()): string {
  const year = now.getFullYear();
  const month = String(now.getMonth() + 1).padStart(2, "0");
  const day = String(now.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}
