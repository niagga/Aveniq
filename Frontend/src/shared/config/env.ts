/**
 * Server-only environment.
 *
 * `API_ORIGIN` must never reach the browser bundle: the browser talks to the
 * Next.js server, which forwards to the Go API. Keeping this module server-only
 * is what makes that boundary real rather than a convention.
 */
export const serverEnv = {
  apiOrigin: process.env.API_ORIGIN ?? "http://127.0.0.1:8080",
} as const;
