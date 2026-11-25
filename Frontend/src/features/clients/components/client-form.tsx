"use client";

import { useEffect, useId, useState } from "react";

import type { Client, ClientProfile } from "@/entities/client/model";
import {
  COUNTRIES,
  CURRENCIES,
  FISCAL_CONDITIONS_BY_COUNTRY,
  TAX_ID_KINDS_BY_COUNTRY,
  currencyLabel,
  fiscalConditionLabel,
  taxIDKindLabel,
} from "@/entities/patient/model";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { inputClass } from "@/shared/ui/field";
import { FormModal } from "@/shared/ui/form-modal";

/**
 * Create and edit a billing client in a popup (T74). The API takes the same profile for both,
 * so the fiscal data the receipts freeze is typed in one place and not on the patient.
 */
export function ClientFormModal({
  open,
  client,
  onClose,
  onSaved,
}: {
  open: boolean;
  client: Client | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [displayName, setDisplayName] = useState("");
  const [legalName, setLegalName] = useState("");
  const [country, setCountry] = useState("AR");
  const [taxIdKind, setTaxIdKind] = useState("");
  const [taxIdNumber, setTaxIdNumber] = useState("");
  const [fiscalCondition, setFiscalCondition] = useState("");
  const [fiscalAddress, setFiscalAddress] = useState("");
  const [billingCurrency, setBillingCurrency] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const writesLocked = useWritesLocked();

  const prefix = useId();
  const isEditing = client !== null;

  useEffect(() => {
    if (!open) return;
    setDisplayName(client?.displayName ?? "");
    setLegalName(client?.legalName ?? "");
    setCountry(client?.country ?? "AR");
    setTaxIdKind(client?.taxIdKind ?? "");
    setTaxIdNumber(client?.taxIdNumber ?? "");
    setFiscalCondition(client?.fiscalCondition ?? "");
    setFiscalAddress(client?.fiscalAddress ?? "");
    setBillingCurrency(client?.billingCurrency ?? "");
    setError(null);
  }, [open, client]);

  const taxIdKinds = TAX_ID_KINDS_BY_COUNTRY[country] ?? [];
  const fiscalConditions = FISCAL_CONDITIONS_BY_COUNTRY[country] ?? [];

  /** A catalog that changes with the country cannot keep a value the new country does not have. */
  function changeCountry(next: string) {
    setCountry(next);
    setTaxIdKind((current) =>
      (TAX_ID_KINDS_BY_COUNTRY[next] ?? []).includes(current) ? current : "",
    );
    setFiscalCondition((current) =>
      (FISCAL_CONDITIONS_BY_COUNTRY[next] ?? []).includes(current) ? current : "",
    );
  }

  async function submit() {
    if (writesLocked) return;
    setError(null);

    if (displayName.trim() === "") {
      setError("El nombre es obligatorio.");
      return;
    }
    if (billingCurrency === "") {
      setError("Elegí la moneda de facturación: la elige una persona, no se deduce del país.");
      return;
    }

    const body: ClientProfile = {
      displayName: displayName.trim(),
      legalName: legalName.trim(),
      country,
      taxIdKind,
      taxIdNumber: taxIdNumber.trim(),
      fiscalCondition,
      fiscalAddress: fiscalAddress.trim(),
      billingCurrency: billingCurrency as ClientProfile["billingCurrency"],
    };

    setBusy(true);
    try {
      const result = isEditing
        ? await browserApi.PATCH("/api/v1/clients/{id}", {
            params: { path: { id: client.id } },
            body,
          })
        : await browserApi.POST("/api/v1/clients", { body });

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
      title={isEditing ? "Editar cliente" : "Nuevo cliente"}
      description="Quien paga las sesiones. Puede ser el propio paciente o un tercero: la mutual, la familia, una empresa."
      submitLabel={isEditing ? "Guardar cambios" : "Crear cliente"}
      busy={busy}
      error={error}
      onClose={onClose}
      onSubmit={submit}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <Labeled id={`${prefix}-displayName`} label="Nombre" className="sm:col-span-2">
          <input
            id={`${prefix}-displayName`}
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
            required
            maxLength={160}
            placeholder="Juan Pérez"
            disabled={writesLocked}
            className={inputClass}
          />
        </Labeled>

        <Labeled id={`${prefix}-legalName`} label="Razón social (opcional)" className="sm:col-span-2">
          <input
            id={`${prefix}-legalName`}
            value={legalName}
            onChange={(event) => setLegalName(event.target.value)}
            maxLength={160}
            placeholder="Pérez Juan Ignacio"
            disabled={writesLocked}
            className={inputClass}
          />
        </Labeled>

        <Labeled id={`${prefix}-country`} label="País">
          <select
            id={`${prefix}-country`}
            value={country}
            onChange={(event) => changeCountry(event.target.value)}
            disabled={writesLocked}
            className={inputClass}
          >
            {COUNTRIES.map((option) => (
              <option key={option.code} value={option.code}>
                {option.label}
              </option>
            ))}
          </select>
        </Labeled>

        <Labeled id={`${prefix}-currency`} label="Moneda de facturación">
          <select
            id={`${prefix}-currency`}
            value={billingCurrency}
            onChange={(event) => setBillingCurrency(event.target.value)}
            required
            disabled={writesLocked}
            className={inputClass}
          >
            <option value="">Elegir…</option>
            {CURRENCIES.map((code) => (
              <option key={code} value={code}>
                {currencyLabel(code)}
              </option>
            ))}
          </select>
        </Labeled>

        <Labeled id={`${prefix}-taxIdKind`} label="Tipo de documento fiscal">
          <select
            id={`${prefix}-taxIdKind`}
            value={taxIdKind}
            onChange={(event) => setTaxIdKind(event.target.value)}
            disabled={writesLocked}
            className={inputClass}
          >
            <option value="">Sin documento fiscal</option>
            {taxIdKinds.map((kind) => (
              <option key={kind} value={kind}>
                {taxIDKindLabel(kind)}
              </option>
            ))}
          </select>
        </Labeled>

        <Labeled id={`${prefix}-taxIdNumber`} label="Número de documento fiscal">
          <input
            id={`${prefix}-taxIdNumber`}
            value={taxIdNumber}
            onChange={(event) => setTaxIdNumber(event.target.value)}
            maxLength={20}
            placeholder="20-12345678-3"
            disabled={writesLocked}
            className={inputClass}
          />
        </Labeled>

        <Labeled id={`${prefix}-fiscalCondition`} label="Condición fiscal">
          <select
            id={`${prefix}-fiscalCondition`}
            value={fiscalCondition}
            onChange={(event) => setFiscalCondition(event.target.value)}
            disabled={writesLocked}
            className={inputClass}
          >
            <option value="">Sin condición fiscal</option>
            {fiscalConditions.map((condition) => (
              <option key={condition} value={condition}>
                {fiscalConditionLabel(condition)}
              </option>
            ))}
          </select>
        </Labeled>

        <Labeled id={`${prefix}-fiscalAddress`} label="Dirección fiscal">
          <input
            id={`${prefix}-fiscalAddress`}
            value={fiscalAddress}
            onChange={(event) => setFiscalAddress(event.target.value)}
            maxLength={240}
            placeholder="Av. Siempre Viva 742"
            disabled={writesLocked}
            className={inputClass}
          />
        </Labeled>
      </div>
    </FormModal>
  );
}

function Labeled({
  id,
  label,
  className,
  children,
}: {
  id: string;
  label: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <div className={className}>
      <label htmlFor={id} className="block text-xs font-medium text-ink-muted">
        {label}
      </label>
      {children}
    </div>
  );
}
