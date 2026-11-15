import type { components } from "@/shared/api/schema";

/**
 * Internal vouchers as the API describes them. The wire types are the schema, not copies,
 * so the screens cannot drift from the contract silently.
 */
export type Voucher = components["schemas"]["VoucherBody"];
export type Subject = components["schemas"]["SubjectBody"];
export type VoucherStatus = Voucher["status"];

/**
 * Owner, admin and accountant hold billing:read. The admin only reads: issuing and voiding
 * belong to the owner and the accountant (S22, L46). Professional and assistant hold no
 * billing permission at all. The API enforces it; these helpers only keep a role off a
 * screen, or a button, that can only fail.
 */
export function canReadBilling(role: string | undefined): boolean {
  return role === "owner" || role === "admin" || role === "accountant";
}

export function canIssueVouchers(role: string | undefined): boolean {
  return role === "owner" || role === "accountant";
}

/** Voiding is held by the same two roles as issuing. */
export function canVoidVouchers(role: string | undefined): boolean {
  return canIssueVouchers(role);
}

/**
 * The currency of a new voucher. The MVP bills in pesos; the API accepts any ISO 4217 code,
 * so adding a currency later is a screen change, not a contract change.
 */
export const DEFAULT_CURRENCY = "ARS";

/** The API's page for the history: 20 by default, 100 at most. */
export const VOUCHER_PAGE_SIZE = 50;

/** The shortest fragment the subject search accepts (the API answers 422 below it). */
export const SUBJECT_SEARCH_MIN = 3;

const STATUS_LABEL: Record<VoucherStatus, string> = {
  issued: "Emitido",
  voided: "Anulado",
};

export function statusLabel(status: string): string {
  return STATUS_LABEL[status as VoucherStatus] ?? status;
}

/** The number as the practice reads it: consecutive, padded, never reused after a void. */
export function voucherNumber(number: number): string {
  return `N° ${String(number).padStart(6, "0")}`;
}

/** Formats minor units (centavos) in a currency, without going through a float of the sum. */
export function formatMoney(amountMinor: number, currency: string): string {
  const negative = amountMinor < 0;
  const absolute = Math.abs(amountMinor);
  const whole = Math.floor(absolute / 100);
  const cents = absolute % 100;
  const grouped = whole.toLocaleString("es-AR");
  return `${negative ? "-" : ""}${currency} ${grouped},${String(cents).padStart(2, "0")}`;
}

export type AmountResult = { ok: true; minor: number } | { ok: false; message: string };

/** The most digits before the decimals. Keeps the value far from the safe-integer limit. */
const MAX_WHOLE_DIGITS = 10;

const AMOUNT_FORMAT = /^(\d+)(?:[.,](\d{1,2}))?$/;

/**
 * Reads an amount typed by a person into minor units.
 *
 * It accepts digits with an optional decimal part of one or two digits, separated by a
 * comma or a point. It refuses a thousands separator: "1.500" is ambiguous between one
 * thousand five hundred and one and a half, and a wrong amount on a fiscal voucher is
 * worse than a refused one. The sum is built from the digits, never from a float.
 */
export function parseAmount(text: string): AmountResult {
  const trimmed = text.trim();
  if (trimmed === "") return { ok: false, message: "Escribí el importe." };

  const match = AMOUNT_FORMAT.exec(trimmed);
  if (match === null) {
    return {
      ok: false,
      message: "Escribí el importe solo con números, hasta dos decimales y sin separador de miles.",
    };
  }

  const whole = (match[1] ?? "").replace(/^0+(?=\d)/, "");
  if (whole.length > MAX_WHOLE_DIGITS) {
    return { ok: false, message: "El importe es demasiado grande." };
  }

  const cents = (match[2] ?? "").padEnd(2, "0");
  const minor = Number(whole) * 100 + Number(cents);
  if (minor <= 0) return { ok: false, message: "El importe tiene que ser mayor que cero." };

  return { ok: true, minor };
}

/** Formats an RFC 3339 instant with its time, tolerating a value it cannot parse. */
export function formatDateTime(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleString("es-AR", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

/**
 * The instant a date input starts, in the viewer's zone. An empty or invalid input is no
 * bound at all. The accountant holds no tenant:read, so the practice zone is not known here.
 */
export function startOfDay(dateInput: string): string | undefined {
  if (dateInput === "") return undefined;
  const parsed = new Date(`${dateInput}T00:00:00`);
  return Number.isNaN(parsed.getTime()) ? undefined : parsed.toISOString();
}

/**
 * The first instant after the chosen day. The API's `to` is exclusive ("issued before"), so
 * choosing a day as the end of the range has to include that whole day.
 */
export function startOfNextDay(dateInput: string): string | undefined {
  if (dateInput === "") return undefined;
  const parsed = new Date(`${dateInput}T00:00:00`);
  if (Number.isNaN(parsed.getTime())) return undefined;
  parsed.setDate(parsed.getDate() + 1);
  return parsed.toISOString();
}
