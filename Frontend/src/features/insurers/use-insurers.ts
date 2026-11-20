"use client";

import { useCallback, useEffect, useState } from "react";

import type { Insurer } from "@/entities/insurer/model";
import { browserApi } from "@/shared/api/client";

/**
 * The practice's mutuals, deactivated ones included. A patient keeps the name of a mutual
 * that stopped taking new patients, so the roster must still resolve it.
 */
export function useInsurers(enabled: boolean) {
  const [insurers, setInsurers] = useState<Insurer[] | null>(null);
  const [failedStatus, setFailedStatus] = useState<number | null>(null);

  const load = useCallback(async () => {
    const { data, response } = await browserApi.GET("/api/v1/insurers");
    if (!response.ok) {
      setFailedStatus(response.status);
      return;
    }
    setFailedStatus(null);
    setInsurers(data?.items ?? []);
  }, []);

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    void (async () => {
      if (active) await load();
    })();
    return () => {
      active = false;
    };
  }, [enabled, load]);

  return { insurers, failedStatus, reload: load };
}
