"use client";

import { useEffect, useId, useState } from "react";

import { mayGrant, roleLabel, ROLES, type Role, type User } from "@/entities/iam/model";
import { apiErrorMessage, browserApi, CONNECTION_ERROR_MESSAGE } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { Field, inputClass } from "@/shared/ui/field";
import { FormModal } from "@/shared/ui/form-modal";

/** Change a account's role in a popup (E8, T70). Access on/off stays in the table. */
export function UserEditModal({
  open,
  user,
  viewerID,
  viewerRole,
  onClose,
  onSaved,
}: {
  open: boolean;
  user: User | null;
  viewerID: string;
  viewerRole: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [role, setRole] = useState<Role>("professional");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const writesLocked = useWritesLocked();
  const prefix = useId();

  useEffect(() => {
    if (!open || user === null) return;
    setRole(user.role);
    setError(null);
  }, [open, user]);

  if (user === null) return null;

  const isSelf = user.id === viewerID;
  const locked =
    writesLocked || isSelf || !mayGrant(viewerRole, user.role) || !mayGrant(viewerRole, role);

  async function submit() {
    if (locked) return;
    if (role === user!.role) {
      onClose();
      return;
    }

    setBusy(true);
    setError(null);
    try {
      const { error: apiError, response } = await browserApi.PUT("/api/v1/users/{id}/role", {
        params: { path: { id: user!.id } },
        body: { role },
      });
      if (!response.ok) {
        setError(apiErrorMessage(apiError));
        return;
      }
      onSaved();
      onClose();
    } catch {
      setError(CONNECTION_ERROR_MESSAGE);
    } finally {
      setBusy(false);
    }
  }

  return (
    <FormModal
      open={open}
      title="Editar cuenta"
      description={user.fullName}
      submitLabel="Guardar cambios"
      busy={busy}
      error={error}
      onClose={onClose}
      onSubmit={submit}
    >
      <Field label="Email" htmlFor={`${prefix}-email`}>
        <input id={`${prefix}-email`} value={user.email} readOnly disabled className={inputClass} />
      </Field>

      <Field label="Rol" htmlFor={`${prefix}-role`}>
        <select
          id={`${prefix}-role`}
          value={role}
          onChange={(event) => setRole(event.target.value as Role)}
          disabled={locked || busy}
          className={inputClass}
        >
          {ROLES.map((code) => (
            <option
              key={code}
              value={code}
              disabled={code === "owner" && viewerRole !== "owner"}
            >
              {roleLabel(code)}
            </option>
          ))}
        </select>
      </Field>
    </FormModal>
  );
}