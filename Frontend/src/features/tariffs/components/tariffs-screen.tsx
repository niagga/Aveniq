"use client";

import { useState } from "react";

import { formatMoney } from "@/entities/billing/model";
import {
  canEditTariffs,
  coverageLabel,
  formatValidFrom,
  modalityLabel,
  type Tariff,
} from "@/entities/tariff/model";
import { TariffForm } from "@/features/tariffs/components/tariff-form";
import { useTariffs } from "@/features/tariffs/use-tariffs";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { getSessionUser } from "@/shared/auth/session";
import { ConfirmDialog } from "@/shared/ui/confirm-dialog";
import { Notice } from "@/shared/ui/notice";

/**
 * The fee schedule (T53): what one session is worth, per professional, coverage, modality and
 * currency, from the day each value starts. Owner and admin write it; the accountant reads it.
 *
 * A combination without a value has no row here, and nothing is proposed for it: the amount is
 * typed at emission (T54). Removing a value does not touch the vouchers already issued.
 */
export function TariffsScreen() {
  const role = getSessionUser()?.role;
  const canEdit = canEditTariffs(role);
  const { tariffs, failedStatus, reload } = useTariffs(true);
  const writesLocked = useWritesLocked();

  const [mode, setMode] = useState<"idle" | "create" | "edit">("idle");
  const [editing, setEditing] = useState<Tariff | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busyID, setBusyID] = useState<string | null>(null);
  // The removal used window.confirm: the one destructive action in the app that asked, with
  // an idiom that cannot be styled, translated or tested like the rest.
  const [confirming, setConfirming] = useState<Tariff | null>(null);

  function startEdit(tariff: Tariff) {
    setEditing(tariff);
    setMode("edit");
  }

  function closeForm() {
    setMode("idle");
    setEditing(null);
  }

  function toggleCreate() {
    if (mode === "create") {
      closeForm();
      return;
    }
    setEditing(null);
    setMode("create");
  }

  async function remove(tariff: Tariff) {
    setActionError(null);
    setBusyID(tariff.id);
    try {
      const { error, response } = await browserApi.DELETE("/api/v1/tariffs/{id}", {
        params: { path: { id: tariff.id } },
      });
      if (!response.ok) {
        setActionError(apiErrorMessage(error));
        return;
      }
      if (editing?.id === tariff.id) closeForm();
      void reload();
    } catch {
      setActionError("No pudimos contactar al servidor.");
    } finally {
      setBusyID(null);
    }
  }

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Tarifario</h1>
          <p className="mt-1 text-sm text-ink-muted">
            Cuánto vale una sesión por profesional, cobertura, modalidad y moneda, desde cada día de
            vigencia. Una mutual tiene dos partes: lo que paga la mutual y el copago del paciente.
          </p>
        </div>
        {canEdit ? (
          <button
            type="button"
            onClick={toggleCreate}
            disabled={writesLocked}
            className="rounded-md bg-brand px-4 py-2 text-sm font-medium text-brand-ink disabled:opacity-60"
          >
            {mode === "create" ? "Cerrar formulario" : "Nuevo valor"}
          </button>
        ) : null}
      </header>

      {canEdit && mode !== "idle" && !writesLocked ? (
        <div className="mt-6">
          <TariffForm
            key={editing?.id ?? "new"}
            tariff={mode === "edit" ? editing : null}
            onSaved={() => {
              closeForm();
              void reload();
            }}
            onCancel={closeForm}
          />
        </div>
      ) : null}

      <div className="mt-6">
        {failedStatus !== null ? (
          <Notice tone="danger" title="No pudimos leer el tarifario">
            {failedStatus === 0
              ? "La API no responde. Verificá que el backend esté corriendo en 127.0.0.1:8080."
              : `La API respondió ${failedStatus}.`}
          </Notice>
        ) : tariffs === null ? (
          <p role="status" className="text-sm text-ink-muted">
            Cargando tarifario…
          </p>
        ) : tariffs.length === 0 ? (
          <Notice tone="info" title="Todavía no hay valores cargados">
            {canEdit
              ? "Cargá el primer valor con el botón de arriba. Sin valor para una combinación, el importe se escribe a mano al emitir."
              : "El owner o el admin cargan los valores desde esta pantalla."}
          </Notice>
        ) : (
          <div className="overflow-x-auto rounded-[var(--radius-panel)] border border-border bg-surface">
            {actionError ? (
              <p role="alert" className="border-b border-border px-4 py-2 text-sm text-danger">
                {actionError}
              </p>
            ) : null}
            <table className="w-full text-left text-sm">
              <thead className="bg-surface-muted text-xs uppercase tracking-wide text-ink-muted">
                <tr>
                  <th scope="col" className="px-4 py-3 font-medium">Profesional</th>
                  <th scope="col" className="px-4 py-3 font-medium">Cobertura</th>
                  <th scope="col" className="px-4 py-3 font-medium">Modalidad</th>
                  <th scope="col" className="px-4 py-3 font-medium">Moneda</th>
                  <th scope="col" className="px-4 py-3 font-medium">Vigente desde</th>
                  <th scope="col" className="px-4 py-3 text-right font-medium">Paga la mutual</th>
                  <th scope="col" className="px-4 py-3 text-right font-medium">Paga el paciente</th>
                  <th scope="col" className="px-4 py-3 text-right font-medium">Total</th>
                  <th className="px-4 py-3 font-medium">
                    <span className="sr-only">Acciones</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {tariffs.map((tariff) => (
                  <tr key={tariff.id} className="border-t border-border">
                    <td className="px-4 py-3 font-medium">{tariff.professionalName}</td>
                    <td className="px-4 py-3">{coverageLabel(tariff)}</td>
                    <td className="px-4 py-3">{modalityLabel(tariff.modality)}</td>
                    <td className="px-4 py-3 font-mono text-xs">{tariff.billingCurrency}</td>
                    <td className="px-4 py-3 text-ink-muted">{formatValidFrom(tariff.validFrom)}</td>
                    <td className="px-4 py-3 text-right font-mono text-xs">
                      {tariff.coverage === "insurer"
                        ? formatMoney(tariff.insurerMinor, tariff.billingCurrency)
                        : "—"}
                    </td>
                    <td className="px-4 py-3 text-right font-mono text-xs">
                      {formatMoney(tariff.patientMinor, tariff.billingCurrency)}
                    </td>
                    <td className="px-4 py-3 text-right font-mono text-xs font-semibold">
                      {formatMoney(tariff.totalMinor, tariff.billingCurrency)}
                    </td>
                    <td className="px-4 py-3 text-right">
                      {canEdit ? (
                        <div className="flex justify-end gap-2">
                          <button
                            type="button"
                            onClick={() => startEdit(tariff)}
                            disabled={writesLocked}
                            className="rounded-md border border-border px-3 py-1.5 text-xs font-medium disabled:opacity-60"
                          >
                            Editar importes
                          </button>
                          <button
                            type="button"
                            onClick={() => setConfirming(tariff)}
                            disabled={busyID === tariff.id || writesLocked}
                            className="rounded-control border border-border px-3 py-1.5 text-xs font-medium text-ink-muted disabled:opacity-60"
                          >
                            {busyID === tariff.id ? "Quitando…" : "Quitar"}
                          </button>
                          <ConfirmDialog
                            open={confirming?.id === tariff.id}
                            title="¿Quitar este valor del tarifario?"
                            description="Deja de ofrecerse en las altas nuevas. Los comprobantes ya emitidos conservan el importe con el que se emitieron."
                            confirmLabel="Quitar"
                            tone="danger"
                            busy={busyID === tariff.id}
                            onCancel={() => setConfirming(null)}
                            onConfirm={() => {
                              const target = confirming;
                              setConfirming(null);
                              if (target !== null) void remove(target);
                            }}
                          />
                        </div>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </>
  );
}
