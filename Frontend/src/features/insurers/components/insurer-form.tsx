"use client";

import { useEffect, useId, useState } from "react";

import type { Insurer, InsurerProfile } from "@/entities/insurer/model";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { FormModal } from "@/shared/ui/form-modal";
import { inputClass } from "@/shared/ui/field";

/**
 * Create and edit a mutual in a popup (E8, T69). The API takes the same profile for both.
 */
export function InsurerFormModal({
  open,
  insurer,
  onClose,
  onSaved,
}: {
  open: boolean;
  insurer: Insurer | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState("");
  const [taxId, setTaxId] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const writesLocked = useWritesLocked();

  const prefix = useId();
  const isEditing = insurer !== null;

  useEffect(() => {
    if (!open) return;
    setName(insurer?.name ?? "");
    setTaxId(insurer?.taxId ?? "");
    setError(null);
  }, [open, insurer]);

  async function submit() {
    if (writesLocked) return;
    setError(null);

    if (name.trim() === "") {
      setError("El nombre es obligatorio.");
      return;
    }

    const body: InsurerProfile = { name: name.trim(), taxId: taxId.trim() };

    setBusy(true);
    try {
      const result = isEditing
        ? await browserApi.PATCH("/api/v1/insurers/{id}", {
            params: { path: { id: insurer.id } },
            body,
          })
        : await browserApi.POST("/api/v1/insurers", { body });

      if (result.error || !result.data) {
        setError(apiErrorMessage(result.error));
        return;
      }

      onSaved();
      onClose();
    } catch {
      setError("No pudimos contactar al servidor. Revisá que la API esté corriendo.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <FormModal
      open={open}
      title={isEditing ? "Editar mutual" : "Nueva mutual"}
      description="El CUIT es opcional hasta que liquiden a la mutual; después lo van a necesitar."
      submitLabel={isEditing ? "Guardar cambios" : "Crear mutual"}
      busy={busy}
      error={error}
      onClose={onClose}
      onSubmit={submit}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <TextField
          id={`${prefix}-name`}
          label="Nombre"
          value={name}
          onChange={setName}
          required
          maxLength={120}
          placeholder="OSDE"
          disabled={writesLocked}
        />
        <TextField
          id={`${prefix}-taxId`}
          label="CUIT (opcional)"
          value={taxId}
          onChange={setTaxId}
          maxLength={20}
          placeholder="30-12345678-9"
          disabled={writesLocked}
        />
      </div>
    </FormModal>
  );
}

function TextField({
  id,
  label,
  value,
  onChange,
  required,
  maxLength,
  placeholder,
  disabled,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  required?: boolean;
  maxLength: number;
  placeholder?: string;
  disabled?: boolean;
}) {
  return (
    <div>
      <label htmlFor={id} className="block text-xs font-medium text-ink-muted">
        {label}
      </label>
      <input
        id={id}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        required={required}
        maxLength={maxLength}
        placeholder={placeholder}
        disabled={disabled}
        className={inputClass}
      />
    </div>
  );
}