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
  type BillingFilters,
  type Voucher,
  type VoucherStatus,
} from "@/entities/billing/model";
import { IssueForm } from "@/features/billing/components/issue-form";
import { VoucherDetail } from "@/features/billing/components/voucher-detail";
import { VoucherTable } from "@/features/billing/components/voucher-table";
import { LogoutButton } from "@/features/auth/components/logout-button";
import { browserApi } from "@/shared/api/client";
import { getSessionUser } from "@/shared/auth/session";
import { Button } from "@/shared/ui/button";
import { Field, inputClass } from "@/shared/ui/field";
import { Notice } from "@/shared/ui/notice";
import { ScreenFrame } from "@/shared/ui/shell/app-shell";
import { LoadingState } from "@/shared/ui/states";

type StatusFilter = "" | VoucherStatus;

/**
 * The saved views of the contextual panel. They carry no counts on purpose: the API exposes
 * sessions per patient but no unbilled aggregate, and pinning a number in the UI would be
 * inventing data. A real counter needs the endpoint tracked as T10.
 */
const VIEWS: readonly { status: StatusFilter; label: string }[] = [
  { status: "", label: "Todos" },
  { status: "issued", label: "Emitidos" },
  { status: "voided", label: "Anulados" },
];

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

  // The filters live in the URL, not in component state: a filtered view has to survive a
  // reload and be shareable, and the open voucher rides along with them.
  const statusParam = searchParams.get("estado");
  const status: StatusFilter =
    statusParam === "issued" || statusParam === "voided" ? statusParam : "";
  const from = searchParams.get("desde") ?? "";
  const to = searchParams.get("hasta") ?? "";
  const filters: BillingFilters = { status, from, to };
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

  function applyFilters(next: BillingFilters) {
    router.replace(billingUrl(selectedID, next));
  }

  function openVoucher(id: string) {
    router.replace(billingUrl(id, filters));
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

  // The screen owns the frame because the panel's content is the screen's own state.
  const panel = (
    <aside
      aria-label="Vistas de facturación"
      className="hidden w-64 shrink-0 flex-col border-r border-border bg-surface md:flex"
    >
      <div className="border-b border-border px-4 py-3">
        <h2 className="text-sm font-semibold">Vistas</h2>
        <p className="mt-0.5 text-xs text-ink-muted">Los filtros viven en la URL</p>
      </div>

      <ul className="grid gap-0.5 p-2">
        {VIEWS.map((view) => {
          const active = status === view.status;
          return (
            <li key={view.status || "todos"}>
              <button
                type="button"
                aria-current={active ? "true" : undefined}
                onClick={() => applyFilters({ ...filters, status: view.status })}
                className={`w-full rounded-control border-l-2 px-3 py-1.5 text-left text-sm ${
                  active
                    ? "border-brand bg-brand/10 font-semibold text-brand"
                    : "border-transparent text-ink hover:bg-surface-muted"
                }`}
              >
                {view.label}
              </button>
            </li>
          );
        })}
      </ul>

      <div className="mt-auto grid gap-3 border-t border-border p-3">
        <Field label="Desde" htmlFor={`${prefix}-panel-from`}>
          <input
            id={`${prefix}-panel-from`}
            type="date"
            value={from}
            onChange={(event) => applyFilters({ ...filters, from: event.target.value })}
            className={inputClass}
          />
        </Field>
        <Field label="Hasta (inclusive)" htmlFor={`${prefix}-panel-to`}>
          <input
            id={`${prefix}-panel-to`}
            type="date"
            value={to}
            onChange={(event) => applyFilters({ ...filters, to: event.target.value })}
            className={inputClass}
          />
        </Field>
        <Button
          size="sm"
          onClick={() => applyFilters({ status: "", from: "", to: "" })}
          disabled={!filtered}
        >
          Limpiar filtros
        </Button>
      </div>
    </aside>
  );

  return (
    <ScreenFrame panel={panel}>
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

        {/* Under md the panel is hidden, so the same views stay reachable here. */}
        <div className="flex flex-wrap gap-2 md:hidden">
          {VIEWS.map((view) => (
            <button
              key={view.status || "todos"}
              type="button"
              aria-current={status === view.status ? "true" : undefined}
              onClick={() => applyFilters({ ...filters, status: view.status })}
              className={`rounded-full border px-3 py-1 text-xs ${
                status === view.status
                  ? "border-brand bg-brand/10 font-semibold text-brand"
                  : "border-border bg-surface text-ink-muted"
              }`}
            >
              {view.label}
            </button>
          ))}
        </div>

        <details className="rounded-control border border-border bg-surface p-3 md:hidden">
          <summary className="cursor-pointer text-sm font-medium">Rango de fechas</summary>
          <div className="mt-3 grid gap-3 sm:grid-cols-2">
            <Field label="Desde" htmlFor={`${prefix}-mobile-from`}>
              <input
                id={`${prefix}-mobile-from`}
                type="date"
                value={from}
                onChange={(event) => applyFilters({ ...filters, from: event.target.value })}
                className={inputClass}
              />
            </Field>
            <Field label="Hasta (inclusive)" htmlFor={`${prefix}-mobile-to`}>
              <input
                id={`${prefix}-mobile-to`}
                type="date"
                value={to}
                onChange={(event) => applyFilters({ ...filters, to: event.target.value })}
                className={inputClass}
              />
            </Field>
          </div>
        </details>

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
          <LoadingState label="Cargando comprobantes…" />
        ) : (
          <>
            <VoucherTable
              vouchers={vouchers}
              selectedID={selectedID}
              filtered={filtered}
              filters={filters}
            />
            {hasMore ? (
              <div>
                <Button size="sm" onClick={() => void loadMore()} busy={loadingMore}>
                  {loadingMore ? "Cargando…" : "Cargar más"}
                </Button>
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
          <LoadingState label="Cargando el comprobante…" rows={2} />
        ) : (
          <VoucherDetail
            key={`${opened.id}:${opened.status}`}
            voucher={opened}
            canVoid={mayVoid}
            onVoided={handleVoided}
            onClose={() => router.replace(billingUrl(null, filters))}
          />
        )
      ) : null}
    </ScreenFrame>
  );
}
