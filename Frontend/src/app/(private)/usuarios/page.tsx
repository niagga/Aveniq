"use client";

import { UsersScreen } from "@/features/users/components/users-screen";
import { ScreenFrame } from "@/shared/ui/shell/app-shell";

/**
 * Accounts and invitations of the practice (T37). A Client Component, like the other
 * screens: the access token lives in browser memory. The auth gate and the shell come
 * from the private layout.
 */
export default function UsersPage() {
  return (
    <ScreenFrame>
      <UsersScreen />
    </ScreenFrame>
  );
}
