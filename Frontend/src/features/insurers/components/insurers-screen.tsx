"use client";

import { useState } from "react";

import type { Insurer } from "@/entities/insurer/model";
import { InsurerForm } from "@/features/insurers/components/insurer-form";
import { useInsurers } from "@/features/insurers/use-insurers";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { ConfirmDialog } from "@/shared/ui/confirm-dialog";
import { Notice } from "@/shared/ui/notice";

/**
 * The practice's mutual catalog (T51). Owner and admin write it; the page is behind a
 * RoleGate, and the API refuses the writes of any other role again.
 *
 * A mutual is deactivated, never deleted: patients and, later, settlements refer to it.
 * A deactivated mutual stays in the list, marked, and keeps covering the patients it
 * already covers.
 */
export function InsurersScreen() {
  const { insurers, failedStatus, reload } = useInsurers(true);
  const writesLocked = useWritesLocked();

  const [mode, setMode] = useState<"idle" | "create" | "edit">("idle");
  const [editing, setEditing] = useState<Insurer | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busyID, setBusyID] = useState<string | null>(null);
  const [confirmingDowngrade, setConfirmingDowngrade] = useState<Insurer | null>(null);

  function startEdit(insurer: Insurer) {
    setEditing(insurer);
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

  async function toggleActive(insurer: Insurer) {
    setActionError(null);
    setBusyID(insurer.id);
    try {
      const result = insurer.isActive
        ? await browserApi.POST("/api/v1/insurers/{id}/deactivate", {
            params: { path: { id: insurer.id } },
          })
        : await browserApi.POST("/api/v1/insurers/{id}/activate", {
            params: { path: { id: insurer.id } },
          });

      if (result.error || !result.data) {
        setActionError(apiErrorMessage(result.error));
        return;
      }
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
          <h1 className="text-2xl font-semibold tracking-tight">Mutuales</h1>
          <p className="mt-1 text-sm text-ink-muted">
            Las mutuales que atiende el consultorio. Cada paciente con cobertura de mutual
            se asocia a una de ellas.
          </p>
        </div>
        <button
          type="button"
          onClick={toggleCreate}
          disabled={writesLocked}
          className="rounded-md bg-brand px-4 py-2 text-sm font-medium text-brand-ink disabled:opacity-60"
        >
          {mode === "create" ? "Cerrar formulario" : "Nueva mutual"}
        </button>
      </header>

      {mode !== "idle" && !writesLocked ? (
        <InsurerForm
          key={editing?.id ?? "new"}
          insurer={mode === "edit" ? editing : null}
          onSaved={() => {
            closeForm();
            void reload();
          }}
          onCancel={closeForm}
        />
      ) : null}

      {failedStatus !== null ? (
        <Notice tone="danger" title="No pudimos leer las mutuales">
          {failedStatus === 0
            ? "La API no responde. Verificá que el backend esté corriendo en 127.0.0.1:8080."
            : `La API respondió ${failedStatus}.`}
        </Notice>
      ) : insurers === null ? (
        <p role="status" className="text-sm text-ink-muted">
          Cargando mutuales…
        </p>
      ) : insurers.length === 0 ? (
        <Notice tone="info" title="Todavía no hay mutuales">
          Cargá la primera con el botón de arriba. Los pacientes con cobertura de mutual la
          eligen de esta lista.
        </Notice>
      ) : (
        <div className="overflow-hidden rounded-[var(--radius-panel)] border border-border bg-surface">
          {actionError ? (
            <p role="alert" className="border-b border-border px-4 py-2 text-sm text-danger">
              {actionError}
            </p>
          ) : null}
          <table className="w-full text-left text-sm">
            <thead className="bg-surface-muted text-xs uppercase tracking-wide text-ink-muted">
              <tr>
                <th className="px-4 py-3 font-medium">Mutual</th>
                <th className="px-4 py-3 font-medium">CUIT</th>
                <th className="px-4 py-3 font-medium">Estado</th>
                <th className="px-4 py-3 font-medium">
                  <span className="sr-only">Acciones</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {insurers.map((insurer) => (
                <tr key={insurer.id} className="border-t border-border">
                  <td className="px-4 py-3 font-medium">{insurer.name}</td>
                  <td className="px-4 py-3 font-mono text-xs text-ink-muted">{insurer.taxId || "—"}</td>
                  <td className="px-4 py-3">
                    <span
                      className={`rounded-full border px-2 py-0.5 text-xs ${
                        insurer.isActive
                          ? "border-success/30 bg-success/5 text-success"
                          : "border-border bg-surface-muted text-ink-muted"
                      }`}
                    >
                      {insurer.isActive ? "Activa" : "Dada de baja"}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex justify-end gap-2">
                      <button
                        type="button"
                        onClick={() => startEdit(insurer)}
                        disabled={writesLocked}
                        className="rounded-md border border-border px-3 py-1.5 text-xs font-medium disabled:opacity-60"
                      >
                        Editar
                      </button>
                      <button
                        type="button"
                        onClick={() => {
                          if (insurer.isActive) setConfirmingDowngrade(insurer);
                          else void toggleActive(insurer);
                        }}
                        disabled={busyID === insurer.id || writesLocked}
                        className="rounded-control border border-border px-3 py-1.5 text-xs font-medium text-ink-muted disabled:opacity-60"
                      >
                        {busyID === insurer.id
                          ? "Guardando…"
                          : insurer.isActive
                            ? "Dar de baja"
                            : "Reactivar"}
                      </button>
                      <ConfirmDialog
                        open={confirmingDowngrade?.id === insurer.id}
                        title={`¿Dar de baja ${insurer.name}?`}
                        description="Deja de ofrecerse en las altas y en el tarifario. Los pacientes que ya la tienen siguen cubiertos, y se puede reactivar cuando haga falta."
                        confirmLabel="Dar de baja"
                        tone="danger"
                        busy={busyID === insurer.id}
                        onCancel={() => setConfirmingDowngrade(null)}
                        onConfirm={() => {
                          const target = confirmingDowngrade;
                          setConfirmingDowngrade(null);
                          if (target !== null) void toggleActive(target);
                        }}
                      />
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
