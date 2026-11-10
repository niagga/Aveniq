import "server-only";

import createClient from "openapi-fetch";

import type { paths } from "@/shared/api/schema";
import { serverEnv } from "@/shared/config/env";

/**
 * Server-side API client.
 *
 * Server Components have no document to resolve a relative URL against, so this
 * client targets the API origin directly. The `server-only` import makes it a
 * build error to reach this module from a Client Component.
 */
export const serverApi = createClient<paths>({
  baseUrl: serverEnv.apiOrigin,
});
