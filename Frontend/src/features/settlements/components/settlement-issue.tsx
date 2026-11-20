"use client";

import Link from "next/link";
import { useId, useState } from "react";

import {
  formatDateTime,
  formatMoney,
  modalityName,
  periodName,
  sessionCountText,
  totalsText,
  type Settlement,
  type SettlementInsurer,
  type SettlementPreview,
} from "@/entities/billing/model";
import { apiErrorMessage, browserApi, CONNECTION_ERROR_MESSAGE } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { Field, inputClass } from "@/shared/ui/field";
import { Notice } from "@/shared/ui/notice";

/**
 * The monthly settlement to a mutual (T55). The practice picks the mutual and the month, reads
 * what the settlement would contain, and issues it. The server values the sessions for the
 * issue the same way the preview values them, so what the practice reads is what it issues.
 */
export function SettlementIssue({
  insurers,
  onIssued,
}: {
  insurers: SettlementInsurer[];
  onIssued: (settlement: Settlement) => void;
}) {
  const prefix = useId();
  const writesLocked = useWritesLocked();
  const [insurerID, setInsurerID] = useState("");
  const [period, setPeriod] = useState("");
  const [preview, setPreview] = useState<SettlementPreview | null>(null);
  const [issued, setIssued] = useState<Settlement | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<"preview" | "issue" | null>(null);

  async function handlePreview(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setIssued(null);
    setPreview(null);
    if (insurerID === "" || period === "") {
      setError("Elegí la mutual y el mes.");
      return;
    }

    setBusy("preview");
    try {
      const { data, error: apiError, response } = await browserApi.GET(
        "/api/v1/billing/settlements/preview",
        { params: { query: { insurerId: insurerID, period } } },
      );
      if (!response.ok || !data) {
        setError(apiErrorMessage(apiError));
        return;
      }
      setPreview(data);
    } catch {
      setError(CONNECTION_ERROR_MESSAGE);
    } finally {
      setBusy(null);
    }
  }

  async function handleIssue() {
    if (preview === null) return;
    setError(null);
    setBusy("issue");
    try {
      const { data, error: apiError, response } = await browserApi.POST(
        "/api/v1/billing/settlements",
        { body: { insurerId: insurerID, period } },
      );
      if (!response.ok || !data) {
        setError(apiErrorMessage(apiError));
        return;
      }
      setPreview(null);
      setIssued(data);
      onIssued(data);
    } catch {
      setError(CONNECTION_ERROR_MESSAGE);
    } finally {
      setBusy(null);
    }
  }

  const sessions = preview?.sessions ?? [];
  const unvalued = preview?.unvalued ?? [];
  const totals = preview?.totals ?? [];
  const issuable = sessions.length > 0;

  return (
    <section aria-label="Nueva liquidación" className="grid gap-4">
      <h2 className="text-sm font-semibold">Nueva liquidación</h2>

      {insurers.length === 0 ? (
        <Notice tone="info" title="Todavía no hay mutuales">
          Cargá la mutual con su CUIT en <Link href="/mutuales" className="underline">Mutuales</Link>{" "}
          para liquidarle.
        </Notice>
      ) : null}

      <form
        onSubmit={handlePreview}
        noValidate
        className="grid gap-4 rounded-[var(--radius-panel)] border border-border bg-surface p-4 sm:grid-cols-[1fr_12rem_auto] sm:items-end"
      >
        <Field label="Mutual" htmlFor={`${prefix}-insurer`}>
          <select
            id={`${prefix}-insurer`}
            value={insurerID}
            onChange={(event) => {
              setInsurerID(event.target.value);
              setPreview(null);
            }}
            className={inputClass}
          >
            <option value="">Elegí una mutual</option>
            {insurers.map((insurer) => (
              <option key={insurer.id} value={insurer.id} disabled={!insurer.settlable}>
                {insurer.settlable ? insurer.name : `${insurer.name} (sin CUIT)`}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Mes" htmlFor={`${prefix}-period`}>
          <input
            id={`${prefix}-period`}
            type="month"
            value={period}
            onChange={(event) => {
              setPeriod(event.target.value);
              setPreview(null);
            }}
            className={inputClass}
          />
        </Field>
        <button
          type="submit"
          disabled={busy !== null}
          className="rounded-md border border-border px-3 py-2 text-sm font-medium disabled:opacity-60"
        >
          {busy === "preview" ? "Calculando…" : "Ver sesiones"}
        </button>
      </form>

      {error ? (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      ) : null}

      {issued ? (
        <Notice tone="info" title={`Liquidación emitida: ${periodName(issued.period)}`}>
          {issued.insurerName} · {sessionCountText((issued.items ?? []).length)} · {totalsText(issued.totals ?? [])}
        </Notice>
      ) : null}

      {preview !== null ? (
        <div className="grid gap-4 rounded-[var(--radius-panel)] border border-border bg-surface p-4">
          <div className="flex flex-wrap items-baseline justify-between gap-2">
            <h3 className="text-sm font-semibold">
              {preview.insurerName} · {periodName(preview.period)}
            </h3>
            <p className="text-sm tabular-nums">
              <span className="text-ink-muted">Total a liquidar: </span>
              {totalsText(totals)}
            </p>
          </div>

          {sessions.length === 0 ? (
            <Notice tone="info" title="No hay sesiones para liquidar">
              Ninguna sesión atendida de esta mutual en el mes está pendiente de liquidación.
            </Notice>
          ) : (
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
                  </tr>
                </thead>
                <tbody>
                  {sessions.map((session) => (
                    <tr key={session.appointmentId} className="border-t border-border">
                      <td className="px-3 py-2 text-ink-muted">{formatDateTime(session.startsAt)}</td>
                      <td className="px-3 py-2">{session.patientName}</td>
                      <td className="px-3 py-2 font-mono text-xs">{session.memberNumber}</td>
                      <td className="px-3 py-2 text-ink-muted">{session.professionalName}</td>
                      <td className="px-3 py-2 text-ink-muted">{modalityName(session.modality)}</td>
                      <td className="whitespace-nowrap px-3 py-2 text-right tabular-nums">
                        {formatMoney(session.amountMinor, session.currency)}
                      </td>
                      <td className="whitespace-nowrap px-3 py-2 text-right tabular-nums text-ink-muted">
                        {formatMoney(session.copagoMinor, session.currency)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {unvalued.length > 0 ? (
            <Notice
              tone="warning"
              title={`${sessionCountText(unvalued.length)} sin tarifa para esta mutual`}
            >
              No entran en la liquidación hasta que el tarifario tenga su valor:{" "}
              {unvalued
                .map((session) => `${session.patientName} (${formatDateTime(session.startsAt)})`)
                .join(", ")}
              .
            </Notice>
          ) : null}

          <div className="flex flex-wrap items-center gap-3">
            <button
              type="button"
              onClick={() => void handleIssue()}
              disabled={!issuable || busy !== null || writesLocked}
              className="rounded-md bg-brand px-3 py-2 text-sm font-medium text-white disabled:opacity-60"
            >
              {busy === "issue" ? "Emitiendo…" : "Emitir liquidación"}
            </button>
            <p className="text-xs text-ink-muted">
              Emitida, la liquidación congela estos importes y deja sus sesiones liquidadas. Solo se
              corrige anulándola.
            </p>
          </div>
        </div>
      ) : null}
    </section>
  );
}
