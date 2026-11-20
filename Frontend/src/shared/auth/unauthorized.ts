import { setAccessToken } from "@/shared/auth/token";
import { ensureSession } from "@/shared/auth/session";

/**
 * What happens when the API rejects the access token (S5, T12).
 *
 * A 401 used to travel straight back to whatever screen made the request, so the user saw
 * the raw sentence "The access token is not valid" inside a form they had already filled in,
 * with no way forward but a manual reload. The token lives in memory and expires; the refresh
 * cookie renews it. This is the place that tries that, and the place that gives up visibly.
 *
 * It is idempotent on purpose: one expired token produces a burst of 401s, and a burst of
 * refreshes would spend several rotations to answer the same question.
 */

let recovering: Promise<boolean> | null = null;

export async function recoverSessionOnce(): Promise<boolean> {
  if (recovering !== null) return recovering;

  recovering = (async () => {
    // The rejected token must not be reused: dropping it is what makes ensureSession refresh
    // instead of answering "there is already a live session".
    setAccessToken(null);

    const recovered = await ensureSession();
    if (!recovered && typeof window !== "undefined") {
      window.location.assign("/login");
    }
    return recovered;
  })().finally(() => {
    recovering = null;
  });

  return recovering;
}
