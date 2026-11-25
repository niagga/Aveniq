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
import { IssueVoucherModal } from "@/features/billing/components/issue-form";
import { VoucherDetailModal } from "@/features/billing/components/voucher-detail";
import { VoucherTable } from "@/features/billing/components/voucher-table";
import { LogoutButton } from "@/features/auth/components/logout-button";
import { browserApi } from "@/shared/api/client";
import { getSessionUser } from "@/shared/auth/session";
import { useWritesLocked } from "@/shared/auth/practice";
import { Button } from "@/shared/ui/button";
import { useUnbilledCount, unbilledLabel } from "@/features/billing/use-unbilled";
import { Field, inputClass } from "@/shared/ui/field";
import { FilterBar } from "@/shared/ui/filter-bar";
import { Notice } from "@/shared/ui/notice";
import { PageHeader } from "@/shared/ui/page-header";
import { Segmented } from "@/shared/ui/segmented";
import { LoadingState } from "@/shared/ui/states";

type StatusFilter = "" | VoucherStatus;

/**
 * The views of the history, drawn as one segmented control in the filter bar. They carry no
 * counts on purpose: counting here would mean asking the API for a list the screen is not
 * showing, and pinning a number in the UI would be inventing data.
 */
const VIEWS: readonly { status: StatusFilter; label: string }[] = [
  { status: "", label: "Todos" },
  { status: "issued", label: "Emitidos" },
  { status: "voided", label: "Anulados" },
];

/**
 * Internal vouchers of the practice (T42, and since T76 under S25): the issue form and every
 * detail open in a popup with a blur, and the filters stay fixed at the top of the list.
 *
 * The screen shows what the role may do. The owner and the accountant issue and void; the
 * admin reads the history and sees neither the button nor the void. The API refuses the rest
 * again if asked, so hiding a control is a courtesy and never the protection.
 */
export function BillingScreen() {
  const role = getSessionUser()?.role;
  const mayIssue = canIssueVouchers(role);
  const mayVoid = canVoidVouchers(role);
  const writesLocked = useWritesLocked();
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
  const unbilled = unbilledLabel(useUnbilledCount(true));
  const [vouchers, setVouchers] = useState<Voucher[] | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [failedStatus, setFailedStatus] = useState<number | null>(null);
  const [opened, setOpened] = useState<Voucher | null>(null);
  const [openedStatus, setOpenedStatus] = useState<number | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const [issueOpen, setIssueOpen] = useState(false);

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
    // Closing the issue popup and opening this voucher's detail are one motion: the
    // confirmation is the detail itself, with the number, the amount and the snapshot.
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
      <PageHeader
        title="Facturación"
        description="Comprobantes internos del consultorio: emitidos, anulados y su historial."
        action={
          <div className="flex items-center gap-2">
            {mayIssue ? (
              <Button
                size="sm"
                variant="primary"
                onClick={() => setIssueOpen(true)}
                disabled={writesLocked}
              >
                Emitir comprobante
              </Button>
            ) : null}
            {/* The accountant lands here after sign-in and has no other screen to sign out from. */}
            <LogoutButton />
          </div>
        }
      />

      {mayIssue ? (
        <IssueVoucherModal
          open={issueOpen}
          onClose={() => setIssueOpen(false)}
          onIssued={handleIssued}
        />
      ) : null}

      {/* How many attended sessions of the last 30 days are still owed a receipt. The badge
          in the rail and this card read the same count, so they cannot tell two stories. */}
      <section
        aria-label="Sesiones sin facturar"
        className="rounded-[var(--radius-panel)] border border-border bg-surface p-4"
      >
        <h2 className="text-sm font-semibold">Sin facturar</h2>
        <p className="mt-1 text-sm text-ink-muted">
          {unbilled === ""
            ? "Ninguna sesión atendida quedó sin comprobante en los últimos 30 días."
            : `${unbilled} ${unbilled === "1" ? "sesión atendida" : "sesiones atendidas"} sin comprobante en los últimos 30 días.`}
        </p>
      </section>

      {!mayIssue ? (
        <Notice tone="info" title="Solo lectura">
          Tu rol consulta el historial. Emitir y anular comprobantes es del titular y del contador.
        </Notice>
      ) : null}

      {/* The filters stay in view while the history scrolls (S25): the bar is sticky, so a
          long list never carries the date range off screen. */}
      <FilterBar label="Filtros de facturación" sticky>
        <div>
          <span className="block text-xs font-medium text-ink-muted">Estado</span>
          <Segmented
            label="Vistas de facturación"
            value={status}
            onChange={(next) => applyFilters({ ...filters, status: next })}
            options={VIEWS.map((view) => ({ value: view.status, label: view.label }))}
          />
        </div>
        <Field label="Desde" htmlFor={`${prefix}-from`}>
          <input
            id={`${prefix}-from`}
            type="date"
            value={from}
            onChange={(event) => applyFilters({ ...filters, from: event.target.value })}
            className={inputClass}
          />
        </Field>
        <Field label="Hasta (inclusive)" htmlFor={`${prefix}-to`}>
          <input
            id={`${prefix}-to`}
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
      </FilterBar>

      <section className="grid gap-3">
        <h2 className="text-sm font-semibold">Historial</h2>

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
          <VoucherDetailModal
            key={`${opened.id}:${opened.status}`}
            open
            voucher={opened}
            canVoid={mayVoid}
            onVoided={handleVoided}
            // Closing keeps the filters: only the voucher leaves the URL.
            onClose={() => router.replace(billingUrl(null, filters))}
          />
        )
      ) : null}
    </>
  );
}
