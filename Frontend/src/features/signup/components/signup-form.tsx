"use client";

import Link from "next/link";
import { useId, useState } from "react";

import { formatDate, PASSWORD_MIN_LENGTH } from "@/entities/iam/model";
import { isValidSlug, SLUG_MAX_LENGTH, suggestSlug } from "@/entities/tenant/slug";
import { apiErrorMessage, browserApi, CONNECTION_ERROR_MESSAGE } from "@/shared/api/client";
import { Field, inputClass } from "@/shared/ui/field";
import { Notice } from "@/shared/ui/notice";

type Created = { name: string; slug: string; trialEndsAt: string; ownerEmail: string };

/**
 * Public registration of a practice in trial (T36, screen T37).
 *
 * The slug follows the practice name until the person edits it by hand, so the common
 * case is one field typed. The API is rate limited; a refusal arrives with its reason.
 */
export function SignupForm() {
  const [practiceName, setPracticeName] = useState("");
  const [slug, setSlug] = useState("");
  const [slugEdited, setSlugEdited] = useState(false);
  const [ownerName, setOwnerName] = useState("");
  const [ownerEmail, setOwnerEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [created, setCreated] = useState<Created | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const prefix = useId();

  function changePracticeName(value: string) {
    setPracticeName(value);
    if (!slugEdited) setSlug(suggestSlug(value));
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);

    if (practiceName.trim() === "") return setError("Escribí el nombre del consultorio.");
    if (!isValidSlug(slug)) {
      return setError(
        `La dirección del consultorio usa de 3 a ${SLUG_MAX_LENGTH} letras minúsculas, números y guiones, sin guiones al principio ni al final.`,
      );
    }
    if (ownerName.trim() === "") return setError("Escribí tu nombre y apellido.");
    if (ownerEmail.trim() === "") return setError("Escribí el email con el que vas a ingresar.");
    if (password.length < PASSWORD_MIN_LENGTH) {
      return setError(`La contraseña necesita al menos ${PASSWORD_MIN_LENGTH} caracteres.`);
    }
    if (password !== confirmation) return setError("Las contraseñas no coinciden.");

    setIsSubmitting(true);
    try {
      const { data, error: apiError, response } = await browserApi.POST("/api/v1/signup", {
        body: {
          practiceName: practiceName.trim(),
          slug,
          ownerName: ownerName.trim(),
          ownerEmail: ownerEmail.trim(),
          password,
        },
      });

      if (!response.ok || !data) {
        setError(apiErrorMessage(apiError));
        return;
      }

      setPassword("");
      setConfirmation("");
      setCreated({
        name: data.tenant.name,
        slug: data.tenant.slug,
        trialEndsAt: data.tenant.trialEndsAt,
        ownerEmail: data.owner.email,
      });
    } catch {
      setError(CONNECTION_ERROR_MESSAGE);
    } finally {
      setIsSubmitting(false);
    }
  }

  if (created !== null) {
    return (
      <Notice tone="info" title={`${created.name} está en prueba`}>
        <p>
          La prueba dura 14 días, hasta el {formatDate(created.trialEndsAt)}. Ingresá con{" "}
          <span className="font-medium">{created.ownerEmail}</span> en el consultorio{" "}
          <span className="font-mono">{created.slug}</span>.
        </p>
        <p className="mt-2 text-xs">
          Todavía no verificamos el email: la cuenta funciona igual.
        </p>
        <Link
          href={`/login?consultorio=${encodeURIComponent(created.slug)}`}
          className="mt-3 inline-block text-sm font-medium text-brand underline"
        >
          Ir al inicio de sesión
        </Link>
      </Notice>
    );
  }

  return (
    <form
      onSubmit={handleSubmit}
      noValidate
      className="rounded-[var(--radius-panel)] border border-border bg-surface p-6"
    >
      <h1 className="text-lg font-semibold">Registrar el consultorio</h1>
      <p className="mt-1 text-sm text-ink-muted">
        Empezás con 14 días de prueba. Vos quedás como titular.
      </p>

      <div className="mt-5 grid gap-4">
        <Field label="Nombre del consultorio" htmlFor={`${prefix}-practice`}>
          <input
            id={`${prefix}-practice`}
            value={practiceName}
            onChange={(event) => changePracticeName(event.target.value)}
            maxLength={120}
            placeholder="Consultorio Sur"
            className={inputClass}
          />
        </Field>

        <Field
          label="Dirección del consultorio"
          htmlFor={`${prefix}-slug`}
          hint="Así lo vas a escribir al ingresar. Se sugiere a partir del nombre."
        >
          <input
            id={`${prefix}-slug`}
            value={slug}
            onChange={(event) => {
              setSlugEdited(true);
              setSlug(event.target.value.toLowerCase());
            }}
            maxLength={SLUG_MAX_LENGTH}
            autoCapitalize="none"
            autoCorrect="off"
            spellCheck={false}
            placeholder="consultorio-sur"
            className={`${inputClass} font-mono`}
          />
        </Field>

        <Field label="Tu nombre y apellido" htmlFor={`${prefix}-owner-name`}>
          <input
            id={`${prefix}-owner-name`}
            value={ownerName}
            onChange={(event) => setOwnerName(event.target.value)}
            maxLength={200}
            autoComplete="name"
            className={inputClass}
          />
        </Field>

        <Field label="Tu email" htmlFor={`${prefix}-owner-email`}>
          <input
            id={`${prefix}-owner-email`}
            type="email"
            value={ownerEmail}
            onChange={(event) => setOwnerEmail(event.target.value)}
            maxLength={254}
            autoComplete="email"
            className={inputClass}
          />
        </Field>

        <Field
          label="Contraseña"
          htmlFor={`${prefix}-password`}
          hint={`Al menos ${PASSWORD_MIN_LENGTH} caracteres.`}
        >
          <input
            id={`${prefix}-password`}
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            maxLength={256}
            autoComplete="new-password"
            className={inputClass}
          />
        </Field>

        <Field label="Repetir contraseña" htmlFor={`${prefix}-confirm`}>
          <input
            id={`${prefix}-confirm`}
            type="password"
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
            maxLength={256}
            autoComplete="new-password"
            className={inputClass}
          />
        </Field>
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
        {isSubmitting ? "Creando consultorio…" : "Crear consultorio"}
      </button>
    </form>
  );
}
