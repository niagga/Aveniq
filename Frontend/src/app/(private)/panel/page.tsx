"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

import { LogoutButton } from "@/features/auth/components/logout-button";
import { FiguresPanel } from "@/features/reports/components/figures-panel";
import { canUseAgenda } from "@/entities/iam/model";
import { canReadReports } from "@/entities/report/model";
import { formatSlotTime, type Appointment } from "@/entities/schedule/model";
import { TENANT_STATUS_LABEL, formatCreatedAt } from "@/entities/tenant/model";
import { browserApi } from "@/shared/api/client";
import { usePractice } from "@/shared/auth/practice";
import { getSessionUser } from "@/shared/auth/session";
import { Notice } from "@/shared/ui/notice";
import { ScreenFrame } from "@/shared/ui/shell/app-shell";
import { LoadingState } from "@/shared/ui/states";

const DEFAULT_ZONE = "America/Argentina/Buenos_Aires";

/**
 * The landing of the private screens.
 *
 * It answers the question a professional opens the app with - what is happening today -
 * instead of listing the practices of the tenant. The list stays reachable from the API for
 * whoever needs it; a screen per practice is not what the person in front of it needs.
 *
 * A Client Component, and that is a consequence of the design rather than a preference: the
 * access token lives in browser memory, so the server has no credential to fetch with on the
 * user's behalf (T19).
 */
export default function DashboardPage() {
  return (
    <ScreenFrame>
      <Dashboard />
    </ScreenFrame>
  );
}

function Dashboard() {
  const role = getSessionUser()?.role;
  const seesAgenda = canUseAgenda(role);
  const { practice, status } = usePractice();
  const zone = status?.timezone || DEFAULT_ZONE;

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

      {practice === null ? null : (
        <section className="rounded-panel border border-border bg-surface p-5">
          <h2 className="text-sm font-semibold">{practice.name}</h2>
          <p className="mt-1 text-xs text-ink-muted">
            Consultorio <span className="font-mono">{practice.slug}</span> ·{" "}
            {TENANT_STATUS_LABEL[practice.status] ?? practice.status} · desde el{" "}
            {formatCreatedAt(practice.createdAt)}
          </p>
        </section>
      )}

      {seesAgenda ? <NextSessions zone={zone} /> : <BillingShortcut />}

      {canReadReports(role) ? <FiguresPanel zone={zone} role={role} /> : null}
    </>
  );
}

/**
 * The next 24 hours of the practice.
 *
 * "Next 24 hours" and not "today" on purpose: the day boundaries belong to the practice's
 * timezone, and a summary that quietly used the browser's would be wrong for anyone travelling.
 * The agenda screen owns the calendar; this only answers what comes next.
 */
function NextSessions({ zone }: { zone: string }) {
  const [appointments, setAppointments] = useState<Appointment[] | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let active = true;

    void (async () => {
      const from = new Date();
      const to = new Date(from.getTime() + 24 * 60 * 60 * 1000);
      try {
        const { data, response } = await browserApi.GET("/api/v1/appointments", {
          params: { query: { from: from.toISOString(), to: to.toISOString() } },
        });
        if (!active) return;
        if (!response.ok) {
          setFailed(true);
          return;
        }
        setAppointments(data?.items ?? []);
      } catch {
        if (active) setFailed(true);
      }
    })();

    return () => {
      active = false;
    };
  }, []);

  const scheduled = (appointments ?? []).filter((slot) => slot.status !== "cancelled");
  const next = scheduled[0];
  const attended = scheduled.filter((slot) => slot.status === "attended").length;

  return (
    <section className="grid gap-3">
      <h2 className="text-sm font-semibold">Próximas 24 horas</h2>

      {failed ? (
        <Notice tone="danger" title="No pudimos leer la agenda">
          La API no responde. Verificá que el backend esté corriendo en 127.0.0.1:8080.
        </Notice>
      ) : appointments === null ? (
        <LoadingState label="Cargando los próximos turnos…" rows={2} />
      ) : scheduled.length === 0 ? (
        <Notice tone="info" title="Sin turnos en las próximas 24 horas">
          Cuando se agende uno, aparece acá con su horario.
        </Notice>
      ) : (
        <div className="rounded-panel border border-border bg-surface p-5">
          <p className="text-sm">
            <strong className="text-lg font-semibold">{scheduled.length}</strong>{" "}
            {scheduled.length === 1 ? "turno" : "turnos"} · {attended} ya{" "}
            {attended === 1 ? "atendido" : "atendidos"}
          </p>
          {next ? (
            <p className="mt-1 text-sm text-ink-muted">
              El próximo es a las{" "}
              <span className="font-medium text-ink">{formatSlotTime(next.startsAt, zone)}</span>.
            </p>
          ) : null}
          <div className="mt-4">
            <Link
              href="/agenda"
              className="inline-flex items-center rounded-control border border-border-strong bg-surface px-2.5 py-1.5 text-xs font-medium text-ink hover:bg-surface-muted"
            >
              Ver el calendario
            </Link>
          </div>
        </div>
      )}
    </section>
  );
}

/** The accountant lands on billing and holds no schedule:read, so the dashboard points there. */
function BillingShortcut() {
  return (
    <section className="rounded-panel border border-border bg-surface p-5">
      <h2 className="text-sm font-semibold">Facturación</h2>
      <p className="mt-1 text-sm text-ink-muted">
        Tu rol consulta el historial de comprobantes. La agenda y las fichas clínicas no están a
        tu alcance.
      </p>
      <div className="mt-4">
        <Link
          href="/facturacion"
          className="inline-flex items-center rounded-control border border-border-strong bg-surface px-2.5 py-1.5 text-xs font-medium text-ink hover:bg-surface-muted"
        >
          Ver los comprobantes
        </Link>
      </div>
    </section>
  );
}
