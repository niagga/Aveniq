"use client";

import { useState } from "react";

import {
  mayGrant,
  roleLabel,
  ROLES,
  type Role,
  type User,
} from "@/entities/iam/model";
import { apiErrorMessage, browserApi, CONNECTION_ERROR_MESSAGE } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { ConfirmDialog } from "@/shared/ui/confirm-dialog";
import { Notice } from "@/shared/ui/notice";

/**
 * The accounts of the practice, with the two changes an administrator makes: the role
 * and the access (deactivate or reactivate).
 *
 * A row the viewer may not touch is shown disabled, with the reason in its title. Nobody
 * changes their own access, and only an owner touches the owner role (AuthorizeChange).
 */
export function UserTable({
  users,
  viewerID,
  viewerRole,
  onChanged,
}: {
  users: User[];
  viewerID: string;
  viewerRole: string;
  onChanged: () => void;
}) {
  if (users.length === 0) {
    return (
      <Notice tone="info" title="No hay cuentas todavía">
        Las cuentas nacen al aceptar una invitación.
      </Notice>
    );
  }

  return (
    <div className="overflow-x-auto rounded-[var(--radius-panel)] border border-border bg-surface">
      <table className="w-full text-left text-sm">
        <thead className="bg-surface-muted text-xs uppercase tracking-wide text-ink-muted">
          <tr>
            <th scope="col" className="px-4 py-3 font-medium">Cuenta</th>
            <th scope="col" className="px-4 py-3 font-medium">Email</th>
            <th scope="col" className="px-4 py-3 font-medium">Rol</th>
            <th scope="col" className="px-4 py-3 font-medium">Estado</th>
            <th scope="col" className="px-4 py-3 font-medium">Acciones</th>
          </tr>
        </thead>
        <tbody>
          {users.map((user) => (
            <UserRow
              key={`${user.id}:${user.role}:${user.isActive}`}
              user={user}
              viewerID={viewerID}
              viewerRole={viewerRole}
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
  onChanged,
}: {
  user: User;
  viewerID: string;
  viewerRole: string;
  onChanged: () => void;
}) {
  const [role, setRole] = useState<Role>(user.role);
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

  async function saveRole() {
    setBusy(true);
    setError(null);
    try {
      const { error: apiError, response } = await browserApi.PUT("/api/v1/users/{id}/role", {
        params: { path: { id: user.id } },
        body: { role },
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

  const roleChanged = role !== user.role;

  return (
    <tr className="border-t border-border align-top">
      <td className="px-4 py-3 font-medium">
        {user.fullName}
        {isSelf ? <span className="ml-2 text-xs font-normal text-ink-muted">(vos)</span> : null}
      </td>
      <td className="px-4 py-3 text-ink-muted">{user.email}</td>
      <td className="px-4 py-3">
        <div className="flex items-center gap-2">
          <select
            aria-label={`Rol de ${user.fullName}`}
            value={role}
            disabled={lockReason !== null || busy}
            title={lockReason ?? undefined}
            onChange={(event) => setRole(event.target.value as Role)}
            className="rounded-md border border-border bg-surface px-2 py-1 text-sm focus:border-brand disabled:opacity-60"
          >
            {ROLES.map((code) => (
              <option key={code} value={code} disabled={code === "owner" && viewerRole !== "owner"}>
                {roleLabel(code)}
              </option>
            ))}
          </select>
          {roleChanged ? (
            <button
              type="button"
              onClick={() => void saveRole()}
              disabled={busy}
              className="rounded-md bg-brand px-2 py-1 text-xs font-medium text-brand-ink disabled:opacity-60"
            >
              Guardar rol
            </button>
          ) : null}
        </div>
      </td>
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
      <td className="px-4 py-3">
        <button
          type="button"
          onClick={() => {
            if (user.isActive) setConfirmingDeactivate(true);
            else void toggleAccess();
          }}
          disabled={lockReason !== null || busy}
          title={lockReason ?? undefined}
          className="rounded-control border border-border px-2 py-1 text-xs font-medium disabled:opacity-60"
        >
          {user.isActive ? "Dar de baja" : "Reactivar"}
        </button>
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
