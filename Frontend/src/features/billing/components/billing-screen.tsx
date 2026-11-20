"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useId, useState } from "react";

import {
  billingUrl,
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
import { LogoutButton } from "@/features/auth/components/logout-button";
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
  const router = useRouter();
  const searchParams = useSearchParams();
  const selectedID = searchParams.get("voucher");

  const [status, setStatus] = useState<StatusFilter>("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [vouchers, setVouchers] = useState<Voucher[] | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [failedStatus, setFailedStatus] = useState<number | null>(null);
  const [opened, setOpened] = useState<Voucher | null>(null);
  const [openedStatus, setOpenedStatus] = useState<number | null>(null);
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

  // The detail is always GET /vouchers/{id}: a link must open a voucher that is not on
  // this page of the history, and a missing one must look the same as one of another
  // practice (404, never a leak).
  useEffect(() => {
    if (selectedID === null || selectedID === "") {
      setOpened(null);
      setOpenedStatus(null);
      return;
    }
    let active = true;
    setOpened((current) => (current?.id === selectedID ? current : null));
    setOpenedStatus(null);

    void (async () => {
      try {
        const { data, response } = await browserApi.GET("/api/v1/billing/vouchers/{id}", {
          params: { path: { id: selectedID } },
        });
        if (!active) return;
        if (!response.ok || !data) {
          setOpenedStatus(response.status);
          return;
        }
        setOpened(data);
        setOpenedStatus(null);
      } catch {
        if (active) setOpenedStatus(0);
      }
    })();

    return () => {
      active = false;
    };
  }, [selectedID, refreshKey]);

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

  function openVoucher(id: string) {
    router.replace(billingUrl(id));
  }

  function handleIssued(voucher: Voucher) {
    openVoucher(voucher.id);
    setRefreshKey((key) => key + 1);
  }

  function handleVoided(voucher: Voucher) {
    setOpened(voucher);
    setVouchers((current) =>
      current === null ? current : current.map((item) => (item.id === voucher.id ? voucher : item)),
    );
    setRefreshKey((key) => key + 1);
  }

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Facturación</h1>
          <p className="mt-1 text-sm text-ink-muted">
            Comprobantes internos del consultorio: emitidos, anulados y su historial.
          </p>
        </div>
        {/* The accountant lands here after sign-in and has no other screen to sign out from. */}
        <LogoutButton />
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

      {selectedID ? (
        openedStatus !== null ? (
          <Notice
            tone={openedStatus === 404 || openedStatus === 422 ? "warning" : "danger"}
            title={
              openedStatus === 404 || openedStatus === 422
                ? "Ese comprobante no está en este consultorio"
                : "No pudimos leer el comprobante"
            }
          >
            {openedStatus === 0
              ? "La API no responde. Verificá que el backend esté corriendo en 127.0.0.1:8080."
              : openedStatus === 404 || openedStatus === 422
                ? "El enlace no corresponde a un comprobante de esta práctica, o el comprobante no existe."
                : `La API respondió ${openedStatus}.`}
          </Notice>
        ) : opened === null ? (
          <p role="status" className="text-sm text-ink-muted">
            Cargando el comprobante…
          </p>
        ) : (
          <VoucherDetail
            key={`${opened.id}:${opened.status}`}
            voucher={opened}
            canVoid={mayVoid}
            onVoided={handleVoided}
            onClose={() => router.replace("/facturacion")}
          />
        )
      ) : null}
    </>
  );
}
