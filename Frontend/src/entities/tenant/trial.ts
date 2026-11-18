import type { Tenant } from "@/entities/tenant/model";

/**
 * Whether the practice is on trial and that trial has ended.
 *
 * Mirrors tenant.TrialExpired on the API: inclusive at trialEndsAt, and only a
 * practice in status `trial` can expire. An active practice never expires here.
 * The API is still the authority on a write; this only lets the screen stop
 * offering forms that the server will refuse with 403 trial_expired.
 */
export function trialExpired(practice: Pick<Tenant, "status" | "trialEndsAt">, now: Date = new Date()): boolean {
  if (practice.status !== "trial") return false;
  const ends = new Date(practice.trialEndsAt);
  if (Number.isNaN(ends.getTime())) return false;
  return now.getTime() >= ends.getTime();
}
