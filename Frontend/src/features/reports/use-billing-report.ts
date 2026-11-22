"use client";

import { useEffect, useState } from "react";

import {
  NO_FILTERS,
  type BillingReport,
  type DayRange,
  type ReportDimension,
  type ReportFilters,
} from "@/entities/report/model";
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
 * Reads the report of a range, cut by one dimension and narrowed by the filters. The request is
 * re-made when any of them changes, and an answer that arrives after that is dropped.
 *
 * Nothing here scopes by role: the API decides it, so a professional's request already comes
 * back narrowed to their own figures whatever filter the screen sends.
 */
export function useBillingReport(
  range: DayRange,
  groupBy: ReportDimension,
  filters: ReportFilters = NO_FILTERS,
): ReportState {
  const [state, setState] = useState<ReportState>({ status: "loading" });
  const { from, to } = range;
  const { professionalId, coverage, insurerId, modality } = filters;

  useEffect(() => {
    let active = true;
    setState({ status: "loading" });

    void (async () => {
      try {
        const { data, response } = await browserApi.GET("/api/v1/reports/billing", {
          params: {
            query: {
              from,
              to,
              groupBy,
              // An empty filter is left out: the API reads absence as "no filter".
              ...(professionalId !== "" ? { professionalId } : {}),
              ...(coverage !== "" ? { coverage } : {}),
              ...(insurerId !== "" ? { insurerId } : {}),
              ...(modality !== "" ? { modality } : {}),
            },
          },
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
  }, [from, to, groupBy, professionalId, coverage, insurerId, modality]);

  return state;
}
