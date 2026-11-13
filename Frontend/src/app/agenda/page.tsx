"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

import { WeekCalendar } from "@/features/agenda/components/week-calendar";
import { canUseAgenda } from "@/entities/iam/model";
import { browserApi } from "@/shared/api/client";
import { RoleGate } from "@/shared/auth/role-gate";
import { SessionGate } from "@/shared/auth/session-gate";

export default function AgendaPage() {
  return (
    <main className="mx-auto flex min-h-full w-full max-w-7xl flex-col gap-6 px-6 py-12">
      <SessionGate>
        <RoleGate allowed={canUseAgenda}>
          <AgendaScreen />
        </RoleGate>
      </SessionGate>
    </main>
  );
}

function AgendaScreen() {
  const [timeZone, setTimeZone] = useState("America/Argentina/Buenos_Aires");

  useEffect(() => {
    void (async () => {
      const { data } = await browserApi.GET("/api/v1/tenants");
      const zone = data?.items?.[0]?.timezone;
      if (zone) setTimeZone(zone);
    })();
  }, []);

  return (
    <>
      <header>
        <Link href="/" className="text-xs text-ink-muted underline">
          ← Volver al consultorio
        </Link>
        <h1 className="mt-1 text-2xl font-semibold tracking-tight">Agenda</h1>
        <p className="mt-1 text-sm text-ink-muted">
          Semana del profesional, en la zona horaria del consultorio ({timeZone}).
        </p>
      </header>
      <WeekCalendar timeZone={timeZone} />
    </>
  );
}
