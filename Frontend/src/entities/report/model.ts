import type { components } from "@/shared/api/schema";

/**
 * The billing report (T58) as the API describes it. The wire types are the schema, not copies,
 * so the screens cannot drift from the contract silently.
 */
export type BillingReport = components["schemas"]["BillingReportBody"];
export type ReportRow = components["schemas"]["ReportRowBody"];
export type ReportMoney = components["schemas"]["ReportMoneyBody"];
export type ReportFigures = components["schemas"]["ReportFiguresBody"];
export type ReportDimension = BillingReport["groupBy"];

/**
 * Owner, admin and accountant hold reports:read and see the whole practice; the professional
 * holds reports:read:own and sees only their own sessions and vouchers. The assistant holds
 * neither. The API enforces it and decides the scope; this only keeps a role off a section that
 * can only answer 403.
 */
export function canReadReports(role: string | undefined): boolean {
  return (
    role === "owner" || role === "admin" || role === "accountant" || role === "professional"
  );
}

/** The cuts a person can ask the dashboard for, in the order the tabs show them. */
export const CUTS: readonly { value: ReportDimension; label: string }[] = [
  { value: "coverage", label: "Cobertura" },
  { value: "insurer", label: "Mutual" },
  { value: "professional", label: "Profesional" },
  { value: "modality", label: "Modalidad" },
];

// ─── civil days ──────────────────────────────────────────────────────────────

/**
 * A civil day, YYYY-MM-DD. The report reads its range in the practice's zone, so the screen
 * speaks in days and never in instants: nothing here goes through the browser's own zone.
 */
export type CivilDay = string;

/** The zone every practice of the MVP lives in; used when the practice is not readable. */
export const DEFAULT_ZONE = "America/Argentina/Buenos_Aires";

/** Today as a civil day in the given zone. */
export function civilToday(zone: string, now: Date = new Date()): CivilDay {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: zone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(now);
  const pick = (type: string) => parts.find((part) => part.type === type)?.value ?? "";
  return `${pick("year")}-${pick("month")}-${pick("day")}`;
}

function pad(value: number): string {
  return String(value).padStart(2, "0");
}

function split(day: CivilDay): { year: number; month: number; dayOfMonth: number } {
  const [year = 0, month = 1, dayOfMonth = 1] = day.split("-").map(Number);
  return { year, month, dayOfMonth };
}

/** The first day of the month that is `offset` months from the month of `day`. */
export function firstOfMonth(day: CivilDay, offset = 0): CivilDay {
  const { year, month } = split(day);
  const shifted = new Date(Date.UTC(year, month - 1 + offset, 1));
  return `${shifted.getUTCFullYear()}-${pad(shifted.getUTCMonth() + 1)}-01`;
}

/** The last day of the month that is `offset` months from the month of `day`. */
export function lastOfMonth(day: CivilDay, offset = 0): CivilDay {
  const { year, month } = split(day);
  const shifted = new Date(Date.UTC(year, month + offset, 0));
  return `${shifted.getUTCFullYear()}-${pad(shifted.getUTCMonth() + 1)}-${pad(shifted.getUTCDate())}`;
}

export type DayRange = { from: CivilDay; to: CivilDay };

export type Preset = "this-month" | "last-month" | "last-3-months" | "this-year";

export const PRESETS: readonly { value: Preset; label: string }[] = [
  { value: "this-month", label: "Este mes" },
  { value: "last-month", label: "Mes anterior" },
  { value: "last-3-months", label: "Últimos 3 meses" },
  { value: "this-year", label: "Este año" },
];

/**
 * The range of a preset. A range that includes the current period ends today: nothing has been
 * attended or billed after it, and a figure labelled "this month" reads better as "so far".
 */
export function rangeFor(preset: Preset, today: CivilDay): DayRange {
  switch (preset) {
    case "this-month":
      return { from: firstOfMonth(today), to: today };
    case "last-month":
      return { from: firstOfMonth(today, -1), to: lastOfMonth(today, -1) };
    case "last-3-months":
      return { from: firstOfMonth(today, -2), to: today };
    case "this-year":
      return { from: `${split(today).year}-01-01`, to: today };
  }
}

/** How many months the evolution chart shows, ending with the current one. */
export const TREND_MONTHS = 6;

/** The range of the evolution chart: the last months, always, whatever the preset. */
export function trendRange(today: CivilDay): DayRange {
  return { from: firstOfMonth(today, -(TREND_MONTHS - 1)), to: today };
}

/** The months of a range as the report keys them (YYYY-MM), oldest first. */
export function monthKeys(range: DayRange): string[] {
  const keys: string[] = [];
  let cursor = firstOfMonth(range.from);
  while (cursor <= range.to) {
    keys.push(cursor.slice(0, 7));
    cursor = firstOfMonth(cursor, 1);
  }
  return keys;
}

// ─── money ───────────────────────────────────────────────────────────────────

/** The sum of one currency in a list of figures, or zero when the list has none of it. */
export function amountIn(figures: readonly ReportMoney[] | null, currency: string): number {
  return (figures ?? []).find((money) => money.currency === currency)?.amountMinor ?? 0;
}

/** How many vouchers or sessions make the sum of one currency. */
export function countIn(figures: readonly ReportMoney[] | null, currency: string): number {
  return (figures ?? []).find((money) => money.currency === currency)?.count ?? 0;
}

/**
 * The currencies that appear anywhere in the reports, sorted. Money of different currencies is
 * never added: the screen shows one currency at a time and lets the person switch.
 */
export function currenciesOf(reports: readonly (BillingReport | null)[]): string[] {
  const found = new Set<string>();
  const take = (figures: readonly ReportMoney[] | null) => {
    for (const money of figures ?? []) found.add(money.currency);
  };
  for (const report of reports) {
    if (report === null) continue;
    take(report.totals.billed);
    take(report.totals.pending);
    for (const row of report.rows ?? []) {
      take(row.billed);
      take(row.pending);
    }
  }
  return [...found].sort();
}

// ─── labels ──────────────────────────────────────────────────────────────────

const MODALITY_LABEL: Record<string, string> = {
  online: "Online",
  presencial: "Presencial",
};

const COVERAGE_LABEL: Record<string, string> = {
  private: "Particular",
  insurer: "Con mutual",
};

/**
 * The name of a group of a cut. The lines that carry no value for the cut have an empty key:
 * the screen says what that means for each cut instead of leaving a blank row.
 */
export function rowLabel(dimension: ReportDimension, row: ReportRow): string {
  switch (dimension) {
    case "professional":
      return row.label || "Sin profesional";
    case "coverage":
      return COVERAGE_LABEL[row.key] ?? "Sin cobertura registrada";
    case "insurer":
      return row.label || "Particular o sin mutual";
    case "modality":
      return MODALITY_LABEL[row.key] ?? "Sin dato";
    case "week":
      return weekLabel(row.key);
    case "month":
      return monthLabel(row.key);
    case "year":
      return row.key;
    case "none":
      return row.label;
  }
}

/** A month key as a short label: "nov 2026". */
export function monthLabel(key: string): string {
  const { year, month } = split(`${key}-01`);
  if (!year) return key;
  return new Date(Date.UTC(year, month - 1, 1))
    .toLocaleDateString("es-AR", { month: "short", year: "numeric", timeZone: "UTC" })
    .replace(".", "");
}

/** A week key (its Monday) as a label: "sem. 16/11". */
export function weekLabel(key: string): string {
  const { month, dayOfMonth } = split(key);
  return `sem. ${pad(dayOfMonth)}/${pad(month)}`;
}

/**
 * The total a row stands for, in one currency: what was billed plus what is attended and not
 * yet billed. It sizes the bars; it is never shown as a figure of its own.
 */
export function rowTotal(row: ReportRow, currency: string): number {
  return amountIn(row.billed, currency) + amountIn(row.pending, currency);
}

// ─── saved reports (T60) ─────────────────────────────────────────────────────

/** A saved report: the definition of a question, never its figures. */
export type SavedReport = components["schemas"]["SavedReportBody"];
export type SavedReportDraft = components["schemas"]["SavedReportWriteBody"];
export type ReportPeriod = SavedReport["period"];

/** What narrows a report. An empty string is "no filter": it is what a select holds. */
export type ReportFilters = {
  professionalId: string;
  coverage: "" | "private" | "insurer";
  insurerId: string;
  modality: "" | "online" | "presencial";
};

export const NO_FILTERS: ReportFilters = {
  professionalId: "",
  coverage: "",
  insurerId: "",
  modality: "",
};

/** Every cut the report page offers, the periods first: they are what a trend is read by. */
export const GROUPINGS: readonly { value: ReportDimension; label: string }[] = [
  { value: "none", label: "Sin corte" },
  { value: "month", label: "Mes" },
  { value: "week", label: "Semana" },
  { value: "year", label: "Año" },
  { value: "professional", label: "Profesional" },
  { value: "coverage", label: "Cobertura" },
  { value: "insurer", label: "Mutual" },
  { value: "modality", label: "Modalidad" },
];

export function groupingLabel(value: ReportDimension): string {
  return GROUPINGS.find((option) => option.value === value)?.label ?? value;
}

export const PERIOD_OPTIONS: readonly { value: ReportPeriod; label: string }[] = [
  ...PRESETS,
  { value: "custom", label: "Personalizado" },
];

/** The most days a range may span; the API refuses more, so the screen says so before asking. */
export const MAX_RANGE_DAYS = 1830;

const CIVIL_DAY = /^\d{4}-\d{2}-\d{2}$/;

/** Whether a text is a real calendar day, not only the shape of one ("2026-02-30" is not). */
export function isCivilDay(text: string): boolean {
  if (!CIVIL_DAY.test(text)) return false;
  const { year, month, dayOfMonth } = split(text);
  const date = new Date(Date.UTC(year, month - 1, dayOfMonth));
  return (
    date.getUTCFullYear() === year &&
    date.getUTCMonth() === month - 1 &&
    date.getUTCDate() === dayOfMonth
  );
}

function daysBetween(from: CivilDay, to: CivilDay): number {
  const a = split(from);
  const b = split(to);
  return Math.round(
    (Date.UTC(b.year, b.month - 1, b.dayOfMonth) - Date.UTC(a.year, a.month - 1, a.dayOfMonth)) /
      86_400_000,
  );
}

export type RangeResult = { ok: true; range: DayRange } | { ok: false; message: string };

/**
 * The days a period stands for. A relative period is read against today, so a report saved as
 * "este mes" keeps meaning this month whenever it is opened; a custom one is read as written.
 */
export function resolvePeriod(
  period: ReportPeriod,
  from: string,
  to: string,
  today: CivilDay,
): RangeResult {
  if (period !== "custom") return { ok: true, range: rangeFor(period, today) };

  if (!isCivilDay(from) || !isCivilDay(to)) {
    return { ok: false, message: "Elegí el primer y el último día del período." };
  }
  if (from > to) {
    return { ok: false, message: "El primer día no puede ser posterior al último." };
  }
  if (daysBetween(from, to) + 1 > MAX_RANGE_DAYS) {
    return { ok: false, message: "El período no puede superar los 5 años." };
  }
  return { ok: true, range: { from, to } };
}

/** The filters of a saved report, as the page's selects hold them. */
export function filtersOf(saved: SavedReport): ReportFilters {
  return {
    professionalId: saved.professionalId ?? "",
    coverage: saved.coverage ?? "",
    insurerId: saved.insurerId ?? "",
    modality: saved.modality ?? "",
  };
}

/**
 * The body that saves what the page shows. Only what applies is sent: days only for a custom
 * period, a mutual only when the coverage is not private, so the API never gets a contradiction.
 */
export function draftOf(
  name: string,
  period: ReportPeriod,
  from: string,
  to: string,
  groupBy: ReportDimension,
  filters: ReportFilters,
): SavedReportDraft {
  const draft: SavedReportDraft = { name: name.trim(), period, groupBy };
  if (period === "custom") {
    draft.from = from;
    draft.to = to;
  }
  if (filters.professionalId !== "") draft.professionalId = filters.professionalId;
  if (filters.coverage !== "") draft.coverage = filters.coverage;
  if (filters.insurerId !== "" && filters.coverage !== "private") {
    draft.insurerId = filters.insurerId;
  }
  if (filters.modality !== "") draft.modality = filters.modality;
  return draft;
}

/** A line that says what a saved report asks, for the list: "Este mes · por mutual · Online". */
export function describeSaved(saved: SavedReport): string {
  const period =
    saved.period === "custom"
      ? `${saved.from ?? "?"} a ${saved.to ?? "?"}`
      : (PERIOD_OPTIONS.find((option) => option.value === saved.period)?.label ?? saved.period);
  const parts = [period, saved.groupBy === "none" ? "sin corte" : `por ${groupingLabel(saved.groupBy).toLowerCase()}`];
  if (saved.professionalId) parts.push(saved.professionalName || "un profesional");
  if (saved.coverage === "private") parts.push("Particular");
  if (saved.coverage === "insurer") parts.push(saved.insurerName || "Con mutual");
  if (saved.coverage === undefined && saved.insurerName) parts.push(saved.insurerName);
  if (saved.modality) parts.push(MODALITY_LABEL[saved.modality] ?? saved.modality);
  return parts.join(" · ");
}