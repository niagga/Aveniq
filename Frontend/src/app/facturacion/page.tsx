"use client";

import { canReadBilling } from "@/entities/billing/model";
import { BillingScreen } from "@/features/billing/components/billing-screen";
import { RoleGate } from "@/shared/auth/role-gate";
import { SessionGate } from "@/shared/auth/session-gate";

/**
 * Internal vouchers (T42). A Client Component inside SessionGate, like the other screens:
 * the access token lives in browser memory. RoleGate keeps the clinical roles off a screen
 * that can only answer 403.
 */
export default function BillingPage() {
  return (
    <main className="mx-auto flex min-h-full w-full max-w-6xl flex-col gap-6 px-6 py-12">
      <SessionGate>
        <RoleGate allowed={canReadBilling}>
          <BillingScreen />
        </RoleGate>
      </SessionGate>
    </main>
  );
}
