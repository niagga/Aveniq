"use client";

import Link from "next/link";
import { useCallback, useEffect, useId, useState } from "react";

import {
  canIssueVouchers,
  canVoidVouchers,
  startOfDay,
  startOfNextDay,
  VOUCHER_PAGE_SIZE,
  type Voucher,
  type VoucherStatus,
} from "@/entities/billing/model";
import { IssueForm } from "@/features/billing/components/issue-form";
import { VoucherDetail } from "@/features/billing/components/voucher-detail";
import { VoucherTable } from "@/features/billing/components/voucher-table";
import { browserApi } from "@/shared/api/client";
import { getSessionUser } from "@/shared/auth/session";
import { Field, inputClass } from "@/shared/ui/field";
import { Notice } from "@/shared/ui/notice";

type StatusFilter = "" | VoucherStatus;

/**
 * Internal vouchers of the practice (T42): the history, the issue form, the detail and the
 * void.
 *
 * The screen shows what the role may do. The owner and the accountant issue and void; the
 * admin reads the history and sees neither the form nor the void button. The API refuses
 * the rest again if asked, so hiding a control is a courtesy and never the protection.
 */
export function BillingScreen() {
  const role = getSessionUser()?.role;
  const mayIssue = canIssueVouchers(role);
  const mayVoid = canVoidVouchers(role);
  const prefix = useId();

  const [status, setStatus] = useState<StatusFilter>("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [vouchers, setVouchers] = useState<Voucher[] | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [failedStatus, setFailedStatus] = useState<number | null>(null);
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);

  const rangeInvalid = from !== "" && to !== "" && from > to;
  const filtered = status !== "" || from !== "" || to !== "";

  const fetchPage = useCallback(
    (offset: number) =>
      browserApi.GET("/api/v1/billing/vouchers", {
        params: {
          query: {
            limit: VOUCHER_PAGE_SIZE,
            offset,
            ...(status !== "" ? { status } : {}),
            ...(startOfDay(from) ? { from: startOfDay(from) } : {}),
            ...(startOfNextDay(to) ? { to: startOfNextDay(to) } : {}),
          },
        },
      }),
    [status, from, to],
  );

  // Reloads the first page whenever a filter changes or a voucher is issued or voided.
  useEffect(() => {
    if (rangeInvalid) return;
    let active = true;

    void (async () => {
      try {
        const { data, response } = await fetchPage(0);
        if (!active) return;
        if (!response.ok) {
          setFailedStatus(response.status);
          return;
        }
        const items = data?.items ?? [];
        setFailedStatus(null);
        setVouchers(items);
        setHasMore(items.length === VOUCHER_PAGE_SIZE);
      } catch {
        if (active) setFailedStatus(0);
      }
    })();

    return () => {
      active = false;
    };
  }, [fetchPage, rangeInvalid, refreshKey]);

  async function loadMore() {
    if (vouchers === null) return;
    setLoadingMore(true);
    try {
      const { data, response } = await fetchPage(vouchers.length);
      if (!response.ok) {
        setFailedStatus(response.status);
        return;
      }
      const items = data?.items ?? [];
      setVouchers((current) => [...(current ?? []), ...items]);
      setHasMore(items.length === VOUCHER_PAGE_SIZE);
    } catch {
      setFailedStatus(0);
    } finally {
      setLoadingMore(false);
    }
  }

  function handleIssued(voucher: Voucher) {
    setSelectedID(voucher.id);
    setRefreshKey((key) => key + 1);
  }

  function handleVoided(voucher: Voucher) {
    setVouchers((current) =>
      current === null ? current : current.map((item) => (item.id === voucher.id ? voucher : item)),
    );
    setRefreshKey((key) => key + 1);
  }

  const selected = vouchers?.find((voucher) => voucher.id === selectedID) ?? null;

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <Link href="/" className="text-xs text-ink-muted underline">
            ← Volver al consultorio
          </Link>
          <h1 className="mt-1 text-2xl font-semibold tracking-tight">Facturación</h1>
          <p className="mt-1 text-sm text-ink-muted">
            Comprobantes internos del consultorio: emitidos, anulados y su historial.
          </p>
        </div>
      </header>

      {mayIssue ? (
        <IssueForm onIssued={handleIssued} />
      ) : (
        <Notice tone="info" title="Solo lectura">
          Tu rol consulta el historial. Emitir y anular comprobantes es del titular y del contador.
        </Notice>
      )}

      <section className="grid gap-3">
        <h2 className="text-sm font-semibold">Historial</h2>

        <div className="grid gap-4 rounded-[var(--radius-panel)] border border-border bg-surface p-4 sm:grid-cols-3">
          <Field label="Estado" htmlFor={`${prefix}-status`}>
            <select
              id={`${prefix}-status`}
              value={status}
              onChange={(event) => setStatus(event.target.value as StatusFilter)}
              className={inputClass}
            >
              <option value="">Todos</option>
              <option value="issued">Emitidos</option>
              <option value="voided">Anulados</option>
            </select>
          </Field>
          <Field label="Desde" htmlFor={`${prefix}-from`}>
            <input
              id={`${prefix}-from`}
              type="date"
              value={from}
              onChange={(event) => setFrom(event.target.value)}
              className={inputClass}
            />
          </Field>
          <Field label="Hasta (inclusive)" htmlFor={`${prefix}-to`}>
            <input
              id={`${prefix}-to`}
              type="date"
              value={to}
              onChange={(event) => setTo(event.target.value)}
              className={inputClass}
            />
          </Field>
        </div>

        {rangeInvalid ? (
          <Notice tone="warning" title="El rango de fechas está al revés">
            La fecha «Desde» tiene que ser anterior o igual a «Hasta».
          </Notice>
        ) : failedStatus !== null ? (
          <Notice tone="danger" title="No pudimos leer los comprobantes">
            {failedStatus === 0
              ? "La API no responde. Verificá que el backend esté corriendo en 127.0.0.1:8080."
              : `La API respondió ${failedStatus}.`}
          </Notice>
        ) : vouchers === null ? (
          <p role="status" className="text-sm text-ink-muted">
            Cargando comprobantes…
          </p>
        ) : (
          <>
            <VoucherTable
              vouchers={vouchers}
              selectedID={selectedID}
              onSelect={setSelectedID}
              filtered={filtered}
            />
            {hasMore ? (
              <div>
                <button
                  type="button"
                  onClick={() => void loadMore()}
                  disabled={loadingMore}
                  className="rounded-md border border-border px-3 py-2 text-sm font-medium disabled:opacity-60"
                >
                  {loadingMore ? "Cargando…" : "Cargar más"}
                </button>
              </div>
            ) : null}
          </>
        )}
      </section>

      {selected !== null ? (
        <VoucherDetail
          key={`${selected.id}:${selected.status}`}
          voucher={selected}
          canVoid={mayVoid}
          onVoided={handleVoided}
          onClose={() => setSelectedID(null)}
        />
      ) : null}
    </>
  );
}
