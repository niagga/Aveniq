"use client";

import { Suspense } from "react";

import { canReadBilling } from "@/entities/billing/model";
import { BillingScreen } from "@/features/billing/components/billing-screen";
import { RoleGate } from "@/shared/auth/role-gate";
import { ScreenFrame } from "@/shared/ui/shell/app-shell";

/**
 * Internal vouchers (T42; popups and sticky filters since T76). A Client Component, like the
 * other screens: the access token lives in browser memory. RoleGate keeps the clinical roles
 * off a screen that can only answer 403. The auth gate and the shell come from the private
 * layout; the frame lives here, like on the other screens, so the screen itself stays just
 * the content under S25.
 */
export default function BillingPage() {
  return (
    <ScreenFrame>
      <RoleGate allowed={canReadBilling}>
        <Suspense fallback={<p className="text-sm text-ink-muted">Cargando facturación…</p>}>
          <BillingScreen />
        </Suspense>
      </RoleGate>
    </ScreenFrame>
  );
}
