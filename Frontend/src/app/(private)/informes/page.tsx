"use client";

import { canReadReports } from "@/entities/report/model";
import { ReportsScreen } from "@/features/reports/components/reports-screen";
import { RoleGate } from "@/shared/auth/role-gate";
import { ScreenFrame } from "@/shared/ui/shell/app-shell";

/**
 * The reports (T60). Owner, admin and accountant read the whole practice; the professional
 * reads only their own figures. The API decides the scope; the gate only keeps the assistant
 * off a screen that can only answer 403.
 */
export default function ReportsPage() {
  return (
    <ScreenFrame>
      <RoleGate allowed={canReadReports}>
        <ReportsScreen />
      </RoleGate>
    </ScreenFrame>
  );
}