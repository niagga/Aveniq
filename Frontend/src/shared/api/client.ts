import createClient from "openapi-fetch";

import type { paths } from "@/shared/api/schema";

/**
 * Browser API client.
 *
 * `baseUrl` is the origin itself because the OpenAPI paths already contain
 * `/api/v1`: the same-origin rewrite in next.config.ts forwards them to Go.
 */
export const browserApi = createClient<paths>({
  baseUrl: "/",
  credentials: "same-origin",
});

/**
 * Extracts a human-readable message from an API error.
 *
 * The API answers with RFC 9457 problem details, so `detail` is the specific
 * message and `title` the category. Falling back instead of throwing keeps a
 * broken error payload from becoming a blank screen.
 */
export function apiErrorMessage(error: unknown): string {
  if (typeof error === "string") return error;
  if (error && typeof error === "object") {
    const candidate = error as { detail?: unknown; title?: unknown };
    if (typeof candidate.detail === "string" && candidate.detail.length > 0) {
      return candidate.detail;
    }
    if (typeof candidate.title === "string" && candidate.title.length > 0) {
      return candidate.title;
    }
  }
  return "Ocurrió un error inesperado.";
}
