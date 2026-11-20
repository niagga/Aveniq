"use client";

import { useEffect, useId, useState } from "react";

import {
  periodName,
  SETTLEMENT_PAGE_SIZE,
  sessionCountText,
  settlementStatusLabel,
  totalsText,
  type Settlement,
  type SettlementInsurer,
} from "@/entities/billing/model";
import { SettlementDetail } from "@/features/settlements/components/settlement-detail";
import { browserApi } from "@/shared/api/client";
import { Field, inputClass } from "@/shared/ui/field";
import { Notice } from "@/shared/ui/notice";

/**
 * The settlements of the practice, newest first, optionally of one mutual. Opening one shows its
 * sessions and, for a role that may void, the void. A voided settlement stays in the list.
 */
export function SettlementHistory({
  insurers,
  refreshKey,
  canVoid,
}: {
  insurers: SettlementInsurer[];
  refreshKey: number;
  canVoid: boolean;
}) {
  const prefix = useId();
  const [insurerFilter, setInsurerFilter] = useState("");
  const [settlements, setSettlements] = useState<Settlement[] | null>(null);
  const [failedStatus, setFailedStatus] = useState<number | null>(null);
  const [openedID, setOpenedID] = useState<string | null>(null);

  useEffect(() => {
    let active = true;

    void (async () => {
      try {
        const { data, response } = await browserApi.GET("/api/v1/billing/settlements", {
          params: {
            query: {
              limit: SETTLEMENT_PAGE_SIZE,
              offset: 0,
              ...(insurerFilter !== "" ? { insurerId: insurerFilter } : {}),
            },
          },
        });
        if (!active) return;
        if (!response.ok) {
          setFailedStatus(response.status);
          return;
        }
        setFailedStatus(null);
        setSettlements(data?.items ?? []);
      } catch {
        if (active) setFailedStatus(0);
      }
    })();

    return () => {
      active = false;
    };
  }, [insurerFilter, refreshKey]);

  const opened = settlements?.find((settlement) => settlement.id === openedID) ?? null;

  function handleVoided(updated: Settlement) {
    setSettlements((current) =>
      current === null
        ? current
        : current.map((settlement) => (settlement.id === updated.id ? updated : settlement)),
    );
  }

  return (
    <section aria-label="Historial de liquidaciones" className="grid gap-4">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <h2 className="text-sm font-semibold">Historial</h2>
        <div className="w-full sm:w-72">
          <Field label="Mutual" htmlFor={`${prefix}-insurer`}>
            <select
              id={`${prefix}-insurer`}
              value={insurerFilter}
              onChange={(event) => {
                setInsurerFilter(event.target.value);
                setOpenedID(null);
              }}
              className={inputClass}
            >
              <option value="">Todas las mutuales</option>
              {insurers.map((insurer) => (
                <option key={insurer.id} value={insurer.id}>
                  {insurer.name}
                </option>
              ))}
            </select>
          </Field>
        </div>
      </div>

      {failedStatus !== null ? (
        <Notice tone="danger" title="No pudimos leer las liquidaciones">
          {failedStatus === 0
            ? "La API no responde. Verificá que el backend esté corriendo en 127.0.0.1:8080."
            : `La API respondió ${failedStatus}.`}
        </Notice>
      ) : settlements === null ? (
        <p role="status" className="text-sm text-ink-muted">
          Cargando liquidaciones…
        </p>
      ) : settlements.length === 0 ? (
        <Notice
          tone="info"
          title={insurerFilter ? "Esta mutual no tiene liquidaciones" : "Todavía no hay liquidaciones"}
        >
          Las liquidaciones emitidas aparecen acá.
        </Notice>
      ) : (
        <>
          <div className="overflow-x-auto rounded-[var(--radius-panel)] border border-border bg-surface">
            <table className="w-full text-left text-sm">
              <thead className="bg-surface-muted text-xs uppercase tracking-wide text-ink-muted">
                <tr>
                  <th className="px-4 py-3 font-medium">Mes</th>
                  <th className="px-4 py-3 font-medium">Mutual</th>
                  <th className="px-4 py-3 font-medium">Sesiones</th>
                  <th className="px-4 py-3 text-right font-medium">Total</th>
                  <th className="px-4 py-3 font-medium">Estado</th>
                  <th className="px-4 py-3 font-medium">
                    <span className="sr-only">Acciones</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {settlements.map((settlement) => {
                  const voided = settlement.status === "voided";
                  const selected = settlement.id === openedID;
                  return (
                    <tr
                      key={settlement.id}
                      aria-selected={selected}
                      className={`border-t border-border align-top ${selected ? "bg-brand/5" : ""}`}
                    >
                      <td className={`px-4 py-3 capitalize ${voided ? "text-ink-muted line-through" : ""}`}>
                        {periodName(settlement.period)}
                      </td>
                      <td className="px-4 py-3 text-ink-muted">{settlement.insurerName}</td>
                      <td className="px-4 py-3 text-ink-muted">
                        {sessionCountText((settlement.items ?? []).length)}
                      </td>
                      <td
                        className={`whitespace-nowrap px-4 py-3 text-right tabular-nums ${
                          voided ? "text-ink-muted line-through" : ""
                        }`}
                      >
                        {totalsText(settlement.totals ?? [])}
                      </td>
                      <td className="px-4 py-3">
                        <span
                          className={`rounded-full border px-2 py-0.5 text-xs ${
                            voided
                              ? "border-border bg-surface-muted text-ink-muted"
                              : "border-success/30 bg-success/5 text-success"
                          }`}
                        >
                          {settlementStatusLabel(settlement.status)}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <button
                          type="button"
                          onClick={() => setOpenedID(settlement.id)}
                          aria-label={`Ver la liquidación de ${settlement.insurerName} de ${periodName(settlement.period)}`}
                          className="rounded-md border border-border px-2 py-1 text-xs font-medium"
                        >
                          Ver
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          {settlements.length === SETTLEMENT_PAGE_SIZE ? (
            <p className="text-xs text-ink-muted">
              Mostrando las {SETTLEMENT_PAGE_SIZE} más recientes. Filtrá por mutual para ver más.
            </p>
          ) : null}
        </>
      )}

      {opened ? (
        <SettlementDetail
          key={`${opened.id}:${opened.status}`}
          settlement={opened}
          canVoid={canVoid}
          onVoided={handleVoided}
          onClose={() => setOpenedID(null)}
        />
      ) : null}
    </section>
  );
}
