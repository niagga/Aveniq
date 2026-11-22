import type { components } from "@/shared/api/schema";

/** Public landing text as the API publishes it (no tenant id or audit fields). */
export type PublicLanding = components["schemas"]["LandingBody"];

/** The landing of the signed-in practice, for editing (GET/PUT /practice/landing). */
export type PracticeLanding = components["schemas"]["LandingBody"];

export type LandingDraft = components["schemas"]["LandingDraftBody"];

/** Mirrors the backend domain limits (landing/profile.go, migration 0025). */
export const LANDING_LIMITS = {
  displayName: 120,
  tagline: 160,
  about: 2000,
  address: 200,
  phone: 40,
  email: 254,
  schedule: 300,
} as const;

/**
 * Owner and admin hold tenant:settings, which edits the landing (T64). The API enforces it;
 * this keeps other roles off a screen that can only fail.
 */
export function canManageLanding(role: string | undefined): boolean {
  return role === "owner" || role === "admin";
}

/** Local development: `<slug>.localhost` under BASE_DOMAIN=localhost. */
export function practiceHostHint(slug: string): string {
  return `${slug}.localhost`;
}