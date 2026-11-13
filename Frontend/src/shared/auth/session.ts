import { apiErrorMessage, browserApi } from "@/shared/api/client";
import { getAccessToken, setAccessToken } from "@/shared/auth/token";

declare global {
  interface Window {
    /** Development-only hook for the two-tab refresh test (T32). */
    __aveniqEnsureSession?: () => Promise<boolean>;
  }
}

/**
 * Session orchestration for the browser.
 *
 * Three constraints shape this module, and none of them is cosmetic:
 *
 * 1. The access token lives in memory only (see `token.ts`), so a reload starts
 *    with nothing. The session is re-established from the HttpOnly refresh cookie.
 * 2. Every refresh SPENDS the presented refresh token. The server answers a spent
 *    token by revoking the whole successor chain, so two tabs refreshing at once
 *    would log each other out. That is why refreshes are serialized with the Web
 *    Locks API and the result is shared through a BroadcastChannel instead of
 *    each tab spending a rotation of its own.
 * 3. The access token expires in 15 minutes, so a timer refreshes slightly before
 *    it does. Reacting to a 401 would work too, but a request that fails on an
 *    expired token is already a lost request.
 */

export type SessionUser = {
  id: string;
  tenantId: string;
  role: string;
};

type Session = {
  expiresAt: number;
  user: SessionUser;
};

type SessionPayload = {
  accessToken: string;
  expiresIn: number;
  user: SessionUser;
};

type Message =
  | { type: "session"; token: string; expiresAt: number; user: SessionUser }
  | { type: "logout" };

/** Refresh this long before the access token expires. */
const REFRESH_LEAD_MS = 60_000;

/** Never schedule a refresh closer than this, to avoid a tight loop on a clock skew. */
const MIN_REFRESH_DELAY_MS = 5_000;

const LOCK_NAME = "aveniq-refresh";
const CHANNEL_NAME = "aveniq-session";

let session: Session | null = null;
let inFlight: Promise<boolean> | null = null;
let refreshTimer: ReturnType<typeof setTimeout> | null = null;
let channel: BroadcastChannel | null = null;

/** Reports who is signed in, or null. */
export function getSessionUser(): SessionUser | null {
  return session?.user ?? null;
}

/**
 * Reads the session without asking the server.
 *
 * `false` does not mean "signed out": it means "no live access token in this tab",
 * which after a reload is the normal state until `ensureSession` runs.
 */
export function hasLiveSession(): boolean {
  return getAccessToken() !== null && session !== null && Date.now() < session.expiresAt;
}

/**
 * Guarantees a usable access token, refreshing once if needed.
 *
 * This is the entry point for a route that requires a session: the result tells
 * the caller whether to render the page or send the user to the login screen.
 */
export async function ensureSession(): Promise<boolean> {
  if (typeof window === "undefined") return false;
  if (hasLiveSession()) return true;

  // A refresh already running in this tab is reused instead of started twice.
  if (inFlight) return inFlight;

  inFlight = withLock(async () => {
    // While this tab waited for the lock, another one may have refreshed and
    // broadcast the result. Spending another rotation here would be the bug.
    if (hasLiveSession()) return true;
    return refreshFromCookie();
  }).finally(() => {
    inFlight = null;
  });

  return inFlight;
}

/** Authenticates against the API and adopts the resulting session. */
export async function login(input: {
  tenantSlug: string;
  email: string;
  password: string;
}): Promise<{ ok: true } | { ok: false; message: string }> {
  const { data, error, response } = await browserApi.POST("/api/v1/auth/login", {
    body: input,
  });

  if (!response.ok || !data) {
    return { ok: false, message: apiErrorMessage(error) };
  }

  adopt(data);
  publish();
  return { ok: true };
}

/**
 * Revokes the session and forgets it locally.
 *
 * The local state is cleared even if the request fails: leaving a usable token in
 * memory because the network blinked is worse than a server-side grant that
 * expires on its own.
 */
export async function logout(): Promise<void> {
  clearLocal();
  publishLogout();

  try {
    await browserApi.POST("/api/v1/auth/logout");
  } catch {
    // The cookie is still cleared by the server on its own expiry, and the access
    // token is already gone from this tab.
  }
}

/** Refreshes once, inside the cross-tab lock. */
async function refreshFromCookie(): Promise<boolean> {
  const { data, response } = await browserApi.POST("/api/v1/auth/refresh");

  // A failed refresh is terminal: the server either could not find the session or
  // found it revoked. Either way the only honest state is "signed out", and the
  // caller redirects to the login screen.
  if (!response.ok || !data) {
    clearLocal();
    return false;
  }

  adopt(data);
  publish();
  return true;
}

/** Serializes a critical section across tabs, when the browser supports it. */
async function withLock(run: () => Promise<boolean>): Promise<boolean> {
  if (typeof navigator === "undefined" || !("locks" in navigator)) {
    return run();
  }
  return navigator.locks.request(LOCK_NAME, run);
}

function adopt(payload: SessionPayload): void {
  session = {
    expiresAt: Date.now() + payload.expiresIn * 1000,
    user: payload.user,
  };
  setAccessToken(payload.accessToken);
  scheduleRefresh(payload.expiresIn * 1000);
}

function scheduleRefresh(lifetimeMs: number): void {
  if (refreshTimer !== null) clearTimeout(refreshTimer);

  const delay = Math.max(MIN_REFRESH_DELAY_MS, lifetimeMs - REFRESH_LEAD_MS);
  refreshTimer = setTimeout(() => {
    refreshTimer = null;
    void ensureSession();
  }, delay);
}

function clearLocal(): void {
  session = null;
  setAccessToken(null);
  if (refreshTimer !== null) {
    clearTimeout(refreshTimer);
    refreshTimer = null;
  }
}

/** Shares the new session so other tabs adopt it instead of rotating again. */
function publish(): void {
  const token = getAccessToken();
  if (!session || token === null) return;

  channel?.postMessage({
    type: "session",
    token,
    expiresAt: session.expiresAt,
    user: session.user,
  } satisfies Message);
}

function publishLogout(): void {
  channel?.postMessage({ type: "logout" } satisfies Message);
}

/**
 * Wires the cross-tab channel once.
 *
 * `BroadcastChannel` is available in every browser this app targets; the guard
 * keeps the module safe to import during server rendering and in tests.
 */
export function startSessionChannel(): void {
  if (channel !== null || typeof window === "undefined" || typeof BroadcastChannel === "undefined") {
    return;
  }

  channel = new BroadcastChannel(CHANNEL_NAME);

  // T32: two-tab Playwright needs a way to fire the same refresh path the timer
  // uses. Production never reads this; it is unset in the production bundle.
  if (process.env.NODE_ENV !== "production") {
    window.__aveniqEnsureSession = ensureSession;
  }

  channel.onmessage = (event: MessageEvent<Message>) => {
    const message = event.data;

    if (message.type === "logout") {
      clearLocal();
      return;
    }

    // Another tab refreshed: adopt it without touching the cookie, which now holds
    // a token this tab never saw.
    session = { expiresAt: message.expiresAt, user: message.user };
    setAccessToken(message.token);
    scheduleRefresh(message.expiresAt - Date.now());
  };
}
