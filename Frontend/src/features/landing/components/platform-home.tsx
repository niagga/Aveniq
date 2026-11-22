import Link from "next/link";

/**
 * Aveniq's own front door when the host does not name a practice (L68 rule 3).
 *
 * It stays public and does not look at the session, so it never flashes a screen
 * the session gate would replace.
 */
export function PlatformHome() {
  return (
    <main className="mx-auto flex min-h-full w-full max-w-md flex-col justify-center gap-6 px-6 py-16">
      <header>
        <p className="text-sm font-semibold text-brand">Aveniq</p>
        <h1 className="mt-2 text-2xl font-semibold tracking-tight">
          Gestión de consultorios de psicología
        </h1>
        <p className="mt-2 text-sm text-ink-muted">Pacientes, agenda y facturación en un solo lugar.</p>
      </header>

      <Link
        href="/login"
        className="inline-flex w-fit items-center rounded-control border border-brand bg-brand px-4 py-2 text-sm font-medium text-brand-ink hover:bg-brand/90"
      >
        Entrar
      </Link>

      <p className="text-xs text-ink-muted">
        ¿Tu consultorio todavía no está en Aveniq?{" "}
        <Link href="/registro" className="font-medium text-brand underline">
          Registralo con 14 días de prueba
        </Link>
      </p>
    </main>
  );
}