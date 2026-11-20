"use client";

import { canReadTariffs } from "@/entities/tariff/model";
import { TariffsScreen } from "@/features/tariffs/components/tariffs-screen";
import { RoleGate } from "@/shared/auth/role-gate";
import { ScreenFrame } from "@/shared/ui/shell/app-shell";

/**
 * The fee schedule (T53). Owner, admin and accountant open it; only owner and admin write, and
 * the screen and the API both enforce that.
 */
export default function TariffsPage() {
  return (
    <ScreenFrame>
      <RoleGate allowed={canReadTariffs}>
        <TariffsScreen />
      </RoleGate>
    </ScreenFrame>
  );
}
