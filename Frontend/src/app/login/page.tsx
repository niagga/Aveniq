import { LoginForm } from "@/features/auth/components/login-form";

/**
 * The sign-in screen is public by definition: it is the only page rendered
 * without a session, and the gate sends the user back here when a refresh fails.
 */
export default function LoginPage() {
  return (
    <main className="mx-auto flex min-h-full w-full max-w-md flex-col justify-center gap-6 px-6 py-16">
      <header>
        <p className="text-sm font-semibold text-brand">Aveniq</p>
        <p className="mt-1 text-sm text-ink-muted">
          Gestión de consultorios de psicología: pacientes, agenda y facturación.
        </p>
      </header>

      <LoginForm />
    </main>
  );
}
