"use client";

import { Suspense } from "react";

import { canReadBilling } from "@/entities/billing/model";
import { SettlementScreen } from "@/features/settlements/components/settlement-screen";
import { RoleGate } from "@/shared/auth/role-gate";
import { ScreenFrame } from "@/shared/ui/shell/app-shell";

/**
 * Monthly settlements to the mutuales (T55).
 *
 * A Client Component, like the other screens: the access token lives in browser memory. The
 * auth gate and the shell come from the private layout, so this file only says who may see it.
 */
export default function SettlementsPage() {
  return (
    <ScreenFrame>
      <RoleGate allowed={canReadBilling}>
        <Suspense fallback={<p className="text-sm text-ink-muted">Cargando liquidaciones…</p>}>
          <SettlementScreen />
        </Suspense>
      </RoleGate>
    </ScreenFrame>
  );
}
