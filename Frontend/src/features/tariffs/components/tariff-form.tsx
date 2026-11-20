"use client";

import Link from "next/link";
import { useId, useState } from "react";

import { parseAmount } from "@/entities/billing/model";
import type { Insurer } from "@/entities/insurer/model";
import {
  COVERAGES,
  CURRENCIES,
  MODALITIES,
  coverageLabel,
  formatValidFrom,
  minorToInput,
  modalityLabel,
  parsePatientShare,
  todayIso,
  type Coverage,
  type Currency,
  type Modality,
  type Tariff,
  type TariffCreate,
} from "@/entities/tariff/model";
import { useInsurers } from "@/features/insurers/use-insurers";
import { useStaff } from "@/features/patients/staff";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { getSessionUser } from "@/shared/auth/session";

const LABEL = "block text-xs font-medium text-ink-muted";
const FIELD = "mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-brand";
const DAY = /^\d{4}-\d{2}-\d{2}$/;

type Amounts = { insurerMinor: number; patientMinor: number };
type Read = { ok: true; amounts: Amounts } | { ok: false; message: string };

/**
 * Creates a price, or changes the amounts of one that exists. Only owner and admin reach it.
 *
 * Nothing is proposed: the coverage, the modality and the currency start empty, so a price is
 * never written by accident. A price is identified by its professional, coverage, modality,
 * currency and start day, so an existing one only changes its amounts here.
 */
export function TariffForm({
  tariff,
  onSaved,
  onCancel,
}: {
  tariff?: Tariff | null;
  onSaved: () => void;
  onCancel: () => void;
}) {
  const prefix = useId();
  const writesLocked = useWritesLocked();
  const isEditing = Boolean(tariff);

  const staff = useStaff(getSessionUser()?.role);
  const { insurers } = useInsurers(true);
  const professionals = staff.filter((member) => member.role === "professional");
  const activeInsurers: Insurer[] = (insurers ?? []).filter((insurer) => insurer.isActive);

  const [professionalId, setProfessionalId] = useState("");
  const [coverageInput, setCoverageInput] = useState<Coverage | "">("");
  const [insurerId, setInsurerId] = useState("");
  const [modality, setModality] = useState<Modality | "">("");
  const [currency, setCurrency] = useState<Currency | "">("");
  const [validFrom, setValidFrom] = useState(todayIso());
  const [patientAmount, setPatientAmount] = useState(tariff ? minorToInput(tariff.patientMinor) : "");
  const [insurerAmount, setInsurerAmount] = useState(
    tariff && tariff.coverage === "insurer" ? minorToInput(tariff.insurerMinor) : "",
  );
  const [error, setError] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  // An existing price keeps its coverage; a new one takes it from the form.
  const coverage: Coverage | "" = tariff ? tariff.coverage : coverageInput;

  function readAmounts(kind: Coverage): Read {
    if (kind === "private") {
      const session = parseAmount(patientAmount);
      if (!session.ok) return { ok: false, message: session.message };
      return { ok: true, amounts: { insurerMinor: 0, patientMinor: session.minor } };
    }

    const insurerShare = parseAmount(insurerAmount);
    if (!insurerShare.ok) return { ok: false, message: `Lo que paga la mutual. ${insurerShare.message}` };
    const copago = parsePatientShare(patientAmount);
    if (!copago.ok) return { ok: false, message: `El copago del paciente. ${copago.message}` };
    return { ok: true, amounts: { insurerMinor: insurerShare.minor, patientMinor: copago.minor } };
  }

  async function save(send: () => Promise<{ data?: unknown; error?: unknown }>) {
    setIsSubmitting(true);
    try {
      const result = await send();
      if (result.error || !result.data) {
        // The API names what is wrong: a price for the same day, a mutual that is not active.
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

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (writesLocked) return;
    setError(null);

    if (tariff) {
      const read = readAmounts(tariff.coverage);
      if (!read.ok) {
        setError(read.message);
        return;
      }
      const { amounts } = read;
      await save(() =>
        browserApi.PATCH("/api/v1/tariffs/{id}", {
          params: { path: { id: tariff.id } },
          body: { insurerMinor: amounts.insurerMinor, patientMinor: amounts.patientMinor },
        }),
      );
      return;
    }

    if (professionalId === "") {
      setError("Elegí el profesional.");
      return;
    }
    if (coverage === "") {
      setError("Elegí la cobertura.");
      return;
    }
    if (coverage === "insurer" && insurerId === "") {
      setError("Elegí la mutual.");
      return;
    }
    if (modality === "") {
      setError("Elegí la modalidad.");
      return;
    }
    if (currency === "") {
      setError("Elegí la moneda.");
      return;
    }
    if (!DAY.test(validFrom)) {
      setError("Elegí el día desde el que rige el valor.");
      return;
    }
    const read = readAmounts(coverage);
    if (!read.ok) {
      setError(read.message);
      return;
    }

    const body: TariffCreate = {
      professionalUserId: professionalId,
      coverage,
      modality,
      billingCurrency: currency,
      validFrom,
      insurerMinor: read.amounts.insurerMinor,
      patientMinor: read.amounts.patientMinor,
      ...(coverage === "insurer" ? { insurerId } : {}),
    };
    await save(() => browserApi.POST("/api/v1/tariffs", { body }));
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="rounded-[var(--radius-panel)] border border-border bg-surface p-5"
      noValidate
    >
      <h2 className="text-sm font-semibold">{isEditing ? "Editar importes" : "Nuevo valor"}</h2>

      {tariff ? (
        <p className="mt-1 text-sm text-ink-muted">
          {tariff.professionalName} · {coverageLabel(tariff)} · {modalityLabel(tariff.modality)} ·{" "}
          {tariff.billingCurrency} · vigente desde {formatValidFrom(tariff.validFrom)}. Para cambiar
          cualquiera de estos datos, quitá el valor y cargalo de nuevo.
        </p>
      ) : null}

      {!isEditing && professionals.length === 0 ? (
        <p className="mt-3 text-sm text-ink-muted">
          No hay profesionales activos para elegir. Invitá uno desde{" "}
          <Link href="/usuarios" className="underline">
            Usuarios
          </Link>
          .
        </p>
      ) : null}

      <div className="mt-4 grid gap-4 sm:grid-cols-2">
        {!isEditing ? (
          <>
            <div>
              <label htmlFor={`${prefix}-professional`} className={LABEL}>
                Profesional
              </label>
              <select
                id={`${prefix}-professional`}
                value={professionalId}
                onChange={(event) => setProfessionalId(event.target.value)}
                className={FIELD}
              >
                <option value="">Elegí un profesional</option>
                {professionals.map((member) => (
                  <option key={member.id} value={member.id}>
                    {member.fullName}
                  </option>
                ))}
              </select>
            </div>

            <div>
              <label htmlFor={`${prefix}-coverage`} className={LABEL}>
                Cobertura
              </label>
              <select
                id={`${prefix}-coverage`}
                value={coverageInput}
                onChange={(event) => setCoverageInput(event.target.value as Coverage | "")}
                className={FIELD}
              >
                <option value="">Elegí la cobertura</option>
                {COVERAGES.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </div>

            {coverage === "insurer" ? (
              <div>
                <label htmlFor={`${prefix}-insurer`} className={LABEL}>
                  Mutual
                </label>
                <select
                  id={`${prefix}-insurer`}
                  value={insurerId}
                  onChange={(event) => setInsurerId(event.target.value)}
                  className={FIELD}
                >
                  <option value="">Elegí la mutual</option>
                  {activeInsurers.map((insurer) => (
                    <option key={insurer.id} value={insurer.id}>
                      {insurer.name}
                    </option>
                  ))}
                </select>
                {activeInsurers.length === 0 ? (
                  <p className="mt-1 text-xs text-ink-muted">
                    No hay mutuales activas. Cargá una en{" "}
                    <Link href="/mutuales" className="underline">
                      Mutuales
                    </Link>
                    .
                  </p>
                ) : null}
              </div>
            ) : null}

            <div>
              <label htmlFor={`${prefix}-modality`} className={LABEL}>
                Modalidad
              </label>
              <select
                id={`${prefix}-modality`}
                value={modality}
                onChange={(event) => setModality(event.target.value as Modality | "")}
                className={FIELD}
              >
                <option value="">Elegí la modalidad</option>
                {MODALITIES.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </div>

            <div>
              <label htmlFor={`${prefix}-currency`} className={LABEL}>
                Moneda
              </label>
              <select
                id={`${prefix}-currency`}
                value={currency}
                onChange={(event) => setCurrency(event.target.value as Currency | "")}
                className={FIELD}
              >
                <option value="">Elegí la moneda</option>
                {CURRENCIES.map((code) => (
                  <option key={code} value={code}>
                    {code}
                  </option>
                ))}
              </select>
            </div>

            <div>
              <label htmlFor={`${prefix}-validFrom`} className={LABEL}>
                Vigente desde
              </label>
              <input
                id={`${prefix}-validFrom`}
                type="date"
                value={validFrom}
                onChange={(event) => setValidFrom(event.target.value)}
                className={FIELD}
              />
            </div>
          </>
        ) : null}

        {coverage === "private" ? (
          <div>
            <label htmlFor={`${prefix}-session`} className={LABEL}>
              Valor de la sesión
            </label>
            <input
              id={`${prefix}-session`}
              value={patientAmount}
              onChange={(event) => setPatientAmount(event.target.value)}
              inputMode="decimal"
              placeholder="15000,00"
              className={`${FIELD} font-mono`}
            />
          </div>
        ) : null}

        {coverage === "insurer" ? (
          <>
            <div>
              <label htmlFor={`${prefix}-insurerAmount`} className={LABEL}>
                Lo que paga la mutual
              </label>
              <input
                id={`${prefix}-insurerAmount`}
                value={insurerAmount}
                onChange={(event) => setInsurerAmount(event.target.value)}
                inputMode="decimal"
                placeholder="8000,00"
                className={`${FIELD} font-mono`}
              />
            </div>
            <div>
              <label htmlFor={`${prefix}-copago`} className={LABEL}>
                Copago del paciente
              </label>
              <input
                id={`${prefix}-copago`}
                value={patientAmount}
                onChange={(event) => setPatientAmount(event.target.value)}
                inputMode="decimal"
                placeholder="1500,00"
                className={`${FIELD} font-mono`}
              />
              <p className="mt-1 text-xs text-ink-muted">Escribí 0 si la mutual cubre toda la sesión.</p>
            </div>
          </>
        ) : null}
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
          {isSubmitting ? "Guardando…" : isEditing ? "Guardar importes" : "Cargar valor"}
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
