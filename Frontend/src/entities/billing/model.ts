import type { components } from "@/shared/api/schema";

/**
 * Internal vouchers as the API describes them. The wire types are the schema, not copies,
 * so the screens cannot drift from the contract silently.
 */
export type Voucher = components["schemas"]["VoucherBody"];
export type Subject = components["schemas"]["SubjectBody"];
export type VoucherStatus = Voucher["status"];
/** A session of a patient, as the emission screen lists it, with the tariff's proposal. */
export type Session = components["schemas"]["PatientSessionBody"];
export type Professional = components["schemas"]["ProfessionalBody"];
export type Photo = components["schemas"]["PhotoBody"];

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
 * The filters of the history. They live in the URL so a filtered view survives a reload and
 * can be shared, which is what a worklist is for; the voucher id rides with them instead of
 * replacing them.
 */
export type BillingFilters = {
  status: "" | VoucherStatus;
  from: string;
  to: string;
};

export const EMPTY_FILTERS: BillingFilters = { status: "", from: "", to: "" };

/**
 * The screen of the history, optionally opened on one voucher. The id rides in the query so
 * a voucher can be linked without being on the current page of the list; GET /vouchers/{id}
 * is what loads it. The shape is a Next.js location (pathname + query), not a string: typed
 * routes only know `/facturacion`.
 */
export function billingHref(
  voucherId: string | null,
  filters: BillingFilters = EMPTY_FILTERS,
): {
  pathname: "/facturacion";
  query?: Record<string, string>;
} {
  const query: Record<string, string> = {};
  if (filters.status !== "") query.estado = filters.status;
  if (filters.from !== "") query.desde = filters.from;
  if (filters.to !== "") query.hasta = filters.to;
  if (voucherId !== null && voucherId !== "") query.voucher = voucherId;

  return Object.keys(query).length === 0
    ? { pathname: "/facturacion" }
    : { pathname: "/facturacion", query };
}

/**
 * The same location as a string, for `router.replace`. Typed routes only know `/facturacion`,
 * so the query is asserted onto that path: it is still `/facturacion`, with a search.
 */
export function billingUrl(
  voucherId: string | null,
  filters: BillingFilters = EMPTY_FILTERS,
): "/facturacion" {
  const href = billingHref(voucherId, filters);
  if (href.query === undefined) return href.pathname;
  const search = new URLSearchParams(href.query).toString();
  return `${href.pathname}?${search}` as "/facturacion";
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

const MODALITY_LABEL: Record<string, string> = {
  online: "Online",
  presencial: "Presencial",
};

/**
 * Why a session cannot be billed, or null when it can. A cancelled session is never billed,
 * and a session that an issued voucher already bills is billed once: the API refuses the
 * second voucher with a 409, so the list says so before the person picks it.
 */
export function sessionBlockReason(session: Session): string | null {
  if (session.status === "cancelled") return "Cancelado";
  if (session.billedVoucherId !== null) return "Ya facturado";
  return null;
}

/** The modality as a person reads it, or null when the voucher does not carry one. */
export function modalityName(modality: string | null): string | null {
  if (modality === null) return null;
  return MODALITY_LABEL[modality] ?? modality;
}

/** One session as a line of the picker: when, with whom, and in which modality. */
export function sessionLabel(session: Session): string {
  return `${formatDateTime(session.startsAt)} · ${session.professionalName} · ${modalityName(session.modality)}`;
}

/** The day of a session as the concept of its voucher names it: 20/11/2026. */
export function formatDay(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleDateString("es-AR", { day: "2-digit", month: "2-digit", year: "numeric" });
}

/**
 * The coverage a voucher was issued with, as the photo reads it. A voucher for a patient that
 * carries no coverage was issued before the photo existed: the screen says so instead of
 * showing a particular patient it cannot know. A loose charge has no coverage at all.
 */
export function coverageText(voucher: Voucher): string {
  const coverage = voucher.photo.coverage;
  if (coverage === "private") return "Particular";
  if (coverage === "insurer") return voucher.photo.insurerName ?? "Mutual";
  return voucher.patientId ? "Sin registrar" : "—";
}

/** A field of the photo, or what stands in for it when the voucher does not carry one. */
export function photoField(voucher: Voucher, value: string | null): string {
  if (voucher.patientId && voucher.photo.coverage === null) return "Sin registrar";
  return value ?? "—";
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

/**
 * The monthly settlements to the mutuales (T55). The same roles as the vouchers: the owner and
 * the accountant issue and void, and the admin reads.
 */
export type SettlementInsurer = components["schemas"]["SettlementInsurerBody"];
export type Settlement = components["schemas"]["SettlementBody"];
export type SettlementItem = components["schemas"]["SettlementItemBody"];
export type SettlementPreview = components["schemas"]["SettlementPreviewBody"];
export type CurrencyTotal = components["schemas"]["CurrencyTotalBody"];
export type UnvaluedSession = components["schemas"]["UnvaluedSessionBody"];

/** The API's page for the settlements: 50 at most per request. */
export const SETTLEMENT_PAGE_SIZE = 50;

const SETTLEMENT_STATUS_LABEL: Record<string, string> = {
  issued: "Emitida",
  voided: "Anulada",
};

export function settlementStatusLabel(status: string): string {
  return SETTLEMENT_STATUS_LABEL[status] ?? status;
}

/** The month as a person reads it: noviembre de 2026. */
export function periodName(period: string): string {
  const parsed = new Date(`${period}-01T00:00:00`);
  if (Number.isNaN(parsed.getTime())) return period;
  return parsed.toLocaleDateString("es-AR", { month: "long", year: "numeric" });
}

/**
 * The totals as a line, one per currency. Currencies are never converted: the MVP has no
 * exchange rate, so "ARS 16.000,00 · USD 120,00" is the honest sum.
 */
export function totalsText(totals: CurrencyTotal[]): string {
  if (totals.length === 0) return "Sin importe";
  return totals.map((total) => formatMoney(total.amountMinor, total.currency)).join(" · ");
}

/** A session count as a person reads it: 1 sesión, 2 sesiones. */
export function sessionCountText(count: number): string {
  return count === 1 ? "1 sesión" : `${count} sesiones`;
}