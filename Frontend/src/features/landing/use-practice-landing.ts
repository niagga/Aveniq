"use client";

import { useCallback, useEffect, useState } from "react";

import type { PracticeLanding } from "@/entities/landing/model";
import { browserApi } from "@/shared/api/client";

/** The editable landing of the caller's practice. */
export function usePracticeLanding(enabled: boolean) {
  const [landing, setLanding] = useState<PracticeLanding | null>(null);
  const [failedStatus, setFailedStatus] = useState<number | null>(null);

  const reload = useCallback(async () => {
    const { data, response } = await browserApi.GET("/api/v1/practice/landing");
    if (!response.ok) {
      setFailedStatus(response.status);
      return;
    }
    setFailedStatus(null);
    if (data) setLanding(data);
  }, []);

  useEffect(() => {
    if (!enabled) return;
    let active = true;
    void (async () => {
      if (active) await reload();
    })();
    return () => {
      active = false;
    };
  }, [enabled, reload]);

  return { landing, failedStatus, reload, setLanding };
}