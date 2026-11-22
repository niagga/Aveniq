"use client";

import type { Patient } from "@/entities/patient/model";
import { MODALITY_OPTIONS, type Modality } from "@/entities/schedule/model";
import { inputClass } from "@/shared/ui/field";

export function PatientSelect({
  patients,
  value,
  onChange,
  disabled = false,
}: {
  patients: Patient[];
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  return (
    <label className="block text-xs font-medium text-ink-muted">
      Paciente
      <select
        value={value}
        onChange={(event) => onChange(event.target.value)}
        disabled={disabled}
        className={`${inputClass} mt-1`}
        required
      >
        <option value="">Elegí un paciente</option>
        {patients.map((patient) => (
          <option key={patient.id} value={patient.id}>
            {patient.fullName}
          </option>
        ))}
      </select>
    </label>
  );
}

export function ModalityField({
  value,
  onChange,
  disabled = false,
}: {
  value: Modality | "";
  onChange: (value: Modality | "") => void;
  disabled?: boolean;
}) {
  return (
    <label className="block text-xs font-medium text-ink-muted">
      Modalidad
      <select
        value={value}
        onChange={(event) => onChange(event.target.value as Modality | "")}
        disabled={disabled}
        className={`${inputClass} mt-1`}
        required
      >
        <option value="">Elegí online o presencial</option>
        {MODALITY_OPTIONS.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  );
}

export function DurationField({
  value,
  onChange,
  disabled = false,
}: {
  value: number;
  onChange: (value: number) => void;
  disabled?: boolean;
}) {
  return (
    <label className="block text-xs font-medium text-ink-muted">
      Duración (minutos)
      <input
        type="number"
        min={5}
        max={480}
        value={value}
        onChange={(event) => onChange(Number(event.target.value))}
        disabled={disabled}
        className={`${inputClass} mt-1`}
      />
    </label>
  );
}

export function StartsAtField({
  value,
  onChange,
  disabled = false,
}: {
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  return (
    <label className="block text-xs font-medium text-ink-muted">
      Inicio
      <input
        type="datetime-local"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        disabled={disabled}
        className={`${inputClass} mt-1`}
        required
      />
    </label>
  );
}