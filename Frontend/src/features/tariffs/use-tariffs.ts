"use client";

import { useCallback, useEffect, useState } from "react";

import type { Tariff } from "@/entities/tariff/model";
import { browserApi } from "@/shared/api/client";

/**
 * The practice's fee schedule, every price in force or past. The API answers the names of the
 * professional and the mutual with each row, so the accountant, who cannot read either list,
 * still sees who each price belongs to.
 */
export function useTariffs(enabled: boolean) {
  const [tariffs, setTariffs] = useState<Tariff[] | null>(null);
  const [failedStatus, setFailedStatus] = useState<number | null>(null);

  const load = useCallback(async () => {
    const { data, response } = await browserApi.GET("/api/v1/tariffs");
    if (!response.ok) {
      setFailedStatus(response.status);
      return;
    }
    setFailedStatus(null);
    setTariffs(data?.items ?? []);
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

  return { tariffs, failedStatus, reload: load };
}
