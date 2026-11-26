"use client";

import { useEffect, useState } from "react";

import type { Patient } from "@/entities/patient/model";
import { MODALITY_OPTIONS, type Modality } from "@/entities/schedule/model";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import { inputClass } from "@/shared/ui/field";

/** The search asks for one small page of the roster (T80), not for the whole practice. */
const SEARCH_LIMIT = 20;

/** One request per pause, the same cadence the roster's search box uses. */
const SEARCH_DEBOUNCE_MS = 300;

export function PatientSelect({
  patients,
  value,
  onChange,
  disabled = false,
  canBook = null,
}: {
  patients: Patient[];
  value: string;
  /** Reports the whole patient chosen (T80): the calendar needs the name it searched for, not only the id. */
  onChange: (patient: Patient | null) => void;
  disabled?: boolean;
  /** The bookability rule the screen already applies (T80); a search can never widen it. */
  canBook?: ((patient: Patient) => boolean) | null;
}) {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Patient[] | null>(null);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const text = query.trim();
    if (text === "") {
      // Nothing typed keeps today's behaviour: the list the screen already loads, so a
      // small practice sees the same screen with no search in between.
      setResults(null);
      setSearching(false);
      setSearchError(null);
      return;
    }
    setSearching(true);
    const timer = setTimeout(() => {
      void (async () => {
        const { data, error, response } = await browserApi.GET("/api/v1/patients", {
          params: { query: { search: text, activeOnly: true, limit: SEARCH_LIMIT, offset: 0 } },
        });
        // A newer search (or the popup closing) already took over: its answer wins.
        if (cancelled) return;
        setSearching(false);
        if (!response.ok) {
          setSearchError(apiErrorMessage(error));
          return;
        }
        setSearchError(null);
        setResults((data?.items ?? []).filter(canBook ?? (() => true)));
      })();
    }, SEARCH_DEBOUNCE_MS);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [query, canBook]);

  // The search only narrows the options; it never touches the choice, so a selected
  // patient (an appointment being booked in steps) survives every keystroke. The chosen one
  // is kept here rather than derived from the list: a patient picked from the search results
  // is not in the preloaded page, so a select whose value is missing from its options would
  // show the placeholder while the form still submits that patient — it looks empty and books
  // for someone the person can no longer see.
  const [chosen, setChosen] = useState<Patient | null>(null);
  useEffect(() => {
    if (value === "") setChosen(null);
  }, [value]);

  const base = results ?? patients;
  const options = chosen !== null && !base.some((patient) => patient.id === chosen.id)
    ? [chosen, ...base]
    : base;
  const searched = query.trim() !== "";
  return (
    <>
      <label className="block text-xs font-medium text-ink-muted">
        Buscar paciente
        <input
          type="search"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          disabled={disabled}
          placeholder="Nombre o documento"
          className={`${inputClass} mt-1`}
        />
      </label>
      <label className="block text-xs font-medium text-ink-muted">
        Paciente
        <select
          value={value}
          onChange={(event) => {
            const next =
              event.target.value === ""
                ? null
                : options.find((patient) => patient.id === event.target.value) ?? null;
            setChosen(next);
            onChange(next);
          }}
          disabled={disabled}
          className={`${inputClass} mt-1`}
          required
        >
          <option value="">Elegí un paciente</option>
          {options.map((patient) => (
            <option key={patient.id} value={patient.id}>
              {patient.fullName}
            </option>
          ))}
        </select>
      </label>
      {searched && !searching && searchError === null && options.length === 0 ? (
        <p aria-live="polite" className="text-xs text-ink-muted">
          Ningún paciente activo coincide con la búsqueda.
        </p>
      ) : null}
      {searched && searchError ? (
        <p role="alert" className="text-xs text-danger">
          {searchError}
        </p>
      ) : null}
    </>
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