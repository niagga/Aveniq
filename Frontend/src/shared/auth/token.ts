/**
 * The in-memory access token.
 *
 * It is deliberately not persisted anywhere: `localStorage` and `sessionStorage`
 * are readable by any script on the origin, so one XSS would walk away with a
 * live credential. The cost of that choice is that a reload starts empty, which
 * is why the session is re-established on boot from the HttpOnly refresh cookie
 * instead of from storage.
 *
 * This module has no imports on purpose: the API client depends on it, and the
 * session orchestration depends on both, so a cycle here would be permanent.
 */
let accessToken: string | null = null;

const listeners = new Set<() => void>();

/** Returns the current token, or null when there is no session in this tab. */
export function getAccessToken(): string | null {
  return accessToken;
}

/**
 * Replaces the token and notifies every subscriber.
 *
 * Returning early on an unchanged value keeps `useSyncExternalStore` from
 * re-rendering on every broadcast that carries the same session.
 */
export function setAccessToken(token: string | null): void {
  if (accessToken === token) return;
  accessToken = token;
  for (const listener of listeners) listener();
}

/** Subscribes to token changes; returns the unsubscribe function. */
export function subscribeToToken(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
