import type { ReactNode } from "react";

import { SessionGate } from "@/shared/auth/session-gate";
import { AppShell } from "@/shared/ui/shell/app-shell";

/**
 * Layout of the private screens (S1).
 *
 * SessionGate and the shell live here once, instead of in each of the seven pages. The
 * public flows — `/login`, `/registro`, `/invitacion` — stay outside this group and
 * outside the shell.
 *
 * Auth behavior is unchanged: SessionGate still sends an unauthenticated visitor to
 * `/login`, and it is still a UX boundary, not a security one.
 */
export default function PrivateLayout({ children }: { children: ReactNode }) {
  return (
    <SessionGate>
      <AppShell>{children}</AppShell>
    </SessionGate>
  );
}
