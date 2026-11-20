"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

import {
  canIssueVouchers,
  canVoidVouchers,
  type SettlementInsurer,
} from "@/entities/billing/model";
import { LogoutButton } from "@/features/auth/components/logout-button";
import { SettlementHistory } from "@/features/settlements/components/settlement-history";
import { SettlementIssue } from "@/features/settlements/components/settlement-issue";
import { browserApi } from "@/shared/api/client";
import { getSessionUser } from "@/shared/auth/session";
import { Notice } from "@/shared/ui/notice";

/**
 * The monthly settlements to the mutuales (T55). The owner and the accountant issue and void; the
 * admin reads the history and sees no form. The API refuses the rest again if asked, so hiding a
 * control is a courtesy and never the protection.
 */
export function SettlementScreen() {
  const role = getSessionUser()?.role;
  const mayIssue = canIssueVouchers(role);
  const mayVoid = canVoidVouchers(role);

  const [insurers, setInsurers] = useState<SettlementInsurer[] | null>(null);
  const [failedStatus, setFailedStatus] = useState<number | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);

  useEffect(() => {
    let active = true;

    void (async () => {
      try {
        const { data, response } = await browserApi.GET("/api/v1/billing/settlement-insurers");
        if (!active) return;
        if (!response.ok) {
          setFailedStatus(response.status);
          return;
        }
        setInsurers(data?.items ?? []);
      } catch {
        if (active) setFailedStatus(0);
      }
    })();

    return () => {
      active = false;
    };
  }, []);

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <Link href="/" className="text-xs text-ink-muted underline">
            ← Volver al consultorio
          </Link>
          <h1 className="mt-1 text-2xl font-semibold tracking-tight">Liquidaciones</h1>
          <p className="mt-1 text-sm text-ink-muted">
            Cobro mensual a las mutuales: las sesiones atendidas de sus pacientes, valuadas con el
            tarifario. Una sesión se liquida una sola vez.
          </p>
        </div>
        {/* The accountant lands on this screen by its own link and has no other place to sign out from. */}
        <LogoutButton />
      </header>

      {failedStatus !== null ? (
        <Notice tone="danger" title="No pudimos leer las mutuales">
          {failedStatus === 0
            ? "La API no responde. Verificá que el backend esté corriendo en 127.0.0.1:8080."
            : `La API respondió ${failedStatus}.`}
        </Notice>
      ) : null}

      {mayIssue ? (
        <SettlementIssue
          insurers={insurers ?? []}
          onIssued={() => setRefreshKey((key) => key + 1)}
        />
      ) : (
        <Notice tone="info" title="Solo lectura">
          Tu rol consulta las liquidaciones. Emitir y anular es del titular y del contador.
        </Notice>
      )}

      <SettlementHistory
        insurers={insurers ?? []}
        refreshKey={refreshKey}
        canVoid={mayVoid}
      />
    </>
  );
}
