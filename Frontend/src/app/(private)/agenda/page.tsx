"use client";

import { useEffect, useState } from "react";

import { WeekCalendar } from "@/features/agenda/components/week-calendar";
import { canUseAgenda } from "@/entities/iam/model";
import { browserApi } from "@/shared/api/client";
import { RoleGate } from "@/shared/auth/role-gate";
import { ScreenFrame } from "@/shared/ui/shell/app-shell";

export default function AgendaPage() {
  return (
    <ScreenFrame>
      <RoleGate allowed={canUseAgenda}>
        <AgendaScreen />
      </RoleGate>
    </ScreenFrame>
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
        <h1 className="text-2xl font-semibold tracking-tight">Agenda</h1>
        <p className="mt-1 text-sm text-ink-muted">
          Semana del profesional, en la zona horaria del consultorio ({timeZone}).
        </p>
      </header>
      <WeekCalendar timeZone={timeZone} />
    </>
  );
}
