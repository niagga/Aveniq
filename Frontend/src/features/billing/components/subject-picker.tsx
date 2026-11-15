"use client";

import { useEffect, useId, useState } from "react";

import { SUBJECT_SEARCH_MIN, type Subject } from "@/entities/billing/model";
import { apiErrorMessage, browserApi, CONNECTION_ERROR_MESSAGE } from "@/shared/api/client";
import { Field, inputClass } from "@/shared/ui/field";

/** Wait this long after the last keystroke before asking the API. */
const SEARCH_DELAY_MS = 300;

/**
 * Picks the patient a voucher is for, from the billing subject search.
 *
 * The search is not a roster: it needs a fragment of at least three characters and answers
 * at most twenty matches, so the list is never browsed, only found. The patient is optional
 * on a voucher, so choosing none is a valid state and the screen says what it means.
 */
export function SubjectPicker({
  value,
  onChange,
}: {
  value: Subject | null;
  onChange: (subject: Subject | null) => void;
}) {
  const prefix = useId();
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Subject[]>([]);
  const [failure, setFailure] = useState<string | null>(null);
  const [searching, setSearching] = useState(false);

  const fragment = query.trim();
  const searchable = value === null && fragment.length >= SUBJECT_SEARCH_MIN;

  useEffect(() => {
    if (!searchable) return;
    let active = true;

    const timer = setTimeout(() => {
      void (async () => {
        setSearching(true);
        try {
          const { data, error, response } = await browserApi.GET("/api/v1/billing/subjects", {
            params: { query: { search: fragment } },
          });
          if (!active) return;
          if (!response.ok) {
            setFailure(apiErrorMessage(error));
            setResults([]);
            return;
          }
          setFailure(null);
          setResults(data?.items ?? []);
        } catch {
          if (active) setFailure(CONNECTION_ERROR_MESSAGE);
        } finally {
          if (active) setSearching(false);
        }
      })();
    }, SEARCH_DELAY_MS);

    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [fragment, searchable]);

  function choose(subject: Subject) {
    onChange(subject);
    setQuery("");
    setResults([]);
  }

  if (value !== null) {
    return (
      <div>
        <p className="block text-xs font-medium text-ink-muted">Paciente</p>
        <div className="mt-1 flex flex-wrap items-center justify-between gap-2 rounded-md border border-border bg-surface px-3 py-2 text-sm">
          <span>
            <span className="font-medium">{value.fullName}</span>
            <span className="ml-2 text-xs text-ink-muted">
              {[value.taxId, value.fiscalCondition].filter(Boolean).join(" · ") ||
                "Sin datos fiscales"}
            </span>
          </span>
          <button
            type="button"
            onClick={() => onChange(null)}
            className="rounded-md border border-border px-2 py-1 text-xs font-medium"
          >
            Quitar
          </button>
        </div>
      </div>
    );
  }

  return (
    <div>
      <Field
        label="Paciente (opcional)"
        htmlFor={`${prefix}-search`}
        hint={`Sin paciente es un cobro suelto. Escribí al menos ${SUBJECT_SEARCH_MIN} letras para buscar por nombre, documento o CUIT.`}
      >
        <input
          id={`${prefix}-search`}
          type="search"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          autoComplete="off"
          maxLength={100}
          className={inputClass}
        />
      </Field>

      {failure !== null ? (
        <p role="alert" className="mt-2 text-xs text-danger">
          {failure}
        </p>
      ) : null}

      {searchable ? (
        <ul
          aria-label="Pacientes encontrados"
          className="mt-2 divide-y divide-border rounded-md border border-border bg-surface text-sm"
        >
          {searching && results.length === 0 ? (
            <li role="status" className="px-3 py-2 text-xs text-ink-muted">
              Buscando…
            </li>
          ) : results.length === 0 && failure === null ? (
            <li className="px-3 py-2 text-xs text-ink-muted">
              No hay pacientes activos que coincidan.
            </li>
          ) : (
            results.map((subject) => (
              <li key={subject.patientId}>
                <button
                  type="button"
                  onClick={() => choose(subject)}
                  className="flex w-full flex-wrap items-baseline justify-between gap-2 px-3 py-2 text-left hover:bg-surface-muted"
                >
                  <span className="font-medium">{subject.fullName}</span>
                  <span className="text-xs text-ink-muted">
                    {subject.documentNumber ?? subject.taxId ?? ""}
                  </span>
                </button>
              </li>
            ))
          )}
        </ul>
      ) : null}
    </div>
  );
}
