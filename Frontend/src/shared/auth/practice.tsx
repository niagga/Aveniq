"use client";

import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

import type { Tenant } from "@/entities/tenant/model";
import type { components } from "@/shared/api/schema";
import { trialExpired } from "@/entities/tenant/trial";
import { canReadPractice } from "@/entities/iam/model";
import { browserApi } from "@/shared/api/client";
import { getSessionUser } from "@/shared/auth/session";

/** The state of the practice every role may read (GET /practice/status). */
export type PracticeStatus = components["schemas"]["PracticeStatusBody"];

type PracticeState = {
  /** The practice as tenant:read roles see it, with its name and slug. Null for the others. */
  practice: Tenant | null;
  /** The trial and the time zone of the practice, for every role. Null until it is read. */
  status: PracticeStatus | null;
  /** True when the practice is on trial and the trial has ended. */
  expired: boolean;
};

const PracticeContext = createContext<PracticeState>({ practice: null, status: null, expired: false });

/**
 * Loads the current practice so authenticated screens can show the trial banner
 * and stop offering writes the API will refuse.
 *
 * Two reads, because two things are true. The state of the practice (trial, time zone, and
 * whether the trial ended on the server's clock) is readable by every role, accountant
 * included: it is what keeps a screen from offering a write that will answer 403 (L59). The
 * practice itself, with its name and slug, needs tenant:read and is only read by the roles
 * that hold it.
 */
export function PracticeProvider({ children }: { children: ReactNode }) {
  const [practice, setPractice] = useState<Tenant | null>(null);
  const [status, setStatus] = useState<PracticeStatus | null>(null);
  const user = getSessionUser();
  const mayRead = canReadPractice(user?.role);

  useEffect(() => {
    let active = true;

    void (async () => {
      const { data, response } = await browserApi.GET("/api/v1/practice/status");
      if (active && response.ok && data !== undefined) setStatus(data);
    })();

    return () => {
      active = false;
    };
  }, [user?.tenantId]);

  useEffect(() => {
    if (!mayRead) return;
    let active = true;

    void (async () => {
      const { data, response } = await browserApi.GET("/api/v1/tenants");
      if (!active || !response.ok) return;
      const items = data?.items ?? [];
      const match =
        items.find((item) => item.id === user?.tenantId) ?? items[0] ?? null;
      setPractice(match);
    })();

    return () => {
      active = false;
    };
  }, [mayRead, user?.tenantId]);

  // The server's answer wins. If the status could not be read, the practice's own dates give
  // the same rule for the roles that can read it, as before.
  const expired = status !== null ? status.expired : practice !== null && trialExpired(practice);
  const value = useMemo(() => ({ practice, status, expired }), [practice, status, expired]);

  return (
    <PracticeContext.Provider value={value}>{children}</PracticeContext.Provider>
  );
}
/**
 * The current practice, for the shell that shows it.
 *
 * The banner used to render here, above every screen and above the rail: it sat outside the
 * shell's layout because the provider wraps it. The shell renders it now, inside the content
 * column, where a notice about the practice belongs.
 */
export function usePractice(): PracticeState {
  return useContext(PracticeContext);
}

/** Whether write controls must be disabled because the trial has ended. */
export function useWritesLocked(): boolean {
  return useContext(PracticeContext).expired;
}
