"use client";

import { useEffect, useId, useState } from "react";

import { invitationPath, roleLabel, ROLES, type Role } from "@/entities/iam/model";
import { apiErrorMessage, browserApi, CONNECTION_ERROR_MESSAGE } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { Button } from "@/shared/ui/button";
import { Field, inputClass } from "@/shared/ui/field";
import { FormModal } from "@/shared/ui/form-modal";
import { Modal } from "@/shared/ui/modal";

type IssuedLink = { email: string; url: string };

/**
 * Invite someone in a popup (E8, T70). After the API answers, the one-time link stays in the
 * same popup with a copy button — the token is not shown again.
 */
export function InviteFormModal({
  open,
  viewerRole,
  practiceSlug,
  onClose,
  onIssued,
}: {
  open: boolean;
  viewerRole: string;
  practiceSlug: string | null;
  onClose: () => void;
  onIssued: () => void;
}) {
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<Role>("professional");
  const [error, setError] = useState<string | null>(null);
  const [issued, setIssued] = useState<IssuedLink | null>(null);
  const [busy, setBusy] = useState(false);
  const writesLocked = useWritesLocked();

  const prefix = useId();

  useEffect(() => {
    if (!open) return;
    setEmail("");
    setRole("professional");
    setError(null);
    setIssued(null);
  }, [open]);

  async function submit() {
    if (writesLocked || issued !== null) return;
    setError(null);

    if (email.trim() === "") {
      setError("Escribí el email de la persona que vas a invitar.");
      return;
    }

    setBusy(true);
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
      onIssued();
    } catch {
      setError(CONNECTION_ERROR_MESSAGE);
    } finally {
      setBusy(false);
    }
  }

  if (issued) {
    return (
      <Modal
        open={open}
        title="Enlace de invitación"
        description={`Para ${issued.email}. Aveniq no envía emails: copiá el enlace y mandáselo por el medio que prefieras.`}
        onClose={onClose}
        footer={(requestClose) => (
          <Button variant="primary" onClick={requestClose}>
            Listo
          </Button>
        )}
      >
        <IssuedLinkPanel email={issued.email} url={issued.url} />
      </Modal>
    );
  }

  return (
    <FormModal
      open={open}
      title="Invitar a una persona"
      description="Generá un enlace de un solo uso, válido por siete días."
      submitLabel="Generar enlace"
      busy={busy}
      error={error}
      onClose={onClose}
      onSubmit={submit}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="Email" htmlFor={`${prefix}-email`}>
          <input
            id={`${prefix}-email`}
            type="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            maxLength={320}
            placeholder="persona@consultorio.test"
            disabled={writesLocked}
            className={inputClass}
          />
        </Field>

        <Field label="Rol" htmlFor={`${prefix}-role`}>
          <select
            id={`${prefix}-role`}
            value={role}
            onChange={(event) => setRole(event.target.value as Role)}
            disabled={writesLocked}
            className={inputClass}
          >
            {ROLES.map((code) => (
              <option key={code} value={code} disabled={code === "owner" && viewerRole !== "owner"}>
                {roleLabel(code)}
              </option>
            ))}
          </select>
        </Field>
      </div>
    </FormModal>
  );
}

function IssuedLinkPanel({ email, url }: { email: string; url: string }) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  }

  return (
    <div role="status" className="grid gap-3">
      <p className="text-xs text-ink-muted">
        Se muestra una sola vez. Si lo perdés, retirá la invitación y generá otra.
      </p>
      <h3 className="sr-only">Enlace para {email}</h3>
      <div className="flex flex-wrap items-center gap-2">
        <input
          readOnly
          aria-label="Enlace de invitación"
          value={url}
          onFocus={(event) => event.currentTarget.select()}
          className="min-w-0 flex-1 rounded-md border border-border bg-surface px-3 py-2 font-mono text-xs"
        />
        <Button type="button" size="sm" onClick={() => void copy()}>
          {copied ? "Copiado" : "Copiar enlace"}
        </Button>
      </div>
    </div>
  );
}