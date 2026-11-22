import Link from "next/link";

/**
 * The first screen of Aveniq at its own address (S24, T63).
 *
 * It is public and it is not the dashboard any more: the dashboard lives at `/panel`. T65 turns
 * this into the landing of the practice that owns the host, and until then it only says what
 * Aveniq is and leads to the sign-in. It does not look at the session on purpose, so it renders
 * the same for everyone and never flashes a screen that the gate would then replace.
 */
export default function HomePage() {
  return (
    <main className="mx-auto flex min-h-full w-full max-w-md flex-col justify-center gap-6 px-6 py-16">
      <header>
        <p className="text-sm font-semibold text-brand">Aveniq</p>
        <h1 className="mt-2 text-2xl font-semibold tracking-tight">Gestión de consultorios de psicología</h1>
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
