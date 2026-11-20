import type { components } from "@/shared/api/schema";

/**
 * A mutual (obra social or prepaid) of the practice's catalog, as the API describes it.
 * The wire types are the schema, not copies, so the screens cannot drift silently.
 */
export type Insurer = components["schemas"]["InsurerBody"];
export type InsurerProfile = components["schemas"]["InsurerProfileBody"];

/**
 * Owner and admin hold tenant:settings, which writes the catalog. Every role with
 * patients:read can read it, because the patient form needs the names to choose from.
 * The API enforces both; this only keeps a role off a screen that can only fail.
 */
export function canManageInsurers(role: string | undefined): boolean {
  return role === "owner" || role === "admin";
}
