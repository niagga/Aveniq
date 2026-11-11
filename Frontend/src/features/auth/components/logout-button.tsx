"use client";

import { useRouter } from "next/navigation";
import { useSyncExternalStore, useState } from "react";

import { getSessionUser, logout } from "@/shared/auth/session";
import { getAccessToken, subscribeToToken } from "@/shared/auth/token";

/**
 * Ends the session and returns to the login screen.
 *
 * The role shown next to the button comes from the session, which is the same
 * claim the API reads: if they ever disagree, the API wins.
 */
export function LogoutButton() {
  const router = useRouter();
  const [isSubmitting, setIsSubmitting] = useState(false);

  // Subscribing keeps the label honest across a silent refresh in another tab.
  useSyncExternalStore(subscribeToToken, getAccessToken, () => null);
  const user = getSessionUser();

  async function handleClick() {
    setIsSubmitting(true);
    await logout();
    router.replace("/login");
  }

  return (
    <div className="flex items-center gap-3">
      {user ? (
        <span className="text-xs text-ink-muted">
          {user.role} · {user.tenantId}
        </span>
      ) : null}
      <button
        type="button"
        onClick={handleClick}
        disabled={isSubmitting}
        className="rounded-md border border-border px-3 py-1.5 text-xs font-medium text-ink-muted disabled:opacity-60"
      >
        {isSubmitting ? "Saliendo…" : "Cerrar sesión"}
      </button>
    </div>
  );
}
