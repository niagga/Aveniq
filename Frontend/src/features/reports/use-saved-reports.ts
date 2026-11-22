"use client";

import { useCallback, useEffect, useState } from "react";

import type { SavedReport } from "@/entities/report/model";
import { browserApi } from "@/shared/api/client";

/** The saved reports of the practice, by name, and a way to read them again after a write. */
export function useSavedReports() {
  const [reports, setReports] = useState<SavedReport[] | null>(null);
  const [failed, setFailed] = useState(false);

  const load = useCallback(async () => {
    try {
      const { data, response } = await browserApi.GET("/api/v1/reports/saved");
      if (!response.ok || data === undefined) {
        setFailed(true);
        return;
      }
      setFailed(false);
      setReports(data.items ?? []);
    } catch {
      setFailed(true);
    }
  }, []);

  useEffect(() => {
    let active = true;
    void (async () => {
      if (active) await load();
    })();
    return () => {
      active = false;
    };
  }, [load]);

  return { reports, failed, reload: load };
}
