"use client";

import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

import type { Tenant } from "@/entities/tenant/model";
import { trialExpired } from "@/entities/tenant/trial";
import { TrialExpiredBanner } from "@/features/tenants/components/trial-expired-banner";
import { canReadPractice } from "@/entities/iam/model";
import { browserApi } from "@/shared/api/client";
import { getSessionUser } from "@/shared/auth/session";

type PracticeState = {
  practice: Tenant | null;
  /** True when the API says this practice is on trial and trialEndsAt has passed. */
  expired: boolean;
};

const PracticeContext = createContext<PracticeState>({ practice: null, expired: false });

/**
 * Loads the current practice so authenticated screens can show the trial banner
 * and stop offering writes the API will refuse.
 *
 * GET /tenants needs tenant:read. The accountant does not hold it, so this
 * provider stays silent there: billing forms stay enabled until a write comes
 * back 403 trial_expired. That gap is recorded in L59; closing it needs the
 * trial fields on a route the accountant can read.
 */
export function PracticeProvider({ children }: { children: ReactNode }) {
  const [practice, setPractice] = useState<Tenant | null>(null);
  const user = getSessionUser();
  const mayRead = canReadPractice(user?.role);

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

  const expired = practice !== null && trialExpired(practice);
  const value = useMemo(() => ({ practice, expired }), [practice, expired]);

  return (
    <PracticeContext.Provider value={value}>
      {expired && practice ? <TrialExpiredBanner trialEndsAt={practice.trialEndsAt} /> : null}
      {children}
    </PracticeContext.Provider>
  );
}

/** Whether write controls must be disabled because the trial has ended. */
export function useWritesLocked(): boolean {
  return useContext(PracticeContext).expired;
}
