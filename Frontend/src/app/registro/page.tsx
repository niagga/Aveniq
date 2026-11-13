import { SignupForm } from "@/features/signup/components/signup-form";

/**
 * Public registration of a practice. Like the sign-in screen it is rendered without a
 * session: it is how a practice starts.
 */
export default function SignupPage() {
  return (
    <main className="mx-auto flex min-h-full w-full max-w-md flex-col justify-center gap-6 px-6 py-16">
      <header>
        <p className="text-sm font-semibold text-brand">Aveniq</p>
        <p className="mt-1 text-sm text-ink-muted">
          Gestión de consultorios de psicología: pacientes, agenda y facturación.
        </p>
      </header>

      <SignupForm />
    </main>
  );
}
