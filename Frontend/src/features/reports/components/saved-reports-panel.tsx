"use client";

import { useState } from "react";

import { describeSaved, type SavedReport } from "@/entities/report/model";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import { Button } from "@/shared/ui/button";
import { ConfirmDialog } from "@/shared/ui/confirm-dialog";
import { Notice } from "@/shared/ui/notice";
import { LoadingState } from "@/shared/ui/states";

/**
 * The reports the practice shares: opening one puts its period, cut and filters on the page,
 * where the figures are computed again with the viewer's own role. Only the author, the owner and
 * the admin may remove one (`canManage`, decided by the API); everyone else just opens it.
 */
export function SavedReportsPanel({
  reports,
  failed,
  activeId,
  writesLocked,
  onOpen,
  onRemoved,
}: {
  reports: readonly SavedReport[] | null;
  failed: boolean;
  activeId: string | null;
  writesLocked: boolean;
  onOpen: (saved: SavedReport) => void;
  onRemoved: (saved: SavedReport) => void;
}) {
  const [confirming, setConfirming] = useState<SavedReport | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function remove(saved: SavedReport) {
    setError(null);
    setBusyId(saved.id);
    try {
      const { error: failure, response } = await browserApi.DELETE("/api/v1/reports/saved/{id}", {
        params: { path: { id: saved.id } },
      });
      // A report someone else already removed is gone either way: the list refreshes.
      if (!response.ok && response.status !== 404) {
        setError(apiErrorMessage(failure));
        return;
      }
      onRemoved(saved);
    } catch {
      setError("No pudimos contactar al servidor.");
    } finally {
      setBusyId(null);
      setConfirming(null);
    }
  }

  return (
    <section aria-labelledby="saved-title" className="rounded-panel border border-border bg-surface p-5">
      <h2 id="saved-title" className="text-sm font-semibold">
        Informes guardados
      </h2>
      <p className="mt-1 text-xs text-ink-muted">
        Los ve todo el consultorio. Guardan la pregunta, no las cifras: cada uno las calcula con su
        rol.
      </p>

      <div className="mt-4">
        {failed ? (
          <Notice tone="danger" title="No pudimos leer los informes guardados">
            La API no responde. Verificá que el backend esté corriendo.
          </Notice>
        ) : reports === null ? (
          <LoadingState label="Cargando informes guardados…" rows={2} />
        ) : reports.length === 0 ? (
          <p className="text-sm text-ink-muted">
            Todavía no hay informes guardados. Armá uno arriba y guardalo con un nombre.
          </p>
        ) : (
          <ul className="grid gap-2">
            {reports.map((saved) => (
              <li
                key={saved.id}
                className={`rounded-control border px-3 py-2 ${
                  saved.id === activeId ? "border-brand bg-brand/5" : "border-border"
                }`}
              >
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div className="min-w-0">
                    <button
                      type="button"
                      onClick={() => onOpen(saved)}
                      aria-current={saved.id === activeId ? "true" : undefined}
                      className="text-left text-sm font-medium text-brand hover:underline"
                    >
                      {saved.name}
                    </button>
                    <p className="mt-0.5 text-xs text-ink-muted">{describeSaved(saved)}</p>
                    {saved.createdByName ? (
                      <p className="text-xs text-ink-muted">Guardado por {saved.createdByName}</p>
                    ) : null}
                  </div>
                  {saved.canManage ? (
                    <Button
                      size="sm"
                      variant="danger"
                      disabled={writesLocked || busyId === saved.id}
                      onClick={() => setConfirming(saved)}
                      aria-label={`Quitar el informe ${saved.name}`}
                    >
                      Quitar
                    </Button>
                  ) : null}
                </div>
              </li>
            ))}
          </ul>
        )}
        {error ? (
          <p role="alert" className="mt-2 text-sm text-danger">
            {error}
          </p>
        ) : null}
      </div>

      <ConfirmDialog
        open={confirming !== null}
        title="¿Quitar este informe guardado?"
        description={
          confirming
            ? `«${confirming.name}» deja de aparecer para todo el consultorio. No se borra ninguna cifra: solo la pregunta guardada.`
            : undefined
        }
        confirmLabel="Quitar informe"
        tone="danger"
        busy={busyId !== null}
        onCancel={() => setConfirming(null)}
        onConfirm={() => {
          if (confirming !== null) void remove(confirming);
        }}
      />
    </section>
  );
}
