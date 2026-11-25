"use client";

import {
  cloneElement,
  createContext,
  isValidElement,
  useContext,
  useEffect,
  useId,
  useState,
  type ReactElement,
} from "react";

import {
  COUNTRIES,
  CURRENCIES,
  currencyLabel,
  DOCUMENT_KINDS_BY_COUNTRY,
  documentKindLabel,
  FISCAL_CONDITIONS_BY_COUNTRY,
  fiscalConditionLabel,
  TAX_ID_KINDS_BY_COUNTRY,
  taxIDKindLabel,
  type Patient,
} from "@/entities/patient/model";
import type { Insurer } from "@/entities/insurer/model";
import { useClients } from "@/features/clients/use-clients";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import type { components } from "@/shared/api/schema";
import { useWritesLocked } from "@/shared/auth/practice";
import { Field as BaseField, fieldErrorId, inputClass } from "@/shared/ui/field";
import { FormModal } from "@/shared/ui/form-modal";

type PatientProfile = components["schemas"]["PatientProfileBody"];
type NewClient = NonNullable<components["schemas"]["CreatePatientBody"]["client"]>;

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
  coverageKind: string;
  insurerId: string;
  memberNumber: string;
  clientId: string;
  billingCurrency: string;
  taxIdKind: string;
  taxIdNumber: string;
  fiscalCondition: string;
  fiscalAddress: string;
  administrativeNotes: string;
};

/**
 * The fiscal fields and the currency describe the client that pays, never the patient (T72).
 * They are typed here only when the patient brings its own new client; picking an existing one
 * leaves them empty, and editing a patient never touches them because the API does not accept
 * them on PATCH: they are edited in Clientes.
 */
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
    coverageKind: patient?.coverageKind ?? "private",
    insurerId: patient?.insurerId ?? "",
    memberNumber: patient?.memberNumber ?? "",
    clientId: patient?.clientId ?? "",
    billingCurrency: "",
    taxIdKind: "",
    taxIdNumber: "",
    fiscalCondition: "",
    fiscalAddress: "",
    administrativeNotes: patient?.administrativeNotes ?? "",
  };
}

/**
 * Create and edit share this popup (E8, T75): the API takes the same profile for both, so
 * two forms would be two places for a field to go missing.
 *
 * The document and the tax id are optional pairs. Sending a kind without a number is
 * refused by the server with a 422 that names the field, but stopping it here saves a
 * round trip and says it in the language of the form.
 */
/**
 * The latest date a patient can have been born: yesterday.
 *
 * A birth date of today or later describes someone zero days old or not born yet, and the
 * practice has no such patient. It is computed in local time so the picker's `max` and the
 * check in `validate()` agree with the date the user sees.
 *
 * The API only rejects the future (internal/domain/patient/patient.go:231), so this bound is
 * deliberately stricter on purpose until the backend enforces the same one (T11).
 */
function birthDateCeiling(): string {
  const limit = new Date();
  limit.setDate(limit.getDate() - 1);
  const local = new Date(limit.getTime() - limit.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 10);
}

type FieldErrors = Partial<Record<keyof Fields, string>>;

/** The order a user reads the form, so "the first problem" means the first one on screen. */
const FIELD_ORDER: readonly (keyof Fields)[] = [
  "fullName",
  "birthDate",
  "documentKind",
  "documentNumber",
  "clientId",
  "billingCurrency",
  "insurerId",
  "memberNumber",
  "taxIdKind",
  "taxIdNumber",
];

const FieldErrorsContext = createContext<FieldErrors>({});

/**
 * Every problem of the form, keyed by the field that has it.
 *
 * It used to be a single string, so the user learned that *something* was wrong and had to
 * guess what. The pairs are symmetrical on purpose: choosing a document type without a
 * number is a problem on the number, and typing a number without a type is a problem on the
 * type. Neither is acceptable, and neither is a form-level message.
 */
function collectErrors(values: Fields, insured: boolean, needsClient: boolean): FieldErrors {
  const errors: FieldErrors = {};

  if (values.fullName.trim() === "") {
    errors.fullName = "El nombre es obligatorio.";
  }
  if (values.birthDate !== "" && values.birthDate > birthDateCeiling()) {
    errors.birthDate = "La fecha tiene que ser anterior a hoy.";
  }
  if (values.documentKind !== "" && values.documentNumber.trim() === "") {
    errors.documentNumber = "Elegiste un tipo de documento sin número.";
  }
  if (values.documentKind === "" && values.documentNumber.trim() !== "") {
    errors.documentKind = "Elegí el tipo del documento que cargaste.";
  }
  if (needsClient && values.taxIdKind !== "" && values.taxIdNumber.trim() === "") {
    errors.taxIdNumber = "Elegiste un identificador fiscal sin número.";
  }
  if (needsClient && values.taxIdKind === "" && values.taxIdNumber.trim() !== "") {
    errors.taxIdKind = "Elegí el tipo del identificador fiscal.";
  }
  // Only a new client of its own needs a currency here: an existing client already has one, and
  // editing a patient never writes on the client. The currency is chosen, never derived (L61).
  if (needsClient && values.billingCurrency === "") {
    errors.billingCurrency = "Elegí la moneda de facturación del cliente.";
  }
  if (insured && values.insurerId === "") {
    errors.insurerId = "Elegí la mutual que cubre al paciente.";
  }
  if (insured && values.memberNumber.trim() === "") {
    errors.memberNumber = "Cargá el número de afiliado de la mutual.";
  }

  return errors;
}

/** Recovers the field name from a control id, which is always `${prefix}-${name}`. */
function fieldNameOf(htmlFor: string): keyof Fields | null {
  const names = Object.keys(fieldsFrom(undefined)) as (keyof Fields)[];
  return names.find((name) => htmlFor.endsWith(`-${name}`)) ?? null;
}

export function PatientFormModal({
  open,
  patient,
  insurers,
  onSaved,
  onClose,
}: {
  open: boolean;
  patient?: Patient | null;
  insurers: Insurer[] | null;
  onSaved: (id: string) => void;
  onClose: () => void;
}) {
  const [fields, setFields] = useState<Fields>(() => fieldsFrom(patient));
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [isSubmitting, setIsSubmitting] = useState(false);
  const writesLocked = useWritesLocked();
  // The client catalog is only needed while the popup is open: the roster is not a reason to
  // ask the API for clients it will not show.
  const { clients } = useClients(open);

  const prefix = useId();
  const id = (name: keyof Fields) => `${prefix}-${name}`;

  // Reopening starts from the patient again: the popup keeps one life per opening, so
  // a cancelled or saved draft cannot leak into the next one.
  useEffect(() => {
    if (!open) return;
    setFields(fieldsFrom(patient));
    setFieldErrors({});
    setError(null);
  }, [open, patient]);

  const isEditing = Boolean(patient);
  const documentKinds = DOCUMENT_KINDS_BY_COUNTRY[fields.country] ?? [];
  const taxIDKinds = TAX_ID_KINDS_BY_COUNTRY[fields.country] ?? [];
  const fiscalConditions = FISCAL_CONDITIONS_BY_COUNTRY[fields.country] ?? [];
  const isInsured = fields.coverageKind === "insurer";
  // A new patient without a chosen client brings its own: the API creates one named after the
  // patient with the fiscal data typed here, which is the default T74 asks for.
  const ownsClient = !isEditing && fields.clientId === "";
  const clientOptions = (clients ?? []).filter((client) => client.isActive);

  // A mutual that was deactivated after this patient was covered stays selectable while
  // editing that patient: the API keeps the coverage and refuses only a new choice.
  const insurerOptions = (insurers ?? []).filter(
    (insurer) => insurer.isActive || insurer.id === patient?.insurerId,
  );

  function set<K extends keyof Fields>(name: K, value: Fields[K]) {
    setFields((current) => ({ ...current, [name]: value }));
    // The error describes the value being replaced, so it clears as the user types.
    setFieldErrors((current) => {
      if (!(name in current)) return current;
      const next = { ...current };
      delete next[name];
      return next;
    });
  }

  // A particular patient carries no mutual and no member number: clearing them here
  // keeps a switch back to private from sending data the API would refuse.
  function changeCoverage(coverageKind: string) {
    setFields((current) => ({
      ...current,
      coverageKind,
      insurerId: coverageKind === "insurer" ? current.insurerId : "",
      memberNumber: coverageKind === "insurer" ? current.memberNumber : "",
    }));
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

  // Validation runs on blur and on submit, never on every keystroke: a field that turns red
  // after the first character is noise, and one that only complains after the trip is worse.
  function checkOnBlur(event: React.FocusEvent) {
    const name = fieldNameOf((event.target as HTMLElement).id);
    if (name === null) return;

    const errors = collectErrors(fields, isInsured, ownsClient);
    setFieldErrors((current) => {
      const next = { ...current };
      if (errors[name] === undefined) delete next[name];
      else next[name] = errors[name];
      return next;
    });
  }

  async function submit() {
    if (writesLocked) return;
    setError(null);

    const errors = collectErrors(fields, isInsured, ownsClient);
    setFieldErrors(errors);

    const first = FIELD_ORDER.find((name) => errors[name] !== undefined);
    if (first !== undefined) {
      // The message sits next to its field now, but a keyboard user still has to be taken there.
      document.getElementById(id(first))?.focus();
      return;
    }

    const profile: PatientProfile = {
      fullName: fields.fullName.trim(),
      country: fields.country,
      birthDate: fields.birthDate.trim(),
      documentKind: fields.documentKind,
      documentNumber: fields.documentNumber.trim(),
      phone: fields.phone.trim(),
      email: fields.email.trim(),
      emergencyContactName: fields.emergencyContactName.trim(),
      emergencyContactPhone: fields.emergencyContactPhone.trim(),
      coverageKind: fields.coverageKind as PatientProfile["coverageKind"],
      insurerId: isInsured ? fields.insurerId : "",
      memberNumber: isInsured ? fields.memberNumber.trim() : "",
      administrativeNotes: fields.administrativeNotes.trim(),
    };

    // A patient is created together with its own client, or linked to one that already exists.
    // Both carry the fiscal data the receipts freeze; the patient no longer does (T72).
    const ownClient: NewClient = {
      displayName: fields.fullName.trim(),
      country: fields.country,
      taxIdKind: fields.taxIdKind,
      taxIdNumber: fields.taxIdNumber.trim(),
      fiscalCondition: fields.fiscalCondition,
      fiscalAddress: fields.fiscalAddress.trim(),
      billingCurrency: fields.billingCurrency as NewClient["billingCurrency"],
    };

    setIsSubmitting(true);
    try {
      const result = patient
        ? await browserApi.PATCH("/api/v1/patients/{id}", {
            params: { path: { id: patient.id } },
            body: profile,
          })
        : await browserApi.POST("/api/v1/patients", {
            body: ownsClient
              ? { ...profile, client: ownClient }
              : { ...profile, clientId: fields.clientId },
          });

      if (result.error || !result.data) {
        // The API detail names what is wrong — a document that does not match the
        // country, a duplicate number — and that is what the person needs to read.
        setError(apiErrorMessage(result.error));
        return;
      }

      onSaved(result.data.id);
    } catch {
      setError("No pudimos contactar al servidor. Revisá que la API esté corriendo.");
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <FormModal
      open={open}
      title={isEditing ? "Editar paciente" : "Nuevo paciente"}
      size="lg"
      submitLabel={isEditing ? "Guardar cambios" : "Crear paciente"}
      busy={isSubmitting}
      error={error}
      onClose={onClose}
      onSubmit={submit}
    >
      <FieldErrorsContext.Provider value={fieldErrors}>
        <div className="grid gap-4 sm:grid-cols-2" onBlur={checkOnBlur}>
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
            max={birthDateCeiling()}
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

        <Field label="Cobertura" htmlFor={id("coverageKind")}>
          <select
            id={id("coverageKind")}
            value={fields.coverageKind}
            onChange={(event) => changeCoverage(event.target.value)}
            className={inputClass}
          >
            <option value="private">Particular</option>
            <option value="insurer">Mutual</option>
          </select>
        </Field>

        {isInsured ? (
          <Field label="Mutual" htmlFor={id("insurerId")}>
            <select
              id={id("insurerId")}
              value={fields.insurerId}
              onChange={(event) => set("insurerId", event.target.value)}
              className={inputClass}
            >
              <option value="">{insurers === null ? "Cargando mutuales…" : "Elegí una mutual"}</option>
              {insurerOptions.map((insurer) => (
                <option key={insurer.id} value={insurer.id}>
                  {insurer.isActive ? insurer.name : `${insurer.name} (dada de baja)`}
                </option>
              ))}
            </select>
          </Field>
        ) : null}

        {isInsured ? (
          <Field label="Número de afiliado" htmlFor={id("memberNumber")}>
            <input
              id={id("memberNumber")}
              value={fields.memberNumber}
              onChange={(event) => set("memberNumber", event.target.value)}
              maxLength={30}
              className={`${inputClass} font-mono`}
            />
          </Field>
        ) : null}

        {isEditing ? (
          <Field label="Cliente que paga" htmlFor={id("clientId")} wide>
            <p className="text-sm">
              {patient?.clientDisplayName || "—"}{" "}
              <span className="text-ink-muted">· sus datos fiscales se editan en Clientes.</span>
            </p>
          </Field>
        ) : (
          <Field label="Cliente que paga" htmlFor={id("clientId")} wide>
            <select
              id={id("clientId")}
              value={fields.clientId}
              onChange={(event) => set("clientId", event.target.value)}
              className={inputClass}
            >
              <option value="">El propio paciente (cliente nuevo)</option>
              {clientOptions.map((client) => (
                <option key={client.id} value={client.id}>
                  {client.displayName}
                </option>
              ))}
            </select>
          </Field>
        )}

        {ownsClient ? (
          <>
        <Field label="Moneda de facturación" htmlFor={id("billingCurrency")}>
          <select
            id={id("billingCurrency")}
            value={fields.billingCurrency}
            onChange={(event) => set("billingCurrency", event.target.value)}
            required
            className={inputClass}
          >
            <option value="">Elegí una moneda</option>
            {CURRENCIES.map((code) => (
              <option key={code} value={code}>
                {currencyLabel(code)}
              </option>
            ))}
          </select>
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
          </>
        ) : null}

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

      </FieldErrorsContext.Provider>
    </FormModal>
  );
}

/**
 * One labelled control of the patient form.
 *
 * It wires the error for the whole grid from one place: the error text comes from the shared
 * Field, and the control inside receives `aria-invalid` plus `aria-describedby` pointing at
 * that text, so the two cannot drift apart.
 */
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
  const errors = useContext(FieldErrorsContext);
  const name = fieldNameOf(htmlFor);
  const error = name === null ? undefined : errors[name];

  const described = error === undefined ? undefined : fieldErrorId(htmlFor);
  const control = isValidElement(children)
    ? cloneElement(
        children as ReactElement<{
          "aria-invalid"?: boolean;
          "aria-describedby"?: string;
        }>,
        {
          "aria-invalid": error === undefined ? undefined : true,
          "aria-describedby": described,
        },
      )
    : children;

  return (
    <div className={wide ? "sm:col-span-2" : undefined}>
      <BaseField label={label} htmlFor={htmlFor} error={error}>
        {control}
      </BaseField>
    </div>
  );
}
