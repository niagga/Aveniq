"use client";

import { useRouter } from "next/navigation";
import { useId, useState } from "react";

import { login } from "@/shared/auth/session";

/**
 * Sign-in form.
 *
 * The tenant is a field and not something inferred from the host: the email is
 * unique per practice, not platform-wide, so the API cannot resolve who is signing
 * in without being told which practice to look in.
 */
export function LoginForm({ initialTenantSlug = "" }: { initialTenantSlug?: string }) {
  const router = useRouter();
  const [tenantSlug, setTenantSlug] = useState(initialTenantSlug);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const tenantId = useId();
  const emailId = useId();
  const passwordId = useId();

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setIsSubmitting(true);

    try {
      const result = await login({
        tenantSlug: tenantSlug.trim().toLowerCase(),
        email: email.trim(),
        password,
      });

      if (!result.ok) {
        // One message for every failure. The API already refuses to say whether the
        // tenant, the email or the password was the wrong part, and the client must
        // not helpfully guess.
        setError(result.message);
        return;
      }

      setPassword("");
      router.replace("/");
    } catch {
      setError("No pudimos contactar al servidor. Revisá que la API esté corriendo.");
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="rounded-[var(--radius-panel)] border border-border bg-surface p-6"
      noValidate
    >
      <h1 className="text-lg font-semibold">Iniciar sesión</h1>
      <p className="mt-1 text-sm text-ink-muted">
        Entrá con la cuenta de tu consultorio.
      </p>

      <div className="mt-5 grid gap-4">
        <div>
          <label htmlFor={tenantId} className="block text-xs font-medium text-ink-muted">
            Consultorio
          </label>
          <input
            id={tenantId}
            name="tenantSlug"
            value={tenantSlug}
            onChange={(event) => setTenantSlug(event.target.value)}
            required
            maxLength={63}
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            placeholder="consultorio-sur"
            className="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 font-mono text-sm outline-none focus:border-brand"
          />
        </div>

        <div>
          <label htmlFor={emailId} className="block text-xs font-medium text-ink-muted">
            Email
          </label>
          <input
            id={emailId}
            name="email"
            type="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            required
            maxLength={320}
            autoComplete="username"
            placeholder="ana@consultorio.test"
            className="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-brand"
          />
        </div>

        <div>
          <label htmlFor={passwordId} className="block text-xs font-medium text-ink-muted">
            Contraseña
          </label>
          <input
            id={passwordId}
            name="password"
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            required
            maxLength={1024}
            autoComplete="current-password"
            className="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-brand"
          />
        </div>
      </div>

      {error ? (
        <p role="alert" className="mt-4 text-sm text-danger">
          {error}
        </p>
      ) : null}

      <button
        type="submit"
        disabled={isSubmitting}
        className="mt-5 inline-flex items-center rounded-md bg-brand px-4 py-2 text-sm font-medium text-brand-ink disabled:opacity-60"
      >
        {isSubmitting ? "Entrando…" : "Entrar"}
      </button>
    </form>
  );
}
