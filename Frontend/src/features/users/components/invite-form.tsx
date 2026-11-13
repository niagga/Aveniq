"use client";

import { useId, useState } from "react";

import { invitationPath, roleLabel, ROLES, type Role } from "@/entities/iam/model";
import { apiErrorMessage, browserApi, CONNECTION_ERROR_MESSAGE } from "@/shared/api/client";
import { Field, inputClass } from "@/shared/ui/field";

type IssuedLink = { email: string; url: string };

/**
 * Offers an account: the owner or admin enters the email and role, and the API answers
 * with the link. The token inside it is the only copy, so the link is shown here once
 * and the screen says so. Losing it means revoking the invitation and offering another.
 */
export function InviteForm({
  viewerRole,
  practiceSlug,
  onIssued,
}: {
  viewerRole: string;
  practiceSlug: string | null;
  onIssued: () => void;
}) {
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<Role>("professional");
  const [error, setError] = useState<string | null>(null);
  const [issued, setIssued] = useState<IssuedLink | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const prefix = useId();

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setIssued(null);

    if (email.trim() === "") {
      setError("Escribí el email de la persona que vas a invitar.");
      return;
    }

    setIsSubmitting(true);
    try {
      const { data, error: apiError, response } = await browserApi.POST("/api/v1/invitations", {
        body: { email: email.trim(), role },
      });

      if (!response.ok || !data) {
        setError(apiErrorMessage(apiError));
        return;
      }

      setIssued({
        email: data.invitation.email,
        url: `${window.location.origin}${invitationPath(data.token, practiceSlug)}`,
      });
      setEmail("");
      onIssued();
    } catch {
      setError(CONNECTION_ERROR_MESSAGE);
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <section className="rounded-[var(--radius-panel)] border border-border bg-surface p-5">
      <h2 className="text-sm font-semibold">Invitar a una persona</h2>
      <p className="mt-1 text-sm text-ink-muted">
        Generá un enlace de un solo uso, válido por siete días. Aveniq no envía emails: copiá el
        enlace y mandáselo por el medio que prefieras.
      </p>

      <form onSubmit={handleSubmit} noValidate className="mt-4 grid gap-4 sm:grid-cols-[2fr_1fr_auto] sm:items-end">
        <Field label="Email" htmlFor={`${prefix}-email`}>
          <input
            id={`${prefix}-email`}
            type="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            maxLength={320}
            placeholder="persona@consultorio.test"
            className={inputClass}
          />
        </Field>

        <Field label="Rol" htmlFor={`${prefix}-role`}>
          <select
            id={`${prefix}-role`}
            value={role}
            onChange={(event) => setRole(event.target.value as Role)}
            className={inputClass}
          >
            {ROLES.map((code) => (
              <option key={code} value={code} disabled={code === "owner" && viewerRole !== "owner"}>
                {roleLabel(code)}
              </option>
            ))}
          </select>
        </Field>

        <button
          type="submit"
          disabled={isSubmitting}
          className="inline-flex items-center justify-center rounded-md bg-brand px-4 py-2 text-sm font-medium text-brand-ink disabled:opacity-60"
        >
          {isSubmitting ? "Generando…" : "Generar enlace"}
        </button>
      </form>

      {error ? (
        <p role="alert" className="mt-4 text-sm text-danger">
          {error}
        </p>
      ) : null}

      {issued ? <IssuedLinkPanel key={issued.url} email={issued.email} url={issued.url} /> : null}
    </section>
  );
}

function IssuedLinkPanel({ email, url }: { email: string; url: string }) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
    } catch {
      // Clipboard access can be refused on a plain-http origin. The field below is
      // selected for manual copy, so the person is never left without the link.
      setCopied(false);
    }
  }

  return (
    <div role="status" className="mt-5 rounded-md border border-brand/30 bg-brand/5 p-4">
      <p className="text-sm font-medium">Enlace para {email}</p>
      <p className="mt-1 text-xs text-ink-muted">
        Se muestra una sola vez. Si lo perdés, retirá la invitación y generá otra.
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <input
          readOnly
          aria-label="Enlace de invitación"
          value={url}
          onFocus={(event) => event.currentTarget.select()}
          className="min-w-0 flex-1 rounded-md border border-border bg-surface px-3 py-2 font-mono text-xs outline-none"
        />
        <button
          type="button"
          onClick={() => void copy()}
          className="rounded-md bg-brand px-3 py-2 text-sm font-medium text-brand-ink"
        >
          {copied ? "Copiado" : "Copiar enlace"}
        </button>
      </div>
    </div>
  );
}
