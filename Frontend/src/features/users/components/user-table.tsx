"use client";

import { useState } from "react";

import { mayGrant, roleLabel, type User } from "@/entities/iam/model";
import { apiErrorMessage, browserApi, CONNECTION_ERROR_MESSAGE } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { Button } from "@/shared/ui/button";
import { ConfirmDialog } from "@/shared/ui/confirm-dialog";
import { Notice } from "@/shared/ui/notice";

/**
 * Accounts of the practice. Role changes open in a popup (T70); deactivate stays here.
 */
export function UserTable({
  users,
  viewerID,
  viewerRole,
  onEdit,
  onChanged,
}: {
  users: User[];
  viewerID: string;
  viewerRole: string;
  onEdit: (user: User) => void;
  onChanged: () => void;
}) {
  if (users.length === 0) {
    return (
      <Notice tone="info" title="Ninguna cuenta coincide con los filtros">
        Probá otra búsqueda o limpiá los filtros. Las cuentas nacen al aceptar una invitación.
      </Notice>
    );
  }

  return (
    <div className="overflow-x-auto rounded-[var(--radius-panel)] border border-border bg-surface">
      <table className="w-full text-left text-sm">
        <thead className="bg-surface-muted text-xs uppercase tracking-wide text-ink-muted">
          <tr>
            <th scope="col" className="px-4 py-3 font-medium">
              Cuenta
            </th>
            <th scope="col" className="px-4 py-3 font-medium">
              Email
            </th>
            <th scope="col" className="px-4 py-3 font-medium">
              Rol
            </th>
            <th scope="col" className="px-4 py-3 font-medium">
              Estado
            </th>
            <th scope="col" className="px-4 py-3 font-medium">
              <span className="sr-only">Acciones</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {users.map((user) => (
            <UserRow
              key={`${user.id}:${user.role}:${user.isActive}`}
              user={user}
              viewerID={viewerID}
              viewerRole={viewerRole}
              onEdit={onEdit}
              onChanged={onChanged}
            />
          ))}
        </tbody>
      </table>
      <p className="border-t border-border px-4 py-2 text-xs text-ink-muted">
        {users.length} cuenta{users.length === 1 ? "" : "s"}
      </p>
    </div>
  );
}

function UserRow({
  user,
  viewerID,
  viewerRole,
  onEdit,
  onChanged,
}: {
  user: User;
  viewerID: string;
  viewerRole: string;
  onEdit: (user: User) => void;
  onChanged: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [confirmingDeactivate, setConfirmingDeactivate] = useState(false);
  const writesLocked = useWritesLocked();

  const isSelf = user.id === viewerID;
  const lockReason = writesLocked
    ? "La prueba de este consultorio venció."
    : isSelf
      ? "No podés cambiar tu propio acceso."
      : !mayGrant(viewerRole, user.role)
        ? "Solo el titular cambia cuentas con rol titular."
        : null;

  async function toggleAccess() {
    setBusy(true);
    setError(null);
    try {
      const path = { params: { path: { id: user.id } } };
      const result = user.isActive
        ? await browserApi.POST("/api/v1/users/{id}/deactivate", path)
        : await browserApi.POST("/api/v1/users/{id}/activate", path);
      if (!result.response.ok) {
        setError(apiErrorMessage(result.error));
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
      <td className="px-4 py-3 font-medium">
        {user.fullName}
        {isSelf ? <span className="ml-2 text-xs font-normal text-ink-muted">(vos)</span> : null}
      </td>
      <td className="px-4 py-3 text-ink-muted">{user.email}</td>
      <td className="px-4 py-3 text-ink-muted">{roleLabel(user.role)}</td>
      <td className="px-4 py-3">
        <span
          className={`rounded-full border px-2 py-0.5 text-xs ${
            user.isActive
              ? "border-success/30 bg-success/5 text-success"
              : "border-border bg-surface-muted text-ink-muted"
          }`}
        >
          {user.isActive ? "Activa" : "Dada de baja"}
        </span>
      </td>
      <td className="px-4 py-3 text-right">
        <div className="flex justify-end gap-2">
          <Button
            variant="secondary"
            size="sm"
            onClick={() => onEdit(user)}
            disabled={lockReason !== null}
            title={lockReason ?? undefined}
          >
            Editar
          </Button>
          <Button
            variant="secondary"
            size="sm"
            onClick={() => {
              if (user.isActive) setConfirmingDeactivate(true);
              else void toggleAccess();
            }}
            disabled={lockReason !== null || busy}
            title={lockReason ?? undefined}
          >
            {user.isActive ? "Dar de baja" : "Reactivar"}
          </Button>
        </div>
        <ConfirmDialog
          open={confirmingDeactivate}
          title={`¿Dar de baja a ${user.fullName}?`}
          description="Deja de poder entrar al consultorio en el acto. La cuenta y su historial de trabajo se conservan: se puede reactivar cuando haga falta."
          confirmLabel="Dar de baja"
          tone="danger"
          busy={busy}
          onCancel={() => setConfirmingDeactivate(false)}
          onConfirm={() => {
            setConfirmingDeactivate(false);
            void toggleAccess();
          }}
        />
        {error ? (
          <p role="alert" className="mt-2 text-xs text-danger">
            {error}
          </p>
        ) : null}
      </td>
    </tr>
  );
}