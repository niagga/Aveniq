"use client";

import Link from "next/link";
import { useEffect, useId, useState } from "react";

import {
  formatDate,
  PASSWORD_MIN_LENGTH,
  roleLabel,
} from "@/entities/iam/model";
import { apiErrorMessage, browserApi, CONNECTION_ERROR_MESSAGE } from "@/shared/api/client";
import { Field, inputClass } from "@/shared/ui/field";
import { Notice } from "@/shared/ui/notice";

type Preview = { email: string; role: string; expiresAt: string };
type Accepted = { email: string };

const UNUSABLE_MESSAGE =
  "Este enlace ya no es válido: ya se usó, venció o fue retirado. Pedile al titular del consultorio que genere uno nuevo.";

/**
 * The invitee's page (T35 acceptance, T37 screen). It is public: the link carries the
 * only credential, and the preview tells the invitee what they are joining before they
 * choose a password.
 *
 * The API answers 410 with one message for every unusable link, and so does the screen.
 */
export function AcceptInvitationForm({
  token,
  practiceSlug,
}: {
  token: string;
  practiceSlug: string | null;
}) {
  const [preview, setPreview] = useState<Preview | null>(null);
  const [unusable, setUnusable] = useState(token === "");
  const [loadError, setLoadError] = useState<string | null>(null);
  const [accepted, setAccepted] = useState<Accepted | null>(null);

  const [fullName, setFullName] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const prefix = useId();

  useEffect(() => {
    if (token === "") return;
    let active = true;
    void (async () => {
      try {
        const { data, error: apiError, response } = await browserApi.GET(
          "/api/v1/invitations/preview",
          { params: { query: { token } } },
        );
        if (!active) return;

        if (response.status === 410) {
          setUnusable(true);
        } else if (!response.ok || !data) {
          setLoadError(apiErrorMessage(apiError));
        } else {
          setPreview(data);
        }
      } catch {
        if (active) setLoadError(CONNECTION_ERROR_MESSAGE);
      }
    })();
    return () => {
      active = false;
    };
  }, [token]);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);

    if (fullName.trim() === "") {
      setError("Escribí tu nombre y apellido.");
      return;
    }
    if (password.length < PASSWORD_MIN_LENGTH) {
      setError(`La contraseña necesita al menos ${PASSWORD_MIN_LENGTH} caracteres.`);
      return;
    }
    if (password !== confirmation) {
      setError("Las contraseñas no coinciden.");
      return;
    }

    setIsSubmitting(true);
    try {
      const { data, error: apiError, response } = await browserApi.POST(
        "/api/v1/invitations/accept",
        { body: { token, fullName: fullName.trim(), password } },
      );

      if (response.status === 410) {
        setUnusable(true);
        return;
      }
      if (!response.ok || !data) {
        // A 422 names the rule that failed. The link survives it, so the invitee can retry.
        setError(apiErrorMessage(apiError));
        return;
      }

      setPassword("");
      setConfirmation("");
      setAccepted({ email: data.email });
    } catch {
      setError(CONNECTION_ERROR_MESSAGE);
    } finally {
      setIsSubmitting(false);
    }
  }

  if (accepted !== null) {
    return (
      <Notice tone="info" title="Tu cuenta quedó creada">
        <p>
          Iniciá sesión con el email <span className="font-medium">{accepted.email}</span>
          {practiceSlug !== null ? (
            <>
              {" "}en el consultorio <span className="font-mono">{practiceSlug}</span>
            </>
          ) : null}
          .
        </p>
        <Link
          href={{ pathname: "/login", query: practiceSlug !== null ? { consultorio: practiceSlug } : undefined }}
          className="mt-3 inline-block text-sm font-medium text-brand underline"
        >
          Ir al inicio de sesión
        </Link>
      </Notice>
    );
  }

  if (unusable) {
    return (
      <Notice tone="danger" title="Enlace no válido">
        {UNUSABLE_MESSAGE}
      </Notice>
    );
  }

  if (loadError !== null) {
    return (
      <Notice tone="danger" title="No pudimos leer la invitación">
        {loadError}
      </Notice>
    );
  }

  if (preview === null) {
    return (
      <p role="status" className="text-sm text-ink-muted">
        Verificando la invitación…
      </p>
    );
  }

  return (
    <form
      onSubmit={handleSubmit}
      noValidate
      className="rounded-[var(--radius-panel)] border border-border bg-surface p-6"
    >
      <h1 className="text-lg font-semibold">Crear tu cuenta</h1>
      <p className="mt-1 text-sm text-ink-muted">
        Te invitaron como <span className="font-medium">{roleLabel(preview.role)}</span> con el
        email <span className="font-medium">{preview.email}</span>. El enlace vence el{" "}
        {formatDate(preview.expiresAt)}.
      </p>

      <div className="mt-5 grid gap-4">
        <Field label="Nombre y apellido" htmlFor={`${prefix}-name`}>
          <input
            id={`${prefix}-name`}
            value={fullName}
            onChange={(event) => setFullName(event.target.value)}
            maxLength={200}
            autoComplete="name"
            className={inputClass}
          />
        </Field>

        <Field
          label="Contraseña"
          htmlFor={`${prefix}-password`}
          hint={`Al menos ${PASSWORD_MIN_LENGTH} caracteres. La elegís vos.`}
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
        {isSubmitting ? "Creando cuenta…" : "Crear cuenta"}
      </button>
    </form>
  );
}
