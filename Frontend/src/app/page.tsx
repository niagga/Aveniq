"use client";

import { useEffect, useState } from "react";
import Link from "next/link";

import { LogoutButton } from "@/features/auth/components/logout-button";
import { TenantTable } from "@/features/tenants/components/tenant-table";
import { browserApi } from "@/shared/api/client";
import type { components } from "@/shared/api/schema";
import { SessionGate } from "@/shared/auth/session-gate";
import { Notice } from "@/shared/ui/notice";

type TenantBody = components["schemas"]["TenantBody"];

/**
 * The dashboard is a Client Component, and that is a consequence of the design
 * rather than a preference: the access token lives in browser memory, so the
 * server has no credential to fetch with on the user's behalf. A Server Component
 * here would receive a 401 from the API, which is exactly what T19 made it do.
 *
 * What keeps the token out of the server bundle is unchanged: the API origin is
 * still server-only, and every call goes through the same-origin proxy.
 */
export default function DashboardPage() {
  return (
    <main className="mx-auto flex min-h-full w-full max-w-5xl flex-col gap-6 px-6 py-12">
      <SessionGate>
        <Dashboard />
      </SessionGate>
    </main>
  );
}

function Dashboard() {
  const [tenants, setTenants] = useState<TenantBody[] | null>(null);
  const [failedStatus, setFailedStatus] = useState<number | null>(null);

  useEffect(() => {
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
  }, []);

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Aveniq</h1>
          <p className="mt-1 text-sm text-ink-muted">
            Gestión de consultorios: pacientes, agenda y facturación.
          </p>
        </div>
        <div className="flex items-center gap-3">
          <Link
            href="/patients"
            className="rounded-md border border-border px-3 py-1.5 text-xs font-medium"
          >
            Pacientes
          </Link>
          <Link
            href="/agenda"
            className="rounded-md border border-border px-3 py-1.5 text-xs font-medium"
          >
            Agenda
          </Link>
          <LogoutButton />
        </div>
      </header>

      {failedStatus !== null ? (
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
