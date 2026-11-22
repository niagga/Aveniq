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
 * A rejected token is a session problem, not a screen problem (T12).
 *
 * The response still returns 401 to its caller - the screen decides what to say - but the
 * session layer gets a chance to refresh, and sends the user to the login screen when the
 * refresh cookie is gone too. Without this, an expired token left every form open with the
 * raw API sentence in it and no way forward but a manual reload.
 *
 * The auth endpoints are excluded: a wrong password is a 401 that must not start a recovery
 * loop. The import is dynamic so the client does not statically depend on the session layer,
 * which already depends on this client.
 */
browserApi.use({
  onResponse({ request, response }) {
    if (response.status !== 401) return response;

    const path = new URL(request.url).pathname;
    if (path.endsWith("/auth/login") || path.endsWith("/auth/refresh")) return response;

    // The public flows have no session to recover: an invitation link that does not open is
    // an error to show on its own screen, not a reason to send anyone to the sign-in form.
    const page = window.location.pathname;
    if (PUBLIC_PATHS.some((publicPath) => page.startsWith(publicPath))) return response;

    void import("@/shared/auth/unauthorized").then((module) => module.recoverSessionOnce());
    return response;
  },
});

/** Screens that render without a session, where a 401 is their own answer to show. */
const PUBLIC_PATHS = ["/login", "/registro", "/invitacion"];

/** Shown when the request never reached the API at all. */
export const CONNECTION_ERROR_MESSAGE =
  "No pudimos contactar al servidor. Revisá que la API esté corriendo.";

/** The stable `code` an API problem carries (for example `trial_expired`), if it has one. */
export function apiErrorCode(error: unknown): string | null {
  if (error && typeof error === "object") {
    const { code } = error as { code?: unknown };
    if (typeof code === "string" && code.length > 0) return code;
  }
  return null;
}

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
    const candidate = error as { code?: unknown; detail?: unknown; title?: unknown };
    if (candidate.code === "trial_expired") {
      return "La prueba de este consultorio venció. Podés seguir leyendo, no se puede cambiar nada.";
    }
    if (typeof candidate.detail === "string" && candidate.detail.length > 0) {
      return candidate.detail;
    }
    if (typeof candidate.title === "string" && candidate.title.length > 0) {
      return candidate.title;
    }
  }
  return "Ocurrió un error inesperado.";
}
