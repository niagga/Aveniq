"use client";

import { useId, useState } from "react";

import type { Insurer, InsurerProfile } from "@/entities/insurer/model";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { inputClass } from "@/shared/ui/field";

/**
 * Create and edit share this form: the API takes the same profile for both. The tax id
 * is optional until the practice knows it; settlements need it later (T55), so the form
 * says so instead of refusing the mutual today.
 */
export function InsurerForm({
  insurer,
  onSaved,
  onCancel,
}: {
  insurer?: Insurer | null;
  onSaved: () => void;
  onCancel: () => void;
}) {
  const [name, setName] = useState(insurer?.name ?? "");
  const [taxId, setTaxId] = useState(insurer?.taxId ?? "");
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const writesLocked = useWritesLocked();

  const prefix = useId();
  const isEditing = Boolean(insurer);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (writesLocked) return;
    setError(null);

    if (name.trim() === "") {
      setError("El nombre es obligatorio.");
      return;
    }

    const body: InsurerProfile = { name: name.trim(), taxId: taxId.trim() };

    setIsSubmitting(true);
    try {
      const result = insurer
        ? await browserApi.PATCH("/api/v1/insurers/{id}", {
            params: { path: { id: insurer.id } },
            body,
          })
        : await browserApi.POST("/api/v1/insurers", { body });

      if (result.error || !result.data) {
        // The API names what is wrong: a name the practice already has, a tax id that is
        // not 11 digits. That is what the person needs to read.
        setError(apiErrorMessage(result.error));
        return;
      }

      onSaved();
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
      <h2 className="text-sm font-semibold">{isEditing ? "Editar mutual" : "Nueva mutual"}</h2>

      <div className="mt-4 grid gap-4 sm:grid-cols-2">
        <div>
          <label htmlFor={`${prefix}-name`} className="block text-xs font-medium text-ink-muted">
            Nombre
          </label>
          <input
            id={`${prefix}-name`}
            value={name}
            onChange={(event) => setName(event.target.value)}
            required
            maxLength={120}
            placeholder="OSDE"
            className={inputClass}
          />
        </div>

        <div>
          <label htmlFor={`${prefix}-taxId`} className="block text-xs font-medium text-ink-muted">
            CUIT (opcional)
          </label>
          <input
            id={`${prefix}-taxId`}
            value={taxId}
            onChange={(event) => setTaxId(event.target.value)}
            maxLength={20}
            placeholder="30-12345678-9"
            className={inputClass}
          />
        </div>
      </div>

      {error ? (
        <p role="alert" className="mt-4 text-sm text-danger">
          {error}
        </p>
      ) : null}

      <div className="mt-5 flex items-center gap-3">
        <button
          type="submit"
          disabled={isSubmitting || writesLocked}
          className="inline-flex items-center rounded-md bg-brand px-4 py-2 text-sm font-medium text-brand-ink disabled:opacity-60"
        >
          {isSubmitting ? "Guardando…" : isEditing ? "Guardar cambios" : "Crear mutual"}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="rounded-md border border-border px-3 py-2 text-sm text-ink-muted"
        >
          Cancelar
        </button>
      </div>
    </form>
  );
}
