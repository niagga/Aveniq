"use client";

import { canManageLanding } from "@/entities/landing/model";
import { PracticeSettingsScreen } from "@/features/landing/components/practice-settings-screen";
import { RoleGate } from "@/shared/auth/role-gate";
import { ScreenFrame } from "@/shared/ui/shell/app-shell";

/** Landing settings: owner and admin only (tenant:settings). */
export default function PracticeSettingsPage() {
  return (
    <ScreenFrame>
      <RoleGate allowed={canManageLanding}>
        <PracticeSettingsScreen />
      </RoleGate>
    </ScreenFrame>
  );
}