"use client";

import { useEffect, useState } from "react";

import { canReadBilling } from "@/entities/billing/model";
import { browserApi } from "@/shared/api/client";

/**
 * How many attended sessions of the last 30 days have no voucher.
 *
 * This is the number the billing screen was missing: the worklist endpoint exists now
 * (`GET /billing/unbilled`), and this reads only the size of it. The window is fixed at 30 days
 * and not "since forever" on purpose: an unbilled session from two years ago is a data problem,
 * not today's work, and a count that never goes down is a count nobody looks at.
 *
 * The page is bounded at 100 by the API, so a full page means "at least 100" and the caller
 * shows it that way instead of pretending to know the exact figure.
 */

const WINDOW_DAYS = 30;
const PAGE_LIMIT = 100;

export type UnbilledCount = { value: number; capped: boolean } | null;

export function useUnbilledCount(enabled: boolean): UnbilledCount {
  const [count, setCount] = useState<UnbilledCount>(null);

  useEffect(() => {
    if (!enabled) return;
    let active = true;

    void (async () => {
      const to = new Date();
      const from = new Date(to.getTime() - WINDOW_DAYS * 24 * 60 * 60 * 1000);
      try {
        const { data, response } = await browserApi.GET("/api/v1/billing/unbilled", {
          params: {
            query: { from: from.toISOString(), to: to.toISOString(), limit: PAGE_LIMIT },
          },
        });
        if (!active) return;
        if (!response.ok) {
          setCount(null);
          return;
        }
        const items = data?.items ?? [];
        setCount({ value: items.length, capped: items.length >= PAGE_LIMIT });
      } catch {
        if (active) setCount(null);
      }
    })();

    return () => {
      active = false;
    };
  }, [enabled]);

  return count;
}

/** Whether the rail should fetch the count at all: the roles that cannot read billing, cannot. */
export function canCountUnbilled(role: string | undefined): boolean {
  return canReadBilling(role);
}

/** The label of the badge: an exact number, or "100+" when the page filled up. */
export function unbilledLabel(count: UnbilledCount): string {
  if (count === null || count.value === 0) return "";
  return count.capped ? `${count.value}+` : String(count.value);
}
