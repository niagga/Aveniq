"use client";

import { useEffect, useState } from "react";

import { LogoutButton } from "@/features/auth/components/logout-button";
import { TenantTable } from "@/features/tenants/components/tenant-table";
import { canReadPractice } from "@/entities/iam/model";
import { browserApi } from "@/shared/api/client";
import type { components } from "@/shared/api/schema";
import { getSessionUser } from "@/shared/auth/session";
import { Notice } from "@/shared/ui/notice";
import { ScreenFrame } from "@/shared/ui/shell/app-shell";

type TenantBody = components["schemas"]["TenantBody"];

/**
 * The dashboard is a Client Component, and that is a consequence of the design
 * rather than a preference: the access token lives in browser memory, so the
 * server has no credential to fetch with on the user's behalf. A Server Component
 * here would receive a 401 from the API, which is exactly what T19 made it do.
 *
 * What keeps the token out of the server bundle is unchanged: the API origin is
 * still server-only, and every call goes through the same-origin proxy.
 *
 * The row of section links that used to live in the header is gone: the rail in the
 * private layout owns navigation now, and the links were only a way back to the menu.
 */
export default function DashboardPage() {
  return (
    <ScreenFrame>
      <Dashboard />
    </ScreenFrame>
  );
}

function Dashboard() {
  const [tenants, setTenants] = useState<TenantBody[] | null>(null);
  const [failedStatus, setFailedStatus] = useState<number | null>(null);

  const role = getSessionUser()?.role;
  // The accountant holds no tenant:read, so the practice card would only show a refusal.
  const showPractice = canReadPractice(role);

  useEffect(() => {
    if (!showPractice) return;
    let active = true;

    void (async () => {
      const { data, response } = await browserApi.GET("/api/v1/tenants");
      if (!active) return;

      if (!response.ok) {
        setFailedStatus(response.status);
        return;
      }
      setTenants(data?.items ?? []);
    })();

    return () => {
      active = false;
    };
  }, [showPractice]);

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Aveniq</h1>
          <p className="mt-1 text-sm text-ink-muted">
            Gestión de consultorios: pacientes, agenda y facturación.
          </p>
        </div>
        <LogoutButton />
      </header>

      {!showPractice ? null : failedStatus !== null ? (
        <Notice tone="danger" title="No pudimos leer el consultorio">
          {failedStatus === 0
            ? "La API no responde. Verificá que el backend esté corriendo en 127.0.0.1:8080."
            : `La API respondió ${failedStatus}.`}
        </Notice>
      ) : tenants === null ? (
        <p role="status" className="text-sm text-ink-muted">
          Cargando consultorio…
        </p>
      ) : (
        <TenantTable tenants={tenants} />
      )}
    </>
  );
}
