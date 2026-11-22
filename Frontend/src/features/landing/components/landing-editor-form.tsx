"use client";

import { useId, useState } from "react";

import {
  LANDING_LIMITS,
  type LandingDraft,
  type PracticeLanding,
} from "@/entities/landing/model";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { inputClass } from "@/shared/ui/field";

const textareaClass = `${inputClass} min-h-[6rem] resize-y`;

export function LandingEditorForm({
  initial,
  onSaved,
}: {
  initial: PracticeLanding;
  onSaved: (saved: PracticeLanding) => void;
}) {
  const [displayName, setDisplayName] = useState(initial.displayName);
  const [tagline, setTagline] = useState(initial.tagline);
  const [about, setAbout] = useState(initial.about);
  const [address, setAddress] = useState(initial.address);
  const [phone, setPhone] = useState(initial.phone);
  const [email, setEmail] = useState(initial.email);
  const [schedule, setSchedule] = useState(initial.schedule);
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const writesLocked = useWritesLocked();

  const prefix = useId();

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (writesLocked) return;
    setError(null);

    const trimmedName = displayName.trim();
    if (trimmedName === "") {
      setError("El nombre es obligatorio.");
      return;
    }

    const body: LandingDraft = {
      displayName: trimmedName,
      tagline: tagline.trim(),
      about: about.trim(),
      address: address.trim(),
      phone: phone.trim(),
      email: email.trim(),
      schedule: schedule.trim(),
    };

    setIsSubmitting(true);
    try {
      const result = await browserApi.PUT("/api/v1/practice/landing", { body });
      if (result.error || !result.data) {
        setError(apiErrorMessage(result.error));
        return;
      }
      onSaved(result.data);
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
      <div className="grid gap-4">
        <TextField
          id={`${prefix}-displayName`}
          label="Nombre público"
          hint="Es el título de la landing."
          value={displayName}
          onChange={setDisplayName}
          required
          maxLength={LANDING_LIMITS.displayName}
        />

        <TextField
          id={`${prefix}-tagline`}
          label="Frase"
          hint="Una línea bajo el nombre."
          value={tagline}
          onChange={setTagline}
          maxLength={LANDING_LIMITS.tagline}
        />

        <div>
          <label htmlFor={`${prefix}-about`} className="block text-xs font-medium text-ink-muted">
            Sobre el consultorio
          </label>
          <textarea
            id={`${prefix}-about`}
            value={about}
            onChange={(event) => setAbout(event.target.value)}
            maxLength={LANDING_LIMITS.about}
            rows={5}
            className={textareaClass}
          />
        </div>

        <TextField
          id={`${prefix}-address`}
          label="Dirección"
          value={address}
          onChange={setAddress}
          maxLength={LANDING_LIMITS.address}
        />

        <div className="grid gap-4 sm:grid-cols-2">
          <TextField
            id={`${prefix}-phone`}
            label="Teléfono"
            value={phone}
            onChange={setPhone}
            maxLength={LANDING_LIMITS.phone}
            inputMode="tel"
          />
          <TextField
            id={`${prefix}-email`}
            label="Email de contacto"
            type="email"
            value={email}
            onChange={setEmail}
            maxLength={LANDING_LIMITS.email}
          />
        </div>

        <div>
          <label htmlFor={`${prefix}-schedule`} className="block text-xs font-medium text-ink-muted">
            Horarios
          </label>
          <textarea
            id={`${prefix}-schedule`}
            value={schedule}
            onChange={(event) => setSchedule(event.target.value)}
            maxLength={LANDING_LIMITS.schedule}
            rows={3}
            placeholder="Lunes a viernes 9 a 18 hs"
            className={textareaClass}
          />
        </div>
      </div>

      {error ? (
        <p role="alert" className="mt-4 text-sm text-danger">
          {error}
        </p>
      ) : null}

      <button
        type="submit"
        disabled={isSubmitting || writesLocked}
        className="mt-5 inline-flex items-center rounded-md bg-brand px-4 py-2 text-sm font-medium text-brand-ink disabled:opacity-60"
      >
        {isSubmitting ? "Guardando…" : "Guardar landing"}
      </button>
    </form>
  );
}

function TextField({
  id,
  label,
  hint,
  value,
  onChange,
  required,
  maxLength,
  type = "text",
  inputMode,
}: {
  id: string;
  label: string;
  hint?: string;
  value: string;
  onChange: (value: string) => void;
  required?: boolean;
  maxLength: number;
  type?: string;
  inputMode?: "tel" | "email" | "text";
}) {
  return (
    <div>
      <label htmlFor={id} className="block text-xs font-medium text-ink-muted">
        {label}
      </label>
      {hint ? <p className="mt-0.5 text-xs text-ink-muted">{hint}</p> : null}
      <input
        id={id}
        type={type}
        inputMode={inputMode}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        required={required}
        maxLength={maxLength}
        className={`${inputClass} mt-1`}
      />
    </div>
  );
}