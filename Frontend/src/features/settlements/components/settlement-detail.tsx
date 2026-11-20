"use client";

import { useId, useState } from "react";

import {
  formatDateTime,
  formatMoney,
  modalityName,
  periodName,
  sessionCountText,
  settlementStatusLabel,
  totalsText,
  type Settlement,
} from "@/entities/billing/model";
import { apiErrorMessage, browserApi, CONNECTION_ERROR_MESSAGE } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { Field, inputClass } from "@/shared/ui/field";

/** The API's limit for the reason (1 to 500 characters). */
const REASON_MAX = 500;

/**
 * One settlement as it was issued: the mutual and the practice as they were that day, and every
 * session with the amount the mutual pays for it. A voided settlement says so and shows when its
 * sessions were released, so the practice can see which ones it may settle again.
 *
 * A void asks for a reason and for a second click: it is final, the sessions are released, and
 * nothing deletes the settlement.
 */
export function SettlementDetail({
  settlement,
  canVoid,
  onVoided,
  onClose,
}: {
  settlement: Settlement;
  canVoid: boolean;
  onVoided: (settlement: Settlement) => void;
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
        "/api/v1/billing/settlements/{id}/void",
        { params: { path: { id: settlement.id } }, body: { reason: trimmed } },
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

  const voided = settlement.status === "voided";
  const items = settlement.items ?? [];
  const totals = settlement.totals ?? [];

  return (
    <section
      aria-label={`Detalle de la liquidación de ${periodName(settlement.period)}`}
      className="grid gap-4 rounded-[var(--radius-panel)] border border-border bg-surface p-5"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">
            {settlement.insurerName} · {periodName(settlement.period)}
          </h2>
          <p className="mt-1 text-xs text-ink-muted">
            {settlement.practiceName} · emitida el {formatDateTime(settlement.createdAt)} · CUIT{" "}
            {settlement.insurerTaxId}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span
            className={`rounded-full border px-2 py-0.5 text-xs ${
              voided
                ? "border-border bg-surface-muted text-ink-muted"
                : "border-success/30 bg-success/5 text-success"
            }`}
          >
            {settlementStatusLabel(settlement.status)}
          </span>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md border border-border px-2 py-1 text-xs font-medium"
          >
            Cerrar
          </button>
        </div>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead className="text-xs uppercase tracking-wide text-ink-muted">
            <tr>
              <th className="px-3 py-2 font-medium">Fecha</th>
              <th className="px-3 py-2 font-medium">Paciente</th>
              <th className="px-3 py-2 font-medium">Afiliado</th>
              <th className="px-3 py-2 font-medium">Profesional</th>
              <th className="px-3 py-2 font-medium">Modalidad</th>
              <th className="px-3 py-2 text-right font-medium">Importe mutual</th>
              <th className="px-3 py-2 text-right font-medium">Copago (no suma al total)</th>
              <th className="px-3 py-2 font-medium">Estado</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item) => {
              const released = item.releasedAt !== null && item.releasedAt !== undefined;
              return (
                <tr key={item.appointmentId} className="border-t border-border">
                  <td className="px-3 py-2 text-ink-muted">{formatDateTime(item.startsAt)}</td>
                  <td className={`px-3 py-2 ${released ? "text-ink-muted line-through" : ""}`}>
                    {item.patientName}
                  </td>
                  <td className="px-3 py-2 font-mono text-xs">{item.memberNumber}</td>
                  <td className="px-3 py-2 text-ink-muted">{item.professionalName}</td>
                  <td className="px-3 py-2 text-ink-muted">{modalityName(item.modality)}</td>
                  <td
                    className={`whitespace-nowrap px-3 py-2 text-right tabular-nums ${
                      released ? "text-ink-muted line-through" : ""
                    }`}
                  >
                    {formatMoney(item.amountMinor, item.currency)}
                  </td>
                  <td
                    className={`whitespace-nowrap px-3 py-2 text-right tabular-nums text-ink-muted ${
                      released ? "line-through" : ""
                    }`}
                  >
                    {formatMoney(item.copagoMinor, item.currency)}
                  </td>
                  <td className="px-3 py-2 text-xs text-ink-muted">
                    {released ? "Liberada" : "Liquidada"}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <p className="text-sm tabular-nums">
        <span className="text-ink-muted">
          {sessionCountText(items.length)} · Total:{" "}
        </span>
        {totalsText(totals)}
      </p>

      {voided ? (
        <div className="rounded-md border border-border bg-surface-muted p-3 text-sm">
          <p className="font-medium">
            Anulada{settlement.voidedAt ? ` el ${formatDateTime(settlement.voidedAt)}` : ""}
          </p>
          <p className="mt-1 text-ink-muted">Motivo: {settlement.voidReason ?? "—"}</p>
          <p className="mt-1 text-xs text-ink-muted">
            Sus sesiones quedaron libres para una nueva liquidación. La liquidación no se borra.
          </p>
        </div>
      ) : canVoid && !writesLocked ? (
        confirming ? (
          <form onSubmit={handleVoid} noValidate className="grid gap-3">
            <Field
              label="Motivo de la anulación"
              htmlFor={`${prefix}-reason`}
              hint="Libera las sesiones para volver a liquidarlas. La anulación no se puede deshacer."
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
          <div>
            <button
              type="button"
              onClick={() => setConfirming(true)}
              className="rounded-md border border-danger/40 px-3 py-2 text-sm font-medium text-danger"
            >
              Anular liquidación
            </button>
          </div>
        )
      ) : null}

      {error ? (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      ) : null}
    </section>
  );
}
