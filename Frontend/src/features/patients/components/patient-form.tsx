"use client";

import { useId, useState } from "react";

import {
  COUNTRIES,
  DOCUMENT_KINDS_BY_COUNTRY,
  documentKindLabel,
  FISCAL_CONDITIONS_BY_COUNTRY,
  fiscalConditionLabel,
  TAX_ID_KINDS_BY_COUNTRY,
  taxIDKindLabel,
  type Patient,
} from "@/entities/patient/model";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import type { components } from "@/shared/api/schema";

type PatientProfile = components["schemas"]["PatientProfileBody"];

type Fields = {
  fullName: string;
  country: string;
  birthDate: string;
  documentKind: string;
  documentNumber: string;
  phone: string;
  email: string;
  emergencyContactName: string;
  emergencyContactPhone: string;
  taxIdKind: string;
  taxIdNumber: string;
  fiscalCondition: string;
  fiscalAddress: string;
  administrativeNotes: string;
};

function fieldsFrom(patient: Patient | null | undefined): Fields {
  return {
    fullName: patient?.fullName ?? "",
    country: patient?.country ?? "AR",
    birthDate: patient?.birthDate ?? "",
    documentKind: patient?.document?.kind ?? "",
    documentNumber: patient?.document?.number ?? "",
    phone: patient?.phone ?? "",
    email: patient?.email ?? "",
    emergencyContactName: patient?.emergencyContactName ?? "",
    emergencyContactPhone: patient?.emergencyContactPhone ?? "",
    taxIdKind: patient?.taxId?.kind ?? "",
    taxIdNumber: patient?.taxId?.number ?? "",
    fiscalCondition: patient?.fiscalCondition ?? "",
    fiscalAddress: patient?.fiscalAddress ?? "",
    administrativeNotes: patient?.administrativeNotes ?? "",
  };
}

/**
 * Create and edit share this component: the API takes the same profile for both, so
 * two forms would be two places for a field to go missing.
 *
 * The document and the tax id are optional pairs. Sending a kind without a number is
 * refused by the server with a 422 that names the field, but stopping it here saves a
 * round trip and says it in the language of the form.
 */
export function PatientForm({
  patient,
  onSaved,
  onCancel,
}: {
  patient?: Patient | null;
  onSaved: (id: string) => void;
  onCancel?: () => void;
}) {
  const [fields, setFields] = useState<Fields>(() => fieldsFrom(patient));
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const prefix = useId();
  const id = (name: keyof Fields) => `${prefix}-${name}`;

  const isEditing = Boolean(patient);
  const documentKinds = DOCUMENT_KINDS_BY_COUNTRY[fields.country] ?? [];
  const taxIDKinds = TAX_ID_KINDS_BY_COUNTRY[fields.country] ?? [];
  const fiscalConditions = FISCAL_CONDITIONS_BY_COUNTRY[fields.country] ?? [];

  function set<K extends keyof Fields>(name: K, value: Fields[K]) {
    setFields((current) => ({ ...current, [name]: value }));
  }

  // Changing the country invalidates a kind the new country does not have.
  function changeCountry(country: string) {
    setFields((current) => ({
      ...current,
      country,
      documentKind: DOCUMENT_KINDS_BY_COUNTRY[country]?.includes(current.documentKind)
        ? current.documentKind
        : "",
      taxIdKind: TAX_ID_KINDS_BY_COUNTRY[country]?.includes(current.taxIdKind) ? current.taxIdKind : "",
      fiscalCondition: FISCAL_CONDITIONS_BY_COUNTRY[country]?.includes(current.fiscalCondition)
        ? current.fiscalCondition
        : "",
    }));
  }

  function validate(): string | null {
    if (fields.fullName.trim() === "") return "El nombre es obligatorio.";
    if (fields.documentKind !== "" && fields.documentNumber.trim() === "") {
      return "Elegiste un tipo de documento sin número.";
    }
    if (fields.documentKind === "" && fields.documentNumber.trim() !== "") {
      return "Cargaste un número de documento sin elegir el tipo.";
    }
    if (fields.taxIdKind !== "" && fields.taxIdNumber.trim() === "") {
      return "Elegiste un identificador fiscal sin número.";
    }
    if (fields.taxIdKind === "" && fields.taxIdNumber.trim() !== "") {
      return "Cargaste un identificador fiscal sin elegir el tipo.";
    }
    return null;
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);

    const problem = validate();
    if (problem !== null) {
      setError(problem);
      return;
    }

    const body: PatientProfile = {
      fullName: fields.fullName.trim(),
      country: fields.country,
      birthDate: fields.birthDate.trim(),
      documentKind: fields.documentKind,
      documentNumber: fields.documentNumber.trim(),
      phone: fields.phone.trim(),
      email: fields.email.trim(),
      emergencyContactName: fields.emergencyContactName.trim(),
      emergencyContactPhone: fields.emergencyContactPhone.trim(),
      taxIdKind: fields.taxIdKind,
      taxIdNumber: fields.taxIdNumber.trim(),
      fiscalCondition: fields.fiscalCondition,
      fiscalAddress: fields.fiscalAddress.trim(),
      administrativeNotes: fields.administrativeNotes.trim(),
    };

    setIsSubmitting(true);
    try {
      const result = patient
        ? await browserApi.PATCH("/api/v1/patients/{id}", {
            params: { path: { id: patient.id } },
            body,
          })
        : await browserApi.POST("/api/v1/patients", { body });

      if (result.error || !result.data) {
        // The API detail names what is wrong — a document that does not match the
        // country, a duplicate number — and that is what the person needs to read.
        setError(apiErrorMessage(result.error));
        return;
      }

      onSaved(result.data.id);
      if (!patient) setFields(fieldsFrom(null));
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
      <h2 className="text-sm font-semibold">
        {isEditing ? "Editar paciente" : "Nuevo paciente"}
      </h2>

      <div className="mt-4 grid gap-4 sm:grid-cols-2">
        <Field label="Nombre y apellido" htmlFor={id("fullName")}>
          <input
            id={id("fullName")}
            value={fields.fullName}
            onChange={(event) => set("fullName", event.target.value)}
            required
            maxLength={200}
            placeholder="Ana Pérez"
            className={inputClass}
          />
        </Field>

        <Field label="País" htmlFor={id("country")}>
          <select
            id={id("country")}
            value={fields.country}
            onChange={(event) => changeCountry(event.target.value)}
            className={inputClass}
          >
            {COUNTRIES.map((country) => (
              <option key={country.code} value={country.code}>
                {country.label}
              </option>
            ))}
          </select>
        </Field>

        <Field label="Fecha de nacimiento" htmlFor={id("birthDate")}>
          <input
            id={id("birthDate")}
            type="date"
            value={fields.birthDate}
            onChange={(event) => set("birthDate", event.target.value)}
            className={inputClass}
          />
        </Field>

        <Field label="Teléfono" htmlFor={id("phone")}>
          <input
            id={id("phone")}
            value={fields.phone}
            onChange={(event) => set("phone", event.target.value)}
            placeholder="+54 11 4444-1111"
            className={inputClass}
          />
        </Field>

        <Field label="Tipo de documento" htmlFor={id("documentKind")}>
          <select
            id={id("documentKind")}
            value={fields.documentKind}
            onChange={(event) => set("documentKind", event.target.value)}
            className={inputClass}
          >
            <option value="">Sin documento</option>
            {documentKinds.map((kind) => (
              <option key={kind} value={kind}>
                {documentKindLabel(kind)}
              </option>
            ))}
          </select>
        </Field>

        <Field label="Número de documento" htmlFor={id("documentNumber")}>
          <input
            id={id("documentNumber")}
            value={fields.documentNumber}
            onChange={(event) => set("documentNumber", event.target.value)}
            placeholder="30.111.222"
            className={`${inputClass} font-mono`}
          />
        </Field>

        <Field label="Email" htmlFor={id("email")}>
          <input
            id={id("email")}
            type="email"
            value={fields.email}
            onChange={(event) => set("email", event.target.value)}
            className={inputClass}
          />
        </Field>

        <Field label="Contacto de emergencia" htmlFor={id("emergencyContactName")}>
          <input
            id={id("emergencyContactName")}
            value={fields.emergencyContactName}
            onChange={(event) => set("emergencyContactName", event.target.value)}
            placeholder="Nombre y vínculo"
            className={inputClass}
          />
        </Field>

        <Field label="Teléfono de emergencia" htmlFor={id("emergencyContactPhone")}>
          <input
            id={id("emergencyContactPhone")}
            value={fields.emergencyContactPhone}
            onChange={(event) => set("emergencyContactPhone", event.target.value)}
            className={inputClass}
          />
        </Field>

        <Field label="Identificador fiscal" htmlFor={id("taxIdKind")}>
          <select
            id={id("taxIdKind")}
            value={fields.taxIdKind}
            onChange={(event) => set("taxIdKind", event.target.value)}
            className={inputClass}
          >
            <option value="">Sin datos fiscales</option>
            {taxIDKinds.map((kind) => (
              <option key={kind} value={kind}>
                {taxIDKindLabel(kind)}
              </option>
            ))}
          </select>
        </Field>

        <Field label="Número fiscal" htmlFor={id("taxIdNumber")}>
          <input
            id={id("taxIdNumber")}
            value={fields.taxIdNumber}
            onChange={(event) => set("taxIdNumber", event.target.value)}
            className={`${inputClass} font-mono`}
          />
        </Field>

        <Field label="Condición fiscal" htmlFor={id("fiscalCondition")}>
          <select
            id={id("fiscalCondition")}
            value={fields.fiscalCondition}
            onChange={(event) => set("fiscalCondition", event.target.value)}
            className={inputClass}
          >
            <option value="">Sin definir</option>
            {fiscalConditions.map((condition) => (
              <option key={condition} value={condition}>
                {fiscalConditionLabel(condition)}
              </option>
            ))}
          </select>
        </Field>

        <Field label="Dirección fiscal" htmlFor={id("fiscalAddress")}>
          <input
            id={id("fiscalAddress")}
            value={fields.fiscalAddress}
            onChange={(event) => set("fiscalAddress", event.target.value)}
            className={inputClass}
          />
        </Field>

        <Field label="Notas administrativas" htmlFor={id("administrativeNotes")} wide>
          <textarea
            id={id("administrativeNotes")}
            value={fields.administrativeNotes}
            onChange={(event) => set("administrativeNotes", event.target.value)}
            rows={2}
            className={inputClass}
          />
        </Field>
      </div>

      {error ? (
        <p role="alert" className="mt-4 text-sm text-danger">
          {error}
        </p>
      ) : null}

      <div className="mt-5 flex items-center gap-3">
        <button
          type="submit"
          disabled={isSubmitting}
          className="inline-flex items-center rounded-md bg-brand px-4 py-2 text-sm font-medium text-brand-ink disabled:opacity-60"
        >
          {isSubmitting ? "Guardando…" : isEditing ? "Guardar cambios" : "Crear paciente"}
        </button>

        {onCancel ? (
          <button
            type="button"
            onClick={onCancel}
            className="rounded-md border border-border px-3 py-2 text-sm text-ink-muted"
          >
            Cancelar
          </button>
        ) : null}
      </div>
    </form>
  );
}

const inputClass =
  "mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-brand";

function Field({
  label,
  htmlFor,
  wide,
  children,
}: {
  label: string;
  htmlFor: string;
  wide?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div className={wide ? "sm:col-span-2" : undefined}>
      <label htmlFor={htmlFor} className="block text-xs font-medium text-ink-muted">
        {label}
      </label>
      {children}
    </div>
  );
}
