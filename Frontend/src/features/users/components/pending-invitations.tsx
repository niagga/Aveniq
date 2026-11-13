"use client";

import { useState } from "react";

import { formatDate, roleLabel, type Invitation } from "@/entities/iam/model";
import { apiErrorMessage, browserApi, CONNECTION_ERROR_MESSAGE } from "@/shared/api/client";
import { Notice } from "@/shared/ui/notice";

/**
 * Invitations not yet accepted. Retiring one stops its link at once; an accepted one
 * is no longer listed here, because it is an account now.
 */
export function PendingInvitations({
  invitations,
  onChanged,
}: {
  invitations: Invitation[];
  onChanged: () => void;
}) {
  if (invitations.length === 0) {
    return (
      <Notice tone="info" title="No hay invitaciones pendientes">
        Las invitaciones aceptadas pasan a la lista de cuentas.
      </Notice>
    );
  }

  return (
    <div className="overflow-hidden rounded-[var(--radius-panel)] border border-border bg-surface">
      <table className="w-full text-left text-sm">
        <thead className="bg-surface-muted text-xs uppercase tracking-wide text-ink-muted">
          <tr>
            <th className="px-4 py-3 font-medium">Email</th>
            <th className="px-4 py-3 font-medium">Rol</th>
            <th className="px-4 py-3 font-medium">Vence</th>
            <th className="px-4 py-3 font-medium">Acciones</th>
          </tr>
        </thead>
        <tbody>
          {invitations.map((invitation) => (
            <InvitationRow key={invitation.id} invitation={invitation} onChanged={onChanged} />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function InvitationRow({
  invitation,
  onChanged,
}: {
  invitation: Invitation;
  onChanged: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function revoke() {
    setBusy(true);
    setError(null);
    try {
      const { error: apiError, response } = await browserApi.DELETE("/api/v1/invitations/{id}", {
        params: { path: { id: invitation.id } },
      });
      if (!response.ok) {
        setError(apiErrorMessage(apiError));
        return;
      }
      onChanged();
    } catch {
      setError(CONNECTION_ERROR_MESSAGE);
    } finally {
      setBusy(false);
    }
  }

  return (
    <tr className="border-t border-border align-top">
      <td className="px-4 py-3 font-medium">{invitation.email}</td>
      <td className="px-4 py-3 text-ink-muted">{roleLabel(invitation.role)}</td>
      <td className="px-4 py-3 text-ink-muted">{formatDate(invitation.expiresAt)}</td>
      <td className="px-4 py-3">
        <button
          type="button"
          onClick={() => void revoke()}
          disabled={busy}
          className="rounded-md border border-border px-2 py-1 text-xs font-medium disabled:opacity-60"
        >
          {busy ? "Retirando…" : "Retirar invitación"}
        </button>
        {error ? (
          <p role="alert" className="mt-2 text-xs text-danger">
            {error}
          </p>
        ) : null}
      </td>
    </tr>
  );
}
