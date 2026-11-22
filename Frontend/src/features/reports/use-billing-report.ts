"use client";

import { useEffect, useState } from "react";

import type { BillingReport, DayRange, ReportDimension } from "@/entities/report/model";
import { browserApi } from "@/shared/api/client";

/**
 * What one request to the billing report is: the state the screen draws, never a half-way value.
 * `loading` is the first load and also every reload, so a chart never shows the figures of the
 * previous range under the label of the new one.
 */
export type ReportState =
  | { status: "loading" }
  | { status: "failed" }
  | { status: "ready"; report: BillingReport };

/**
 * Reads the report of a range, cut by one dimension. The request is re-made when the range or
 * the cut changes, and an answer that arrives after that is dropped.
 *
 * Nothing here filters by professional: the API decides the scope from the role, so a
 * professional's request already comes back narrowed to their own figures.
 */
export function useBillingReport(range: DayRange, groupBy: ReportDimension): ReportState {
  const [state, setState] = useState<ReportState>({ status: "loading" });
  const { from, to } = range;

  useEffect(() => {
    let active = true;
    setState({ status: "loading" });

    void (async () => {
      try {
        const { data, response } = await browserApi.GET("/api/v1/reports/billing", {
          params: { query: { from, to, groupBy } },
        });
        if (!active) return;
        if (!response.ok || data === undefined) {
          setState({ status: "failed" });
          return;
        }
        setState({ status: "ready", report: data });
      } catch {
        if (active) setState({ status: "failed" });
      }
    })();

    return () => {
      active = false;
    };
  }, [from, to, groupBy]);

  return state;
}
