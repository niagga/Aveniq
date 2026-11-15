"use client";

import { useId, useState } from "react";

import {
  DEFAULT_CURRENCY,
  formatMoney,
  parseAmount,
  voucherNumber,
  type Subject,
  type Voucher,
} from "@/entities/billing/model";
import { SubjectPicker } from "@/features/billing/components/subject-picker";
import { apiErrorMessage, browserApi, CONNECTION_ERROR_MESSAGE } from "@/shared/api/client";
import { Field, inputClass } from "@/shared/ui/field";

/** The API's limit for the concept (1 to 200 characters). */
const CONCEPT_MAX = 200;

/**
 * Issues an internal voucher.
 *
 * What the screen sends is the concept, the amount and, optionally, the patient. It never
 * sends the buyer or the practice name: the server stamps both from its own records, so a
 * voucher cannot name someone the patient record does not. The number is assigned by the
 * server too, and the screen shows it once the voucher exists.
 */
export function IssueForm({ onIssued }: { onIssued: (voucher: Voucher) => void }) {
  const prefix = useId();
  const [subject, setSubject] = useState<Subject | null>(null);
  const [concept, setConcept] = useState("");
  const [amount, setAmount] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [issued, setIssued] = useState<Voucher | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setIssued(null);

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
          currency: DEFAULT_CURRENCY,
          ...(subject !== null ? { patientId: subject.patientId } : {}),
        },
      });

      if (!response.ok || !data) {
        setError(apiErrorMessage(apiError));
        return;
      }

      setIssued(data);
      setConcept("");
      setAmount("");
      setSubject(null);
      onIssued(data);
    } catch {
      setError(CONNECTION_ERROR_MESSAGE);
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <section className="rounded-[var(--radius-panel)] border border-border bg-surface p-5">
      <h2 className="text-sm font-semibold">Emitir un comprobante</h2>
      <p className="mt-1 text-sm text-ink-muted">
        Comprobante interno: no es una factura electrónica. El número lo asigna Aveniq y no se
        reutiliza, ni siquiera si el comprobante se anula.
      </p>

      <form onSubmit={handleSubmit} noValidate className="mt-4 grid gap-4">
        <SubjectPicker value={subject} onChange={setSubject} />

        <div className="grid gap-4 sm:grid-cols-[2fr_1fr]">
          <Field label="Concepto" htmlFor={`${prefix}-concept`}>
            <input
              id={`${prefix}-concept`}
              value={concept}
              onChange={(event) => setConcept(event.target.value)}
              maxLength={CONCEPT_MAX}
              placeholder="Sesión de psicoterapia individual"
              className={inputClass}
            />
          </Field>

          <Field
            label={`Importe (${DEFAULT_CURRENCY})`}
            htmlFor={`${prefix}-amount`}
            hint="Por ejemplo 25000 o 25000,50."
          >
            <input
              id={`${prefix}-amount`}
              value={amount}
              onChange={(event) => setAmount(event.target.value)}
              inputMode="decimal"
              autoComplete="off"
              className={inputClass}
            />
          </Field>
        </div>

        <div>
          <button
            type="submit"
            disabled={isSubmitting}
            className="inline-flex items-center justify-center rounded-md bg-brand px-4 py-2 text-sm font-medium text-brand-ink disabled:opacity-60"
          >
            {isSubmitting ? "Emitiendo…" : "Emitir comprobante"}
          </button>
        </div>
      </form>

      {error ? (
        <p role="alert" className="mt-4 text-sm text-danger">
          {error}
        </p>
      ) : null}

      {issued ? (
        <p role="status" className="mt-4 text-sm text-success">
          Emitimos el comprobante {voucherNumber(issued.number)} por{" "}
          {formatMoney(issued.amountMinor, issued.currency)}.
        </p>
      ) : null}
    </section>
  );
}
