import Link from "next/link";

import { LoginForm } from "@/features/auth/components/login-form";
import { isValidSlug } from "@/entities/tenant/slug";

type SearchParams = Promise<{ consultorio?: string | string[] }>;

/**
 * The sign-in screen is public by definition: it is the only page rendered
 * without a session, and the gate sends the user back here when a refresh fails.
 *
 * A link from an invitation or a new registration can name the practice, so the
 * practice field arrives filled in. A value that is not a slug is ignored.
 */
export default async function LoginPage({ searchParams }: { searchParams: SearchParams }) {
  const params = await searchParams;
  const slug = typeof params.consultorio === "string" ? params.consultorio.toLowerCase() : "";
  const initialTenantSlug = isValidSlug(slug) ? slug : "";

  return (
    <main className="mx-auto flex min-h-full w-full max-w-md flex-col justify-center gap-6 px-6 py-16">
      <header>
        <p className="text-sm font-semibold text-brand">Aveniq</p>
        <p className="mt-1 text-sm text-ink-muted">
          Gestión de consultorios de psicología: pacientes, agenda y facturación.
        </p>
      </header>

      <LoginForm initialTenantSlug={initialTenantSlug} />

      <p className="text-center text-xs text-ink-muted">
        ¿Tu consultorio todavía no está en Aveniq?{" "}
        <Link href="/registro" className="font-medium text-brand underline">
          Registralo con 14 días de prueba
        </Link>
      </p>
    </main>
  );
}
