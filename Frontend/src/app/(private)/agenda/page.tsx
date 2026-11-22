"use client";

import { useEffect, useState } from "react";

import { WeekCalendar } from "@/features/agenda/components/week-calendar";
import { canUseAgenda } from "@/entities/iam/model";
import { browserApi } from "@/shared/api/client";
import { RoleGate } from "@/shared/auth/role-gate";
import { PageHeader } from "@/shared/ui/page-header";
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
      <PageHeader
        title="Agenda"
        description={`Turnos de la semana en la zona horaria del consultorio (${timeZone}).`}
      />
      <WeekCalendar timeZone={timeZone} />
    </>
  );
}
