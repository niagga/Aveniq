"use client";

import { canManageInsurers } from "@/entities/insurer/model";
import { InsurersScreen } from "@/features/insurers/components/insurers-screen";
import { RoleGate } from "@/shared/auth/role-gate";
import { ScreenFrame } from "@/shared/ui/shell/app-shell";

/**
 * The mutual catalog (T51). Owner and admin only: the RoleGate keeps other roles off a
 * screen that can only fail, and the API enforces it again on every write.
 */
export default function InsurersPage() {
  return (
    <ScreenFrame>
      <RoleGate allowed={canManageInsurers}>
        <InsurersScreen />
      </RoleGate>
    </ScreenFrame>
  );
}
