import createClient from "openapi-fetch";

import type { paths } from "@/shared/api/schema";
import { getAccessToken } from "@/shared/auth/token";

/**
 * Browser API client.
 *
 * `baseUrl` is the origin itself because the OpenAPI paths already contain
 * `/api/v1`: the same-origin rewrite in next.config.ts forwards them to Go.
 * `credentials: "same-origin"` is what carries the HttpOnly refresh cookie; the
 * access token travels in the header, attached below.
 */
export const browserApi = createClient<paths>({
  baseUrl: "/",
  credentials: "same-origin",
});

/**
 * Attaches the in-memory access token to every request.
 *
 * The token is read per request instead of captured once, so a rotation picked up
 * from the timer or from another tab is used immediately.
 */
browserApi.use({
  onRequest({ request }) {
    const token = getAccessToken();
    if (token !== null) {
      request.headers.set("Authorization", `Bearer ${token}`);
    }
    return request;
  },
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
