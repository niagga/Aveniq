import Link from "next/link";

import { fetchPublicLanding } from "@/entities/landing/fetch-public";
import { LoginForm } from "@/features/auth/components/login-form";
import { isValidSlug } from "@/entities/tenant/slug";
import { requestHost } from "@/shared/request-host";

type SearchParams = Promise<{ consultorio?: string | string[] }>;

/**
 * Sign-in is public. When the host names a practice, the slug is taken from there and
 * the consultorio field stays hidden (S24, T65). On a generic host, an invitation or
 * registration link can still pass `?consultorio=` as a fallback.
 */
export default async function LoginPage({ searchParams }: { searchParams: SearchParams }) {
  const params = await searchParams;
  const host = await requestHost();
  const landing = await fetchPublicLanding(host);

  const querySlug =
    typeof params.consultorio === "string" ? params.consultorio.toLowerCase() : "";
  const queryTenantSlug = isValidSlug(querySlug) ? querySlug : "";

  const resolvedTenantSlug = landing?.slug ?? queryTenantSlug;
  const hideTenantField = landing !== null;

  return (
    <main className="mx-auto flex min-h-full w-full max-w-md flex-col justify-center gap-6 px-6 py-16">
      <header>
        <p className="text-sm font-semibold text-brand">Aveniq</p>
        {landing ? (
          <p className="mt-1 text-sm text-ink-muted">{landing.displayName}</p>
        ) : (
          <p className="mt-1 text-sm text-ink-muted">
            Gestión de consultorios de psicología: pacientes, agenda y facturación.
          </p>
        )}
      </header>

      <LoginForm
        initialTenantSlug={resolvedTenantSlug}
        hideTenantField={hideTenantField}
      />

      {!landing ? (
        <p className="text-center text-xs text-ink-muted">
          ¿Tu consultorio todavía no está en Aveniq?{" "}
          <Link href="/registro" className="font-medium text-brand underline">
            Registralo con 14 días de prueba
          </Link>
        </p>
      ) : null}
    </main>
  );
}