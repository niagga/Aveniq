"use client";

import Link from "next/link";
import { useMemo, useState } from "react";

import { formatMoney } from "@/entities/billing/model";
import {
  CUTS,
  PRESETS,
  TREND_MONTHS,
  amountIn,
  civilToday,
  countIn,
  currenciesOf,
  monthKeys,
  monthLabel,
  rangeFor,
  rowLabel,
  rowTotal,
  trendRange,
  type BillingReport,
  type Preset,
  type ReportDimension,
} from "@/entities/report/model";
import { canReadTariffs } from "@/entities/tariff/model";
import { BarList, ColumnChart, type ChartItem } from "@/shared/ui/charts";
import { Notice } from "@/shared/ui/notice";
import { Segmented } from "@/shared/ui/segmented";
import { LoadingState } from "@/shared/ui/states";
import { useBillingReport, type ReportState } from "@/features/reports/use-billing-report";

/**
 * The figures of the dashboard: what the practice billed and what it attended without billing.
 *
 * Two reads feed it. The headline figures and the cut chart follow the period the person picks;
 * the evolution chart always shows the last months, because a trend of one month is a dot. The
 * API narrows both to the professional's own sessions when the role is a professional, and says
 * so in `scope`, which the screen repeats so nobody mistakes their own numbers for the practice's.
 *
 * Money of different currencies is never added. The screen shows one currency at a time and
 * offers the others as a switch, only when the data has more than one.
 */
export function FiguresPanel({ zone, role }: { zone: string; role: string | undefined }) {
  const today = useMemo(() => civilToday(zone), [zone]);
  const [preset, setPreset] = useState<Preset>("this-month");
  const [cut, setCut] = useState<ReportDimension>("coverage");
  const [chosenCurrency, setChosenCurrency] = useState<string | null>(null);

  const range = useMemo(() => rangeFor(preset, today), [preset, today]);
  const trend = useMemo(() => trendRange(today), [today]);
  // The headline does not depend on the cut, so it has its own read: switching the cut redraws
  // the chart only, not the figures above it.
  const headline = useBillingReport(range, "none");
  const figures = useBillingReport(range, cut);
  const series = useBillingReport(trend, "month");

  const ownOnly = role === "professional";
  const cuts = ownOnly ? CUTS.filter((option) => option.value !== "professional") : CUTS;

  const currencies = currenciesOf([
    headline.status === "ready" ? headline.report : null,
    figures.status === "ready" ? figures.report : null,
    series.status === "ready" ? series.report : null,
  ]);
  const currency =
    chosenCurrency !== null && currencies.includes(chosenCurrency)
      ? chosenCurrency
      : (currencies[0] ?? "ARS");

  return (
    <section aria-labelledby="figures-title" className="grid gap-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 id="figures-title" className="text-sm font-semibold">
            {ownOnly ? "Tus cifras" : "Cifras del consultorio"}
          </h2>
          <p className="mt-1 text-xs text-ink-muted">
            {ownOnly
              ? "Solo de tus turnos y comprobantes."
              : "Facturado y atendido sin facturar, por moneda y sin conversión."}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {currencies.length > 1 ? (
            <Segmented
              label="Moneda"
              options={currencies.map((code) => ({ value: code, label: code }))}
              value={currency}
              onChange={setChosenCurrency}
            />
          ) : null}
          <Segmented label="Período" options={PRESETS} value={preset} onChange={setPreset} />
        </div>
      </div>

      <Headline state={headline} currency={currency} role={role} />

      <div className="grid gap-4 lg:grid-cols-2">
        <article className="rounded-panel border border-border bg-surface p-5">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h3 className="text-sm font-semibold">Por {cutName(cut)}</h3>
            <Segmented label="Cortar por" options={cuts} value={cut} onChange={setCut} />
          </div>
          <div className="mt-4">
            <CutChart state={figures} cut={cut} currency={currency} />
          </div>
        </article>

        <article className="rounded-panel border border-border bg-surface p-5">
          <h3 className="text-sm font-semibold">Últimos {TREND_MONTHS} meses</h3>
          <div className="mt-4">
            <TrendChart state={series} months={monthKeys(trend)} currency={currency} />
          </div>
        </article>
      </div>
    </section>
  );
}

function cutName(cut: ReportDimension): string {
  return (CUTS.find((option) => option.value === cut)?.label ?? "").toLowerCase();
}

function Headline({
  state,
  currency,
  role,
}: {
  state: ReportState;
  currency: string;
  role: string | undefined;
}) {
  if (state.status === "loading") return <LoadingState label="Cargando las cifras…" rows={1} />;
  if (state.status === "failed") return <FailedNotice />;

  const { totals } = state.report;
  const vouchers = countIn(totals.billed, currency);
  const sessions = countIn(totals.pending, currency);

  return (
    <div className="grid gap-3 sm:grid-cols-3">
      <Figure
        label="Facturado"
        value={formatMoney(amountIn(totals.billed, currency), currency)}
        note={vouchers === 1 ? "1 comprobante emitido" : `${vouchers} comprobantes emitidos`}
      />
      <Figure
        label="Atendido sin facturar"
        value={formatMoney(amountIn(totals.pending, currency), currency)}
        note={
          sessions === 1
            ? "1 sesión valuada con el tarifario"
            : `${sessions} sesiones valuadas con el tarifario`
        }
      />
      <Figure
        label="Sesiones sin tarifa"
        value={String(totals.unvaluedSessions)}
        note={
          totals.unvaluedSessions === 0
            ? "Todas las sesiones atendidas tienen valor."
            : "Atendidas y sin facturar, fuera de los importes de arriba."
        }
        action={
          totals.unvaluedSessions > 0 && canReadTariffs(role) ? (
            <Link href="/tarifario" className="text-xs font-medium text-brand hover:underline">
              Cargar tarifas
            </Link>
          ) : null
        }
      />
    </div>
  );
}

function Figure({
  label,
  value,
  note,
  action,
}: {
  label: string;
  value: string;
  note: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="rounded-panel border border-border bg-surface p-4">
      <p className="text-xs font-medium text-ink-muted">{label}</p>
      <p className="mt-1 text-xl font-semibold tabular-nums">{value}</p>
      <p className="mt-1 text-xs text-ink-muted">{note}</p>
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  );
}

function CutChart({
  state,
  cut,
  currency,
}: {
  state: ReportState;
  cut: ReportDimension;
  currency: string;
}) {
  if (state.status === "loading") return <LoadingState label="Cargando el corte…" rows={3} />;
  if (state.status === "failed") return <FailedNotice />;

  const items: ChartItem[] = (state.report.rows ?? [])
    .filter((row) => rowTotal(row, currency) > 0)
    .sort((a, b) => rowTotal(b, currency) - rowTotal(a, currency))
    .map((row) => ({
      key: row.key || "(sin dato)",
      label: rowLabel(cut, row),
      billed: amountIn(row.billed, currency),
      pending: amountIn(row.pending, currency),
    }));

  return (
    <BarList
      title={`Cifras por ${cutName(cut)}`}
      items={items}
      currency={currency}
      empty="Sin movimientos en este período."
    />
  );
}

function TrendChart({
  state,
  months,
  currency,
}: {
  state: ReportState;
  months: readonly string[];
  currency: string;
}) {
  if (state.status === "loading") return <LoadingState label="Cargando la evolución…" rows={3} />;
  if (state.status === "failed") return <FailedNotice />;

  const items = trendItems(state.report, months, currency);
  const empty = items.every((item) => item.billed === 0 && item.pending === 0);
  if (empty) return <p className="text-sm text-ink-muted">Sin movimientos en estos meses.</p>;

  return (
    <ColumnChart
      title={`Facturado y atendido sin facturar por mes, en ${currency}`}
      items={items}
      currency={currency}
    />
  );
}

/** One item per month of the range, including the months with nothing, so the axis has no gaps. */
function trendItems(
  report: BillingReport,
  months: readonly string[],
  currency: string,
): ChartItem[] {
  const byMonth = new Map((report.rows ?? []).map((row) => [row.key, row]));
  return months.map((key) => {
    const row = byMonth.get(key);
    return {
      key,
      label: monthLabel(key),
      billed: row ? amountIn(row.billed, currency) : 0,
      pending: row ? amountIn(row.pending, currency) : 0,
    };
  });
}

function FailedNotice() {
  return (
    <Notice tone="danger" title="No pudimos leer las cifras">
      La API no responde. Verificá que el backend esté corriendo en 127.0.0.1:8080.
    </Notice>
  );
}
