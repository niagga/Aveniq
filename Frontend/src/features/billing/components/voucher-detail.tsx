"use client";

import { useId, useState } from "react";

import {
  formatDateTime,
  formatMoney,
  statusLabel,
  voucherNumber,
  type Voucher,
} from "@/entities/billing/model";
import { apiErrorMessage, browserApi, CONNECTION_ERROR_MESSAGE } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { Field, inputClass } from "@/shared/ui/field";

/** The API's limit for the reason (1 to 500 characters). */
const REASON_MAX = 500;

/**
 * One voucher, as it was issued: the snapshot of the practice and the buyer, not their
 * current records, so a later edit of the patient cannot rewrite a voucher that exists.
 *
 * Voiding is offered only to a role that may void and only while the voucher is issued. It
 * asks for a reason and for a second click, because a void is final: the voucher keeps its
 * number and nothing brings it back.
 */
export function VoucherDetail({
  voucher,
  canVoid,
  onVoided,
  onClose,
}: {
  voucher: Voucher;
  canVoid: boolean;
  onVoided: (voucher: Voucher) => void;
  onClose: () => void;
}) {
  const prefix = useId();
  const writesLocked = useWritesLocked();
  const [confirming, setConfirming] = useState(false);
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function handleVoid(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);

    const trimmed = reason.trim();
    if (trimmed === "") {
      setError("Escribí el motivo de la anulación.");
      return;
    }

    setBusy(true);
    try {
      const { data, error: apiError, response } = await browserApi.POST(
        "/api/v1/billing/vouchers/{id}/void",
        { params: { path: { id: voucher.id } }, body: { reason: trimmed } },
      );
      if (!response.ok || !data) {
        setError(apiErrorMessage(apiError));
        return;
      }
      setConfirming(false);
      setReason("");
      onVoided(data);
    } catch {
      setError(CONNECTION_ERROR_MESSAGE);
    } finally {
      setBusy(false);
    }
  }

  const voided = voucher.status === "voided";

  return (
    <section
      aria-label={`Detalle del comprobante ${voucherNumber(voucher.number)}`}
      className="rounded-[var(--radius-panel)] border border-border bg-surface p-5"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">Comprobante {voucherNumber(voucher.number)}</h2>
          <p className="mt-1 text-xs text-ink-muted">
            {voucher.practiceName} · emitido el {formatDateTime(voucher.issuedAt)}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <StatusBadge status={voucher.status} />
          <button
            type="button"
            onClick={onClose}
            className="rounded-md border border-border px-2 py-1 text-xs font-medium"
          >
            Cerrar
          </button>
        </div>
      </div>

      <dl className="mt-4 grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2">
        <Item label="Concepto" value={voucher.concept} />
        <Item label="Importe" value={formatMoney(voucher.amountMinor, voucher.currency)} />
        <Item label="Comprador" value={voucher.buyerName ?? "Cobro sin paciente"} />
        <Item label="CUIT / CUIL" value={voucher.buyerTaxId ?? "—"} />
        <Item label="Condición fiscal" value={voucher.buyerFiscalCondition ?? "—"} />
      </dl>

      {voided ? (
        <div className="mt-4 rounded-md border border-border bg-surface-muted p-3 text-sm">
          <p className="font-medium">
            Anulado{voucher.voidedAt ? ` el ${formatDateTime(voucher.voidedAt)}` : ""}
          </p>
          <p className="mt-1 text-ink-muted">Motivo: {voucher.voidReason ?? "—"}</p>
          <p className="mt-1 text-xs text-ink-muted">
            El comprobante conserva su número. No se reutiliza.
          </p>
        </div>
      ) : canVoid && !writesLocked ? (
        confirming ? (
          <form onSubmit={handleVoid} noValidate className="mt-4 grid gap-3">
            <Field
              label="Motivo de la anulación"
              htmlFor={`${prefix}-reason`}
              hint="Queda registrado junto al comprobante. La anulación no se puede deshacer."
            >
              <textarea
                id={`${prefix}-reason`}
                value={reason}
                onChange={(event) => setReason(event.target.value)}
                maxLength={REASON_MAX}
                rows={3}
                className={inputClass}
              />
            </Field>
            <div className="flex flex-wrap gap-2">
              <button
                type="submit"
                disabled={busy}
                className="rounded-md bg-danger px-3 py-2 text-sm font-medium text-white disabled:opacity-60"
              >
                {busy ? "Anulando…" : "Confirmar anulación"}
              </button>
              <button
                type="button"
                disabled={busy}
                onClick={() => {
                  setConfirming(false);
                  setError(null);
                }}
                className="rounded-md border border-border px-3 py-2 text-sm font-medium disabled:opacity-60"
              >
                Volver
              </button>
            </div>
          </form>
        ) : (
          <div className="mt-4">
            <button
              type="button"
              onClick={() => setConfirming(true)}
              className="rounded-md border border-danger/40 px-3 py-2 text-sm font-medium text-danger"
            >
              Anular comprobante
            </button>
          </div>
        )
      ) : null}

      {error ? (
        <p role="alert" className="mt-3 text-sm text-danger">
          {error}
        </p>
      ) : null}
    </section>
  );
}

function Item({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs font-medium text-ink-muted">{label}</dt>
      <dd className="mt-0.5">{value}</dd>
    </div>
  );
}

export function StatusBadge({ status }: { status: string }) {
  return (
    <span
      className={`rounded-full border px-2 py-0.5 text-xs ${
        status === "issued"
          ? "border-success/30 bg-success/5 text-success"
          : "border-border bg-surface-muted text-ink-muted"
      }`}
    >
      {statusLabel(status)}
    </span>
  );
}
