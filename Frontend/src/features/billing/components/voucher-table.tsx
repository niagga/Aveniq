import Link from "next/link";

import {
  billingHref,
  formatDateTime,
  formatMoney,
  voucherNumber,
  type Voucher,
} from "@/entities/billing/model";
import { StatusBadge } from "@/features/billing/components/voucher-detail";
import { Notice } from "@/shared/ui/notice";

/**
 * The history of vouchers, newest first (the API's order). A voided voucher stays in the
 * list, struck through but readable: the numbering has no gaps to explain.
 */
export function VoucherTable({
  vouchers,
  selectedID,
  filtered,
}: {
  vouchers: Voucher[];
  selectedID: string | null;
  /** Whether a filter is on, so an empty list can say why it is empty. */
  filtered: boolean;
}) {
  if (vouchers.length === 0) {
    return (
      <Notice tone="info" title={filtered ? "Ningún comprobante coincide" : "Todavía no hay comprobantes"}>
        {filtered
          ? "Probá con otro estado o con otro rango de fechas."
          : "Los comprobantes emitidos aparecen acá."}
      </Notice>
    );
  }

  return (
    <div className="overflow-x-auto rounded-[var(--radius-panel)] border border-border bg-surface">
      <table className="w-full text-left text-sm">
        <thead className="bg-surface-muted text-xs uppercase tracking-wide text-ink-muted">
          <tr>
            <th className="px-4 py-3 font-medium">Número</th>
            <th className="px-4 py-3 font-medium">Emitido</th>
            <th className="px-4 py-3 font-medium">Concepto</th>
            <th className="px-4 py-3 font-medium">Comprador</th>
            <th className="px-4 py-3 text-right font-medium">Importe</th>
            <th className="px-4 py-3 font-medium">Estado</th>
            <th className="px-4 py-3 font-medium">
              <span className="sr-only">Acciones</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {vouchers.map((voucher) => {
            const selected = voucher.id === selectedID;
            const voided = voucher.status === "voided";
            return (
              <tr
                key={voucher.id}
                aria-selected={selected}
                className={`border-t border-border align-top ${selected ? "bg-brand/5" : ""}`}
              >
                <td className="px-4 py-3 font-mono text-xs">{voucherNumber(voucher.number)}</td>
                <td className="px-4 py-3 text-ink-muted">{formatDateTime(voucher.issuedAt)}</td>
                <td className={`px-4 py-3 ${voided ? "text-ink-muted line-through" : ""}`}>
                  {voucher.concept}
                </td>
                <td className="px-4 py-3 text-ink-muted">{voucher.buyerName ?? "Sin paciente"}</td>
                <td
                  className={`whitespace-nowrap px-4 py-3 text-right tabular-nums ${
                    voided ? "text-ink-muted line-through" : ""
                  }`}
                >
                  {formatMoney(voucher.amountMinor, voucher.currency)}
                </td>
                <td className="px-4 py-3">
                  <StatusBadge status={voucher.status} />
                </td>
                <td className="px-4 py-3">
                  <Link
                    href={billingHref(voucher.id)}
                    aria-label={`Ver el comprobante ${voucherNumber(voucher.number)}`}
                    className="rounded-md border border-border px-2 py-1 text-xs font-medium"
                  >
                    Ver
                  </Link>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
