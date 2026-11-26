"use client";

import { useEffect, useId, useState } from "react";

import {
  DEFAULT_CURRENCY,
  formatDay,
  formatMoney,
  parseAmount,
  sessionBlockReason,
  sessionLabel,
  type Professional,
  type Session,
  type Subject,
  type Voucher,
} from "@/entities/billing/model";
import { minorToInput } from "@/entities/tariff/model";
import { SubjectPicker } from "@/features/billing/components/subject-picker";
import { apiErrorMessage, browserApi, CONNECTION_ERROR_MESSAGE } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { Field, inputClass } from "@/shared/ui/field";
import { FormModal } from "@/shared/ui/form-modal";

/** The API's limit for the concept (1 to 200 characters). */
const CONCEPT_MAX = 200;

/** The value of the session picker when no session is chosen: a charge of the patient. */
const NO_SESSION = "";

/** The sessions of one patient, or why they could not be read. Keyed by patient so a stale answer is never shown. */
type SessionsOfPatient = { patientId: string; items: Session[]; failure: string | null };

/**
 * Issues an internal voucher, in a popup (E8, S25, T76): the button of the header opens it
 * over the blurred screen instead of an always-open block between the filters and the list.
 *
 * A voucher is either for a session of a patient, or a loose charge. For a session the screen
 * sends the session and nothing else about the patient: the server takes the patient, the
 * modality and the professional from the session, and it refuses a session billed twice. The
 * amount starts at the tariff's proposal and the person may change it. A voucher for a patient
 * is billed in the patient's currency. A loose charge may name a professional.
 *
 * The screen never sends the buyer, the practice name or the photo: the server stamps them
 * from its own records. The number is assigned by the server too.
 *
 * On success the popup closes and the screen opens the detail of the voucher it just issued:
 * the confirmation the inline form used to print is now the detail itself, which shows the
 * number, the amount and the frozen snapshot.
 */
export function IssueVoucherModal({
  open,
  onClose,
  onIssued,
}: {
  open: boolean;
  onClose: () => void;
  onIssued: (voucher: Voucher) => void;
}) {
  const prefix = useId();
  const writesLocked = useWritesLocked();
  const [subject, setSubject] = useState<Subject | null>(null);
  const [sessionsOf, setSessionsOf] = useState<SessionsOfPatient | null>(null);
  const [sessionId, setSessionId] = useState(NO_SESSION);
  const [professionals, setProfessionals] = useState<Professional[]>([]);
  const [professionalId, setProfessionalId] = useState("");
  const [concept, setConcept] = useState("");
  const [amount, setAmount] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const patientId = subject?.patientId;
  const sessions = sessionsOf !== null && sessionsOf.patientId === patientId ? sessionsOf : null;
  const session = sessions?.items.find((item) => item.id === sessionId) ?? null;
  const currency = subject?.billingCurrency ?? DEFAULT_CURRENCY;

  // Reopening starts from a clean draft: the popup keeps one life per opening, so a cancelled
  // voucher cannot leak into the next one.
  useEffect(() => {
    if (!open) return;
    setSubject(null);
    setSessionsOf(null);
    setSessionId(NO_SESSION);
    setProfessionalId("");
    setConcept("");
    setAmount("");
    setError(null);
  }, [open]);

  // The catalog feeds only an optional field of a loose charge: it is worth a request only
  // while the popup is actually open, not on every visit to the screen.
  useEffect(() => {
    if (!open) return;
    let active = true;
    void (async () => {
      try {
        const { data, response } = await browserApi.GET("/api/v1/billing/professionals");
        if (active && response.ok) setProfessionals(data?.items ?? []);
      } catch {
        // A loose charge can still be issued without the list.
      }
    })();
    return () => {
      active = false;
    };
  }, [open]);

  useEffect(() => {
    if (patientId === undefined) return;
    let active = true;
    void (async () => {
      try {
        const { data, error: apiError, response } = await browserApi.GET(
          "/api/v1/billing/subjects/{patientId}/sessions",
          { params: { path: { patientId } } },
        );
        if (!active) return;
        if (!response.ok) {
          setSessionsOf({ patientId, items: [], failure: apiErrorMessage(apiError) });
          return;
        }
        setSessionsOf({ patientId, items: data?.items ?? [], failure: null });
      } catch {
        if (active) setSessionsOf({ patientId, items: [], failure: CONNECTION_ERROR_MESSAGE });
      }
    })();
    return () => {
      active = false;
    };
  }, [patientId]);

  function changeSubject(next: Subject | null) {
    setSubject(next);
    setSessionId(NO_SESSION);
  }

  function chooseSession(id: string) {
    setSessionId(id);
    const chosen = sessions?.items.find((item) => item.id === id);
    if (chosen === undefined) return;
    setConcept(`Sesión del ${formatDay(chosen.startsAt)}`);
    setAmount(chosen.proposal ? minorToInput(chosen.proposal.amountMinor) : "");
  }

  async function handleSubmit() {
    if (writesLocked) return;
    setError(null);

    const trimmedConcept = concept.trim();
    if (trimmedConcept === "") {
      setError("Escribí el concepto del comprobante.");
      return;
    }
    const parsed = parseAmount(amount);
    if (!parsed.ok) {
      setError(parsed.message);
      return;
    }

    setIsSubmitting(true);
    try {
      const { data, error: apiError, response } = await browserApi.POST("/api/v1/billing/vouchers", {
        body: {
          concept: trimmedConcept,
          amountMinor: parsed.minor,
          currency,
          ...(subject !== null ? { patientId: subject.patientId } : {}),
          ...(session !== null ? { appointmentId: session.id } : {}),
          ...(session === null && professionalId !== "" ? { professionalUserId: professionalId } : {}),
        },
      });

      if (!response.ok || !data) {
        setError(apiErrorMessage(apiError));
        return;
      }

      // Close first, then hand the voucher over: the parent opens its detail, so the
      // leaving popup and the popup that takes its place are never on screen at once.
      onClose();
      onIssued(data);
    } catch {
      setError(CONNECTION_ERROR_MESSAGE);
    } finally {
      setIsSubmitting(false);
    }
  }

  const proposal = session?.proposal ?? null;

  return (
    <FormModal
      open={open}
      title="Emitir un comprobante"
      description="Comprobante interno: no es una factura electrónica. El número lo asigna Aveniq y no se reutiliza, ni siquiera si el comprobante se anula."
      size="lg"
      submitLabel="Emitir el comprobante"
      busyLabel="Emitiendo…"
      busy={isSubmitting}
      error={error}
      onClose={onClose}
      onSubmit={handleSubmit}
    >
      <SubjectPicker value={subject} onChange={changeSubject} disabled={writesLocked} />

      {subject !== null ? (
        <SessionPicker
          id={`${prefix}-session`}
          sessions={sessions}
          value={sessionId}
          onChange={chooseSession}
          disabled={writesLocked}
        />
      ) : null}

      {session === null ? (
        <ProfessionalPicker
          id={`${prefix}-professional`}
          professionals={professionals}
          value={professionalId}
          onChange={setProfessionalId}
          disabled={writesLocked}
        />
      ) : (
        <Field label="Profesional" htmlFor={`${prefix}-professional-fixed`}>
          <input
            id={`${prefix}-professional-fixed`}
            value={session.professionalName}
            readOnly
            className={inputClass}
          />
        </Field>
      )}

      <div className="grid gap-4 sm:grid-cols-[2fr_1fr]">
        <Field label="Concepto" htmlFor={`${prefix}-concept`}>
          <input
            id={`${prefix}-concept`}
            value={concept}
            onChange={(event) => setConcept(event.target.value)}
            maxLength={CONCEPT_MAX}
            placeholder="Sesión de psicoterapia individual"
            disabled={writesLocked}
            className={inputClass}
          />
        </Field>

        <Field
          label={`Importe (${currency})`}
          htmlFor={`${prefix}-amount`}
          hint={amountHint(session !== null, proposal?.amountMinor ?? null, proposal?.currency ?? null)}
        >
          <input
            id={`${prefix}-amount`}
            value={amount}
            onChange={(event) => setAmount(event.target.value)}
            inputMode="decimal"
            autoComplete="off"
            disabled={writesLocked}
            className={inputClass}
          />
        </Field>
      </div>
    </FormModal>
  );
}

/** What the amount field says about where its value comes from. */
function amountHint(hasSession: boolean, proposedMinor: number | null, proposedCurrency: string | null): string {
  if (!hasSession) return "Por ejemplo 25000 o 25000,50.";
  if (proposedMinor === null || proposedCurrency === null) {
    return "El tarifario no tiene precio para este turno: escribí el importe.";
  }
  return `Propuesto por el tarifario: ${formatMoney(proposedMinor, proposedCurrency)}. Podés cambiarlo.`;
}

/**
 * The sessions of the patient to bill. A session that is cancelled or already billed is listed
 * and disabled, with the reason, so the person sees why it cannot be picked.
 */
function SessionPicker({
  id,
  sessions,
  value,
  onChange,
  disabled,
}: {
  id: string;
  sessions: SessionsOfPatient | null;
  value: string;
  onChange: (id: string) => void;
  disabled: boolean;
}) {
  if (sessions === null) {
    return <p className="text-xs text-ink-muted">Cargando los turnos del paciente…</p>;
  }
  if (sessions.failure !== null) {
    return (
      <p role="alert" className="text-xs text-danger">
        {sessions.failure}
      </p>
    );
  }
  if (sessions.items.length === 0) {
    return (
      <p className="text-xs text-ink-muted">
        El paciente no tiene turnos: el cobro se emite sin turno.
      </p>
    );
  }
  return (
    <Field
      label="Turno (opcional)"
      htmlFor={id}
      hint="Elegir un turno toma su paciente, su modalidad y su profesional, y propone el importe del tarifario."
    >
      <select
        id={id}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        disabled={disabled}
        className={inputClass}
      >
        <option value={NO_SESSION}>Sin turno (cobro del paciente)</option>
        {sessions.items.map((item) => {
          const blocked = sessionBlockReason(item);
          return (
            <option key={item.id} value={item.id} disabled={blocked !== null}>
              {sessionLabel(item)}
              {blocked !== null ? ` · ${blocked}` : ""}
            </option>
          );
        })}
      </select>
    </Field>
  );
}

/** The professional of a voucher that names no session. Optional. */
function ProfessionalPicker({
  id,
  professionals,
  value,
  onChange,
  disabled,
}: {
  id: string;
  professionals: Professional[];
  value: string;
  onChange: (id: string) => void;
  disabled: boolean;
}) {
  return (
    <Field label="Profesional (opcional)" htmlFor={id}>
      <select
        id={id}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        disabled={disabled}
        className={inputClass}
      >
        <option value="">Sin profesional</option>
        {professionals.map((professional) => (
          <option key={professional.id} value={professional.id}>
            {professional.fullName}
          </option>
        ))}
      </select>
    </Field>
  );
}
