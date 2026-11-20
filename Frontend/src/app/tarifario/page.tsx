"use client";

import { canReadTariffs } from "@/entities/tariff/model";
import { TariffsScreen } from "@/features/tariffs/components/tariffs-screen";
import { RoleGate } from "@/shared/auth/role-gate";
import { SessionGate } from "@/shared/auth/session-gate";

/**
 * The fee schedule (T53). Owner, admin and accountant open it; only owner and admin write, and
 * the screen and the API both enforce that.
 */
export default function TariffsPage() {
  return (
    <main className="mx-auto flex min-h-full w-full max-w-6xl flex-col gap-6 px-6 py-12">
      <SessionGate>
        <RoleGate allowed={canReadTariffs}>
          <TariffsScreen />
        </RoleGate>
      </SessionGate>
    </main>
  );
}
