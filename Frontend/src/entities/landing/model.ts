import type { components } from "@/shared/api/schema";

/** Public landing text as the API publishes it (no tenant id or audit fields). */
export type PublicLanding = components["schemas"]["LandingBody"];