import { fetchPublicLanding } from "@/entities/landing/fetch-public";
import { PlatformHome } from "@/features/landing/components/platform-home";
import { PracticeLanding } from "@/features/landing/components/practice-landing";
import { requestHost } from "@/shared/request-host";

/**
 * The first screen at this address (S24, T65).
 *
 * The dashboard lives at `/panel`. This route is always public and does not read the
 * session: it shows either the practice that owns the host or, when the host names no
 * practice, the Aveniq platform screen with registration.
 */
export default async function HomePage() {
  const host = await requestHost();
  const landing = await fetchPublicLanding(host);

  if (landing) return <PracticeLanding landing={landing} />;
  return <PlatformHome />;
}