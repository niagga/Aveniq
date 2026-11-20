"use client";

import { Suspense } from "react";

import { canReadBilling } from "@/entities/billing/model";
import { BillingScreen } from "@/features/billing/components/billing-screen";
import { RoleGate } from "@/shared/auth/role-gate";

/**
 * Internal vouchers (T42). A Client Component, like the other screens: the access token
 * lives in browser memory. RoleGate keeps the clinical roles off a screen that can only
 * answer 403. The auth gate and the shell come from the private layout.
 */
export default function BillingPage() {
  return (
    <RoleGate allowed={canReadBilling}>
      <Suspense fallback={<p className="text-sm text-ink-muted">Cargando facturación…</p>}>
        <BillingScreen />
      </Suspense>
    </RoleGate>
  );
}
