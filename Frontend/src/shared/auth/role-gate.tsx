"use client";

import { getSessionUser } from "@/shared/auth/session";
import { Notice } from "@/shared/ui/notice";

/**
 * Keeps a screen away from a role that holds no permission for it. The API refuses those
 * requests anyway; this stops the user from landing on a screen that can only fail.
 *
 * It renders inside SessionGate, so the session is already known when it runs.
 */
export function RoleGate({
  allowed,
  children,
}: {
  allowed: (role: string | undefined) => boolean;
  children: React.ReactNode;
}) {
  if (!allowed(getSessionUser()?.role)) {
    return (
      <Notice title="Tu rol no tiene acceso a esta pantalla.">
        Volvé al consultorio para seguir trabajando.
      </Notice>
    );
  }
  return <>{children}</>;
}
