"use client";

import { useState } from "react";

import { formatMoney } from "@/entities/billing/model";
import { csvFilename, downloadCsv, reportToCsv } from "@/entities/report/csv";
import {
  amountIn,
  countIn,
  currenciesOf,
  groupingLabel,
  rowLabel,
  type BillingReport,
  type DayRange,
  type ReportDimension,
  type ReportFilters,
  type ReportRow,
} from "@/entities/report/model";
import { useBillingReport } from "@/features/reports/use-billing-report";
import { Button } from "@/shared/ui/button";
import { Notice } from "@/shared/ui/notice";
import { Segmented } from "@/shared/ui/segmented";
import { EmptyState, LoadingState } from "@/shared/ui/states";

/**
 * The figures of the report the person composed: a table of the groups of the cut and the total
 * of the range, in one currency at a time, and the CSV of exactly what is on screen.
 *
 * It is rendered only for a range that is valid, so the request never goes out with a day that
 * does not exist. The API narrows a professional to their own figures; `scope` says so and the
 * table repeats it, so nobody mistakes their own numbers for the practice's.
 */
export function ReportResults({
  range,
  groupBy,
  filters,
}: {
  range: DayRange;
  groupBy: ReportDimension;
  filters: ReportFilters;
}) {
  const state = useBillingReport(range, groupBy, filters);
  const [chosenCurrency, setChosenCurrency] = useState<string | null>(null);

  if (state.status === "loading") return <LoadingState label="Calculando el informe…" rows={4} />;
  if (state.status === "failed") {
    return (
      <Notice tone="danger" title="No pudimos calcular el informe">
        La API no responde o rechazó los filtros. Probá de nuevo en unos segundos.
      </Notice>
    );
  }

  const { report } = state;
  const currencies = currenciesOf([report]);
  const currency =
    chosenCurrency !== null && currencies.includes(chosenCurrency)
      ? chosenCurrency
      : (currencies[0] ?? "ARS");

  const rows = (report.rows ?? []).filter((row) => hasMovement(row, currency));
  const empty = currencies.length === 0 && report.totals.unvaluedSessions === 0;

  return (
    <section aria-label="Resultado del informe" className="grid gap-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-sm text-ink-muted">
          {report.scope === "own"
            ? "Solo tus turnos y comprobantes. "
            : "Todo el consultorio. "}
          {report.from} a {report.to}
          {report.groupBy === "none" ? "." : `, por ${groupingLabel(report.groupBy).toLowerCase()}.`}
        </p>
        <div className="flex flex-wrap items-center gap-2">
          {currencies.length > 1 ? (
            <Segmented
              label="Moneda"
              options={currencies.map((code) => ({ value: code, label: code }))}
              value={currency}
              onChange={setChosenCurrency}
            />
          ) : null}
          <Button
            size="sm"
            onClick={() => downloadCsv(csvFilename(report), reportToCsv(report))}
          >
            Exportar CSV
          </Button>
        </div>
      </div>

      {empty ? (
        <EmptyState
          title="Sin movimientos"
          description="No hay comprobantes ni sesiones atendidas sin facturar con estos filtros."
        />
      ) : (
        <ResultTable report={report} rows={rows} currency={currency} />
      )}

      {currencies.length > 1 ? (
        <p className="text-xs text-ink-muted">
          Cada moneda se muestra por separado y nunca se suman entre sí. El CSV las incluye todas.
        </p>
      ) : null}
    </section>
  );
}

/** A group belongs in the table of a currency when it moved money in it, or has sessions to value. */
function hasMovement(row: ReportRow, currency: string): boolean {
  return (
    countIn(row.billed, currency) + countIn(row.pending, currency) > 0 || row.unvaluedSessions > 0
  );
}

function ResultTable({
  report,
  rows,
  currency,
}: {
  report: BillingReport;
  rows: readonly ReportRow[];
  currency: string;
}) {
  const grouped = report.groupBy !== "none";

  return (
    <div className="overflow-x-auto rounded-panel border border-border bg-surface">
      <table className="w-full text-left text-sm">
        <caption className="sr-only">
          Facturado y atendido sin facturar en {currency}
          {grouped ? `, por ${groupingLabel(report.groupBy).toLowerCase()}` : ""}
        </caption>
        <thead className="bg-surface-muted text-xs uppercase tracking-wide text-ink-muted">
          <tr>
            <th scope="col" className="px-4 py-3 font-medium">
              {grouped ? groupingLabel(report.groupBy) : "Período"}
            </th>
            <th scope="col" className="px-4 py-3 text-right font-medium">Facturado</th>
            <th scope="col" className="px-4 py-3 text-right font-medium">Comprobantes</th>
            <th scope="col" className="px-4 py-3 text-right font-medium">Atendido sin facturar</th>
            <th scope="col" className="px-4 py-3 text-right font-medium">Sesiones valuadas</th>
            <th scope="col" className="px-4 py-3 text-right font-medium">Sin tarifa</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <FiguresRow
              key={row.key || "(sin dato)"}
              label={rowLabel(report.groupBy, row)}
              figures={row}
              currency={currency}
            />
          ))}
          <FiguresRow
            label={grouped ? "Total" : `${report.from} a ${report.to}`}
            figures={report.totals}
            currency={currency}
            total
          />
        </tbody>
      </table>
    </div>
  );
}

function FiguresRow({
  label,
  figures,
  currency,
  total = false,
}: {
  label: string;
  figures: {
    billed: BillingReport["totals"]["billed"];
    pending: BillingReport["totals"]["pending"];
    unvaluedSessions: number;
  };
  currency: string;
  total?: boolean;
}) {
  return (
    <tr className={`border-t border-border ${total ? "bg-surface-muted font-semibold" : ""}`}>
      <th scope="row" className="px-4 py-3 text-left font-medium">
        {label}
      </th>
      <td className="whitespace-nowrap px-4 py-3 text-right font-mono text-xs">
        {formatMoney(amountIn(figures.billed, currency), currency)}
      </td>
      <td className="px-4 py-3 text-right tabular-nums">{countIn(figures.billed, currency)}</td>
      <td className="whitespace-nowrap px-4 py-3 text-right font-mono text-xs">
        {formatMoney(amountIn(figures.pending, currency), currency)}
      </td>
      <td className="px-4 py-3 text-right tabular-nums">{countIn(figures.pending, currency)}</td>
      <td className="px-4 py-3 text-right tabular-nums">{figures.unvaluedSessions}</td>
    </tr>
  );
}
