import "server-only";

import type { PublicLanding } from "@/entities/landing/model";
import { serverApi } from "@/shared/api/server";

/**
 * Loads the landing of the practice that owns a host.
 *
 * Unknown hosts, foreign domains and suspended practices all answer 404 from the
 * API on purpose, so the caller shows the Aveniq platform screen instead.
 */
export async function fetchPublicLanding(host: string): Promise<PublicLanding | null> {
  if (!host) return null;

  const { data, error, response } = await serverApi.GET("/api/v1/public/landing", {
    params: { query: { host } },
  });

  if (response.ok && data) return data;
  if (response.status === 404) return null;
  // Rate limit, validation, or the API is down: fall back to the platform screen rather
  // than failing SSR.
  if (error) return null;
  return null;
}