"use client";

import { canReadClients } from "@/entities/client/model";
import { ClientsScreen } from "@/features/clients/components/clients-screen";
import { RoleGate } from "@/shared/auth/role-gate";
import { ScreenFrame } from "@/shared/ui/shell/app-shell";

/**
 * The billing clients of the practice (T74, S26). Every role reads them, because a role that
 * sees a patient also sees who pays for it; only the accountant cannot write.
 */
export default function ClientsPage() {
  return (
    <ScreenFrame>
      <RoleGate allowed={canReadClients}>
        <ClientsScreen />
      </RoleGate>
    </ScreenFrame>
  );
}
