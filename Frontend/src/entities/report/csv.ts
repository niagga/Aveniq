import {
  amountIn,
  countIn,
  currenciesOf,
  groupingLabel,
  rowLabel,
  type BillingReport,
  type ReportMoney,
} from "@/entities/report/model";

/**
 * The CSV export of a report (T60), built in the browser from the figures already on screen.
 *
 * Nothing is asked of the server again, so the file is exactly what the person sees, narrowed the
 * same way for a professional. It is written for a spreadsheet in Spanish: `;` between cells,
 * a decimal comma, a byte-order mark so Excel reads the accents, and CRLF line ends.
 *
 * One row per group and currency, because money of different currencies is never added. The
 * sessions that no tariff values are counted once per group, on its first row, so summing that
 * column over the groups gives the true count. The last line, "Total", repeats the sums of the
 * range: leave it out when adding the groups up.
 */

const SEPARATOR = ";";
const LINE_END = "\r\n";
const BOM = "\uFEFF";

/** Characters that make a spreadsheet read a cell as a formula. */
const FORMULA_START = /^[=+\-@\t\r]/;

/**
 * One text cell. A name typed by a person ("=HYPERLINK(...)") must stay text when the file is
 * opened, so a cell that would start a formula gets a leading apostrophe; a cell that holds the
 * separator, a quote or a line break is quoted.
 */
export function csvText(value: string): string {
  const safe = FORMULA_START.test(value) ? `'${value}` : value;
  if (/[;"\r\n]/.test(safe)) return `"${safe.replaceAll('"', '""')}"`;
  return safe;
}

/** An amount in minor units as a decimal with a comma and no thousands separator: 150000 → 1500,00. */
export function csvAmount(minor: number): string {
  const absolute = Math.abs(minor);
  const whole = Math.floor(absolute / 100);
  const cents = String(absolute % 100).padStart(2, "0");
  return `${minor < 0 ? "-" : ""}${whole},${cents}`;
}

const HEADER = [
  "Grupo",
  "Moneda",
  "Facturado",
  "Comprobantes",
  "Atendido sin facturar",
  "Sesiones valuadas",
  "Sesiones sin tarifa",
];

type Figures = {
  billed: readonly ReportMoney[] | null;
  pending: readonly ReportMoney[] | null;
  unvaluedSessions: number;
};

function rowsFor(group: string, figures: Figures, currencies: readonly string[]): string[][] {
  const used = currencies.filter(
    (currency) => countIn(figures.billed, currency) + countIn(figures.pending, currency) > 0,
  );
  // A group with no money still has a line, so the file says it exists and what it lacks.
  if (used.length === 0) {
    return [[csvText(group), "", "", "0", "", "0", String(figures.unvaluedSessions)]];
  }
  return used.map((currency, index) => [
    csvText(group),
    csvText(currency),
    csvAmount(amountIn(figures.billed, currency)),
    String(countIn(figures.billed, currency)),
    csvAmount(amountIn(figures.pending, currency)),
    String(countIn(figures.pending, currency)),
    index === 0 ? String(figures.unvaluedSessions) : "0",
  ]);
}

/** The whole file: a header, a line per group and currency, then the totals of the range. */
export function reportToCsv(report: BillingReport): string {
  const currencies = currenciesOf([report]);
  const lines: string[][] = [HEADER.map(csvText)];

  for (const row of report.rows ?? []) {
    lines.push(...rowsFor(rowLabel(report.groupBy, row), row, currencies));
  }
  lines.push(...rowsFor("Total", report.totals, currencies));

  return BOM + lines.map((cells) => cells.join(SEPARATOR)).join(LINE_END) + LINE_END;
}

/** The name of the downloaded file: what was asked and the days it covers. */
export function csvFilename(report: BillingReport): string {
  const cut = report.groupBy === "none" ? "total" : `por-${slug(groupingLabel(report.groupBy))}`;
  return `informe-${cut}-${report.from}_${report.to}.csv`;
}

function slug(text: string): string {
  return text
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-");
}

/** Hands the file to the browser as a download. Browser only. */
export function downloadCsv(filename: string, content: string): void {
  const url = URL.createObjectURL(new Blob([content], { type: "text/csv;charset=utf-8" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}
