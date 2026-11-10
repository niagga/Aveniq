"use client";

import { useRouter } from "next/navigation";
import { useId, useState, useTransition } from "react";

import { apiErrorMessage, browserApi } from "@/shared/api/client";

const SLUG_PATTERN = /^[a-z0-9]+(-[a-z0-9]+)*$/;

/**
 * Creates a tenant through the same-origin proxy.
 *
 * The form owns its own busy and error state instead of pushing them upward: it
 * is the only consumer, and a shared store here would buy nothing.
 */
export function CreateTenantForm() {
  const router = useRouter();
  const [isRefreshing, startTransition] = useTransition();
  const [slug, setSlug] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const slugId = useId();
  const nameId = useId();
  const slugHintId = useId();

  const slugLooksValid = slug === "" || SLUG_PATTERN.test(slug);
  const busy = isSubmitting || isRefreshing;

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setIsSubmitting(true);

    try {
      const { error: apiError } = await browserApi.POST("/api/v1/tenants", {
        body: { slug: slug.trim().toLowerCase(), name: name.trim() },
      });

      if (apiError) {
        setError(apiErrorMessage(apiError));
        return;
      }

      setSlug("");
      setName("");
      // Refresh inside a transition so React can keep the current UI responsive
      // while the Server Component re-renders with the new row.
      startTransition(() => router.refresh());
    } catch {
      setError("No pudimos contactar al servidor. Revisá que la API esté corriendo.");
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="rounded-[var(--radius-panel)] border border-border bg-surface p-5"
      noValidate
    >
      <h2 className="text-sm font-semibold">Nuevo consultorio</h2>

      <div className="mt-4 grid gap-4 sm:grid-cols-2">
        <div>
          <label htmlFor={nameId} className="block text-xs font-medium text-ink-muted">
            Nombre
          </label>
          <input
            id={nameId}
            name="name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            required
            maxLength={120}
            autoComplete="organization"
            placeholder="Consultorio Sur"
            className="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-brand"
          />
        </div>

        <div>
          <label htmlFor={slugId} className="block text-xs font-medium text-ink-muted">
            Subdominio
          </label>
          <input
            id={slugId}
            name="slug"
            value={slug}
            onChange={(event) => setSlug(event.target.value)}
            required
            minLength={3}
            maxLength={63}
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            aria-describedby={slugHintId}
            aria-invalid={!slugLooksValid}
            placeholder="consultorio-sur"
            className="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 font-mono text-sm outline-none focus:border-brand"
          />
          <p
            id={slugHintId}
            className={`mt-1 text-xs ${slugLooksValid ? "text-ink-muted" : "text-danger"}`}
          >
            Minúsculas, números y guiones simples.
          </p>
        </div>
      </div>

      {error ? (
        <p role="alert" className="mt-4 text-sm text-danger">
          {error}
        </p>
      ) : null}

      <button
        type="submit"
        disabled={busy}
        className="mt-5 inline-flex items-center rounded-md bg-brand px-4 py-2 text-sm font-medium text-brand-ink disabled:opacity-60"
      >
        {busy ? "Creando…" : "Crear consultorio"}
      </button>
    </form>
  );
}
