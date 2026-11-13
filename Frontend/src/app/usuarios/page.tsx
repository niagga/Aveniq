"use client";

import { UsersScreen } from "@/features/users/components/users-screen";
import { SessionGate } from "@/shared/auth/session-gate";

/**
 * Accounts and invitations of the practice (T37). A Client Component inside SessionGate,
 * like the other screens: the access token lives in browser memory.
 */
export default function UsersPage() {
  return (
    <main className="mx-auto flex min-h-full w-full max-w-6xl flex-col gap-6 px-6 py-12">
      <SessionGate>
        <UsersScreen />
      </SessionGate>
    </main>
  );
}
