"use client";

import { canManageInsurers } from "@/entities/insurer/model";
import { InsurersScreen } from "@/features/insurers/components/insurers-screen";
import { RoleGate } from "@/shared/auth/role-gate";
import { SessionGate } from "@/shared/auth/session-gate";

/**
 * The mutual catalog (T51). Owner and admin only: the RoleGate keeps other roles off a
 * screen that can only fail, and the API enforces it again on every write.
 */
export default function InsurersPage() {
  return (
    <main className="mx-auto flex min-h-full w-full max-w-6xl flex-col gap-6 px-6 py-12">
      <SessionGate>
        <RoleGate allowed={canManageInsurers}>
          <InsurersScreen />
        </RoleGate>
      </SessionGate>
    </main>
  );
}
