"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

import { ensureSession, startSessionChannel } from "@/shared/auth/session";

type Status = "checking" | "authenticated" | "anonymous";

/**
 * Client-side session gate.
 *
 * On mount it asks for a live access token; the answer comes from memory or, after
 * a reload, from one refresh against the HttpOnly cookie. Without it the user is
 * sent to the login screen.
 *
 * It is a UX boundary, not a security boundary: the access token is what the API
 * verifies, and today the API does not require one for every route. Gating the
 * routes themselves is T19. Until then, a Server Component's payload is rendered
 * before this component ever runs, which is exactly why the honest claim here is
 * "the user sees the right screen", not "the data is protected".
 */
export function SessionGate({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const [status, setStatus] = useState<Status>("checking");

  useEffect(() => {
    // Wired once per tab, so another tab's rotation is adopted instead of spent.
    startSessionChannel();

    let active = true;
    void ensureSession().then((authenticated) => {
      if (!active) return;

      if (authenticated) {
        setStatus("authenticated");
        return;
      }

      setStatus("anonymous");
      router.replace("/login");
    });

    return () => {
      active = false;
    };
  }, [router]);

  if (status !== "authenticated") {
    return (
      <p role="status" className="text-sm text-ink-muted">
        {status === "checking" ? "Verificando sesión…" : "Redirigiendo al inicio de sesión…"}
      </p>
    );
  }

  return <>{children}</>;
}
