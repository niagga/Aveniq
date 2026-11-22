import { formatMoney } from "@/entities/billing/model";

/**
 * The two charts of the dashboard, in SVG and CSS. There is no chart library on purpose: the
 * app takes no new dependency for two shapes (L61).
 *
 * Every chart carries the same two series, what was billed and what was attended without being
 * billed. A colour is never the only signal: the legend names both, every bar states its value
 * as text or in a title, and a screen reader gets the numbers as a table.
 *
 * The amounts are minor units of ONE currency. The caller picks the currency; a chart never
 * mixes two, because the MVP has no exchange rate.
 */

export type ChartItem = {
  key: string;
  label: string;
  billed: number;
  pending: number;
};

export function ChartLegend() {
  return (
    <ul className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-ink-muted" aria-label="Referencias">
      <li className="flex items-center gap-1.5">
        <span aria-hidden="true" className="inline-block h-2.5 w-2.5 rounded-sm bg-brand" />
        Facturado
      </li>
      <li className="flex items-center gap-1.5">
        <span aria-hidden="true" className="inline-block h-2.5 w-2.5 rounded-sm bg-warning/60" />
        Atendido sin facturar
      </li>
    </ul>
  );
}

/** The accessible form of a chart: the same numbers, as a table nobody sees. */
function ChartTable({
  caption,
  items,
  currency,
}: {
  caption: string;
  items: readonly ChartItem[];
  currency: string;
}) {
  return (
    <table className="sr-only">
      <caption>{caption}</caption>
      <thead>
        <tr>
          <th scope="col">Grupo</th>
          <th scope="col">Facturado</th>
          <th scope="col">Atendido sin facturar</th>
        </tr>
      </thead>
      <tbody>
        {items.map((item) => (
          <tr key={item.key}>
            <th scope="row">{item.label}</th>
            <td>{formatMoney(item.billed, currency)}</td>
            <td>{formatMoney(item.pending, currency)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/**
 * The top of a scale that ends on a round number: 1, 2 or 5 times a power of ten. Without it the
 * axis would end on whatever the largest value happens to be, and the gridlines would read
 * 183.400 and 366.800.
 */
export function niceCeiling(value: number): number {
  if (value <= 0) return 1;
  const magnitude = 10 ** Math.floor(Math.log10(value));
  const normalized = value / magnitude;
  const step = normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10;
  return step * magnitude;
}

const AXIS_FORMAT = new Intl.NumberFormat("es-AR", {
  notation: "compact",
  maximumFractionDigits: 1,
});

/** An axis label from minor units: "1,5 M", "300 mil". */
function axisLabel(amountMinor: number): string {
  return AXIS_FORMAT.format(amountMinor / 100);
}

// The viewBox is close to the size the chart is drawn at (a half-width card), so the text keeps
// a readable size when the SVG scales down to its container.
const WIDTH = 480;
const HEIGHT = 240;
const MARGIN = { top: 10, right: 8, bottom: 28, left: 46 };
const FONT = 12;

/**
 * Billed and pending, side by side, for each period. The scale starts at zero: a truncated axis
 * would make a small difference look like a collapse.
 */
export function ColumnChart({
  title,
  items,
  currency,
}: {
  title: string;
  items: readonly ChartItem[];
  currency: string;
}) {
  const plotWidth = WIDTH - MARGIN.left - MARGIN.right;
  const plotHeight = HEIGHT - MARGIN.top - MARGIN.bottom;
  const top = niceCeiling(Math.max(0, ...items.flatMap((item) => [item.billed, item.pending])));
  const slot = plotWidth / Math.max(items.length, 1);
  const bar = Math.min(24, slot * 0.34);
  const y = (value: number) => MARGIN.top + plotHeight - (value / top) * plotHeight;
  const ticks = [0, top / 2, top];

  return (
    <figure className="grid gap-2">
      <svg
        viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
        role="img"
        aria-label={title}
        className="h-auto w-full max-w-full"
      >
        <title>{title}</title>
        {ticks.map((tick) => (
          <g key={tick}>
            <line
              x1={MARGIN.left}
              x2={WIDTH - MARGIN.right}
              y1={y(tick)}
              y2={y(tick)}
              className="stroke-border"
              strokeWidth={1}
            />
            <text
              x={MARGIN.left - 6}
              y={y(tick)}
              textAnchor="end"
              dominantBaseline="middle"
              className="fill-ink-muted"
              fontSize={FONT}
            >
              {axisLabel(tick)}
            </text>
          </g>
        ))}
        {items.map((item, index) => {
          const center = MARGIN.left + slot * index + slot / 2;
          return (
            <g key={item.key}>
              <rect
                x={center - bar - 1}
                y={y(item.billed)}
                width={bar}
                height={Math.max(MARGIN.top + plotHeight - y(item.billed), 0)}
                rx={2}
                className="fill-brand"
              >
                <title>{`${item.label}, facturado: ${formatMoney(item.billed, currency)}`}</title>
              </rect>
              <rect
                x={center + 1}
                y={y(item.pending)}
                width={bar}
                height={Math.max(MARGIN.top + plotHeight - y(item.pending), 0)}
                rx={2}
                className="fill-warning/60"
              >
                <title>{`${item.label}, atendido sin facturar: ${formatMoney(item.pending, currency)}`}</title>
              </rect>
              <text
                x={center}
                y={HEIGHT - 8}
                textAnchor="middle"
                className="fill-ink-muted"
                fontSize={FONT}
              >
                {item.label}
              </text>
            </g>
          );
        })}
      </svg>
      <ChartLegend />
      <ChartTable caption={title} items={items} currency={currency} />
    </figure>
  );
}

/**
 * One bar per group, billed and pending stacked and sized against the largest group. The two
 * amounts are written under the bar: the bar says "how much compared with the others", the text
 * says how much.
 */
export function BarList({
  title,
  items,
  currency,
  empty,
}: {
  title: string;
  items: readonly ChartItem[];
  currency: string;
  empty: string;
}) {
  const largest = Math.max(0, ...items.map((item) => item.billed + item.pending));
  if (items.length === 0 || largest === 0) {
    return <p className="text-sm text-ink-muted">{empty}</p>;
  }
  const share = (value: number) => `${(value / largest) * 100}%`;

  return (
    <figure className="grid gap-3">
      <ul className="grid gap-3" aria-label={title}>
        {items.map((item) => (
          <li key={item.key}>
            <div className="flex items-baseline justify-between gap-3 text-sm">
              <span className="min-w-0 truncate font-medium">{item.label}</span>
              <span className="shrink-0 tabular-nums">{formatMoney(item.billed, currency)}</span>
            </div>
            <div
              aria-hidden="true"
              className="mt-1 flex h-2.5 overflow-hidden rounded-full bg-surface-muted"
            >
              <div className="bg-brand" style={{ width: share(item.billed) }} />
              <div className="bg-warning/60" style={{ width: share(item.pending) }} />
            </div>
            {item.pending > 0 ? (
              <p className="mt-0.5 text-xs text-ink-muted">
                Sin facturar {formatMoney(item.pending, currency)}
              </p>
            ) : null}
          </li>
        ))}
      </ul>
      <ChartLegend />
    </figure>
  );
}
