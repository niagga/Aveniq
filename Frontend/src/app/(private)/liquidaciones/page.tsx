"use client";

import { Suspense } from "react";

import { canReadBilling } from "@/entities/billing/model";
import { SettlementScreen } from "@/features/settlements/components/settlement-screen";
import { RoleGate } from "@/shared/auth/role-gate";
import { SessionGate } from "@/shared/auth/session-gate";

/**
 * Monthly settlements to the mutuales (T55). A Client Component inside SessionGate, like the other
 * screens: the access token lives in browser memory. RoleGate keeps the clinical roles off a screen
 * that can only answer 403.
 */
export default function SettlementsPage() {
  return (
    <main className="mx-auto flex min-h-full w-full max-w-6xl flex-col gap-6 px-6 py-12">
      <SessionGate>
        <RoleGate allowed={canReadBilling}>
          <Suspense fallback={<p className="text-sm text-ink-muted">Cargando liquidaciones…</p>}>
            <SettlementScreen />
          </Suspense>
        </RoleGate>
      </SessionGate>
    </main>
  );
}
