"use client";

import { useEffect, useState } from "react";

import { browserApi } from "@/shared/api/client";

export type Option = { id: string; name: string };

/**
 * The names a report can be narrowed by: the professionals and the mutuales of the practice.
 *
 * The roles that read the billing book (owner, admin, accountant) read both lists from billing,
 * which an accountant may open; the professional holds no billing and only ever sees their own
 * figures, so they get no professional list and read the mutuales from the catalog their patient
 * form already uses. A list that cannot be read is empty: the filter just offers fewer choices.
 */
export function useFilterOptions(role: string | undefined) {
  const [professionals, setProfessionals] = useState<Option[]>([]);
  const [insurers, setInsurers] = useState<Option[]>([]);

  useEffect(() => {
    let active = true;

    void (async () => {
      try {
        if (role === "professional") {
          const { data, response } = await browserApi.GET("/api/v1/insurers");
          if (active && response.ok) {
            setInsurers((data?.items ?? []).map((item) => ({ id: item.id, name: item.name })));
          }
          return;
        }

        const [people, mutuals] = await Promise.all([
          browserApi.GET("/api/v1/billing/professionals"),
          browserApi.GET("/api/v1/billing/settlement-insurers"),
        ]);
        if (!active) return;
        if (people.response.ok) {
          setProfessionals(
            (people.data?.items ?? []).map((item) => ({ id: item.id, name: item.fullName })),
          );
        }
        if (mutuals.response.ok) {
          setInsurers((mutuals.data?.items ?? []).map((item) => ({ id: item.id, name: item.name })));
        }
      } catch {
        // The filters are optional: without the lists the report is still readable.
      }
    })();

    return () => {
      active = false;
    };
  }, [role]);

  return { professionals, insurers };
}
