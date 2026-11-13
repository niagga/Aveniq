"use client";

import { useCallback, useEffect, useId, useState } from "react";

import {
  CLINICAL_NOTES_PAGE_SIZE,
  dateTimeLocalToRFC3339,
  formatEncounteredAt,
  toDateTimeLocalValue,
  type ClinicalNote,
  type ClinicalRecord,
} from "@/entities/clinical/model";
import { apiErrorMessage, browserApi } from "@/shared/api/client";

/**
 * Historia clínica of one patient: the treatment frame and the encounters.
 *
 * Lives under the patient it belongs to, matching the API. The screen does not
 * send an author: the token is the author, and a body field would let a client
 * sign a note as somebody else. Notes are append-only; a correction is a new note.
 *
 * This panel is mounted only when the professional opens the clinical tab, so
 * looking at a phone number does not write a clinical.read audit event.
 */
export function ClinicalPanel({ patientID }: { patientID: string }) {
  const [record, setRecord] = useState<ClinicalRecord | null>(null);
  const [notes, setNotes] = useState<ClinicalNote[] | null>(null);
  const [hasMoreNotes, setHasMoreNotes] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  const load = useCallback(async () => {
    setError(null);
    setIsLoading(true);
    try {
      const [recordResult, notesResult] = await Promise.all([
        browserApi.GET("/api/v1/patients/{id}/clinical-record", {
          params: { path: { id: patientID } },
        }),
        browserApi.GET("/api/v1/patients/{id}/clinical-notes", {
          params: { path: { id: patientID }, query: { limit: CLINICAL_NOTES_PAGE_SIZE, offset: 0 } },
        }),
      ]);

      if (!notesResult.response.ok) {
        setError(apiErrorMessage(notesResult.error));
        return;
      }

      // No frame yet is a first visit, not a failure: the form starts empty and
      // the first save creates the row. Any other status is a real problem.
      if (recordResult.response.status === 404) {
        setRecord({ chiefComplaint: "", background: "", objectives: "" });
      } else if (!recordResult.response.ok || !recordResult.data) {
        setError(apiErrorMessage(recordResult.error));
        return;
      } else {
        setRecord(recordResult.data);
      }

      const items = notesResult.data?.items ?? [];
      setNotes(items);
      setHasMoreNotes(items.length === CLINICAL_NOTES_PAGE_SIZE);
    } catch {
      setError("No pudimos contactar al servidor.");
    } finally {
      setIsLoading(false);
    }
  }, [patientID]);

  useEffect(() => {
    void load();
  }, [load]);

  async function loadMore() {
    if (notes === null) return;
    setError(null);
    try {
      const result = await browserApi.GET("/api/v1/patients/{id}/clinical-notes", {
        params: {
          path: { id: patientID },
          query: { limit: CLINICAL_NOTES_PAGE_SIZE, offset: notes.length },
        },
      });
      if (!result.response.ok) {
        setError(apiErrorMessage(result.error));
        return;
      }
      const items = result.data?.items ?? [];
      setNotes((current) => [...(current ?? []), ...items]);
      setHasMoreNotes(items.length === CLINICAL_NOTES_PAGE_SIZE);
    } catch {
      setError("No pudimos contactar al servidor.");
    }
  }

  if (isLoading) {
    return (
      <p role="status" className="mt-4 text-sm text-ink-muted">
        Cargando historia clínica…
      </p>
    );
  }

  return (
    <div className="mt-5 space-y-8">
      {error ? (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      ) : null}

      {record ? (
        <RecordForm
          patientID={patientID}
          record={record}
          onSaved={setRecord}
          onError={setError}
        />
      ) : null}

      {notes ? (
        <NotesSection
          patientID={patientID}
          notes={notes}
          hasMore={hasMoreNotes}
          onAdded={(note) => {
            setNotes((current) => [note, ...(current ?? [])]);
          }}
          onLoadMore={() => void loadMore()}
          onError={setError}
        />
      ) : null}
    </div>
  );
}

function RecordForm({
  patientID,
  record,
  onSaved,
  onError,
}: {
  patientID: string;
  record: ClinicalRecord;
  onSaved: (record: ClinicalRecord) => void;
  onError: (message: string | null) => void;
}) {
  const prefix = useId();
  const [chiefComplaint, setChiefComplaint] = useState(record.chiefComplaint ?? "");
  const [background, setBackground] = useState(record.background ?? "");
  const [objectives, setObjectives] = useState(record.objectives ?? "");
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    setChiefComplaint(record.chiefComplaint ?? "");
    setBackground(record.background ?? "");
    setObjectives(record.objectives ?? "");
  }, [record]);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    onError(null);
    setIsSaving(true);
    try {
      const result = await browserApi.PUT("/api/v1/patients/{id}/clinical-record", {
        params: { path: { id: patientID } },
        body: {
          chiefComplaint: chiefComplaint.trim(),
          background: background.trim(),
          objectives: objectives.trim(),
        },
      });
      if (result.error || !result.data) {
        onError(apiErrorMessage(result.error));
        return;
      }
      onSaved(result.data);
    } catch {
      onError("No pudimos contactar al servidor.");
    } finally {
      setIsSaving(false);
    }
  }

  return (
    <form onSubmit={(event) => void handleSubmit(event)} className="space-y-3">
      <div>
        <h3 className="text-sm font-semibold">Ficha clínica</h3>
        <p className="mt-0.5 text-xs text-ink-muted">
          El marco del tratamiento. Se crea al primer guardado y se reemplaza entero
          cada vez: lo que ves es lo que se guarda.
        </p>
      </div>

      <Field id={`${prefix}-complaint`} label="Motivo de consulta">
        <textarea
          id={`${prefix}-complaint`}
          value={chiefComplaint}
          onChange={(event) => setChiefComplaint(event.target.value)}
          maxLength={4000}
          rows={3}
          className="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-brand"
        />
      </Field>
      <Field id={`${prefix}-background`} label="Antecedentes">
        <textarea
          id={`${prefix}-background`}
          value={background}
          onChange={(event) => setBackground(event.target.value)}
          maxLength={8000}
          rows={4}
          className="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-brand"
        />
      </Field>
      <Field id={`${prefix}-objectives`} label="Objetivos">
        <textarea
          id={`${prefix}-objectives`}
          value={objectives}
          onChange={(event) => setObjectives(event.target.value)}
          maxLength={4000}
          rows={3}
          className="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-brand"
        />
      </Field>

      <div className="flex flex-wrap items-center gap-3">
        <button
          type="submit"
          disabled={isSaving}
          className="rounded-md bg-brand px-4 py-2 text-sm font-medium text-brand-ink disabled:opacity-60"
        >
          {isSaving ? "Guardando…" : "Guardar ficha"}
        </button>
        {record.updatedAt ? (
          <p className="text-xs text-ink-muted">
            Última escritura {formatEncounteredAt(record.updatedAt)}
          </p>
        ) : (
          <p className="text-xs text-ink-muted">Todavía no hay ficha guardada.</p>
        )}
      </div>
    </form>
  );
}

function NotesSection({
  patientID,
  notes,
  hasMore,
  onAdded,
  onLoadMore,
  onError,
}: {
  patientID: string;
  notes: ClinicalNote[];
  hasMore: boolean;
  onAdded: (note: ClinicalNote) => void;
  onLoadMore: () => void;
  onError: (message: string | null) => void;
}) {
  const prefix = useId();
  const [encounteredAt, setEncounteredAt] = useState(() => toDateTimeLocalValue(new Date()));
  const [content, setContent] = useState("");
  const [isSaving, setIsSaving] = useState(false);

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    onError(null);

    const trimmed = content.trim();
    if (trimmed === "") {
      onError("La evolución no puede estar vacía.");
      return;
    }

    const timestamp = dateTimeLocalToRFC3339(encounteredAt);
    if (timestamp === null) {
      onError("La fecha del encuentro no es válida.");
      return;
    }
    if (new Date(timestamp).getTime() > Date.now()) {
      onError("La fecha del encuentro no puede ser futura.");
      return;
    }

    setIsSaving(true);
    try {
      const result = await browserApi.POST("/api/v1/patients/{id}/clinical-notes", {
        params: { path: { id: patientID } },
        body: { encounteredAt: timestamp, content: trimmed },
      });
      if (result.error || !result.data) {
        onError(apiErrorMessage(result.error));
        return;
      }
      onAdded(result.data);
      setContent("");
      setEncounteredAt(toDateTimeLocalValue(new Date()));
    } catch {
      onError("No pudimos contactar al servidor.");
    } finally {
      setIsSaving(false);
    }
  }

  return (
    <section className="space-y-4 border-t border-border pt-6">
      <div>
        <h3 className="text-sm font-semibold">Evoluciones</h3>
        <p className="mt-0.5 text-xs text-ink-muted">
          Un encuentro, una nota. No se editan: una corrección es una nota nueva. La
          autoría la toma la sesión, no este formulario.
        </p>
      </div>

      <form onSubmit={(event) => void handleSubmit(event)} className="space-y-3">
        <Field id={`${prefix}-when`} label="Fecha del encuentro">
          <input
            id={`${prefix}-when`}
            type="datetime-local"
            value={encounteredAt}
            max={toDateTimeLocalValue(new Date())}
            onChange={(event) => setEncounteredAt(event.target.value)}
            required
            className="mt-1 rounded-md border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-brand"
          />
        </Field>
        <Field id={`${prefix}-content`} label="Nota">
          <textarea
            id={`${prefix}-content`}
            value={content}
            onChange={(event) => setContent(event.target.value)}
            maxLength={20000}
            rows={5}
            required
            className="mt-1 w-full rounded-md border border-border bg-surface px-3 py-2 text-sm outline-none focus:border-brand"
          />
        </Field>
        <button
          type="submit"
          disabled={isSaving}
          className="rounded-md border border-border px-4 py-2 text-sm font-medium disabled:opacity-60"
        >
          {isSaving ? "Registrando…" : "Registrar evolución"}
        </button>
      </form>

      {notes.length === 0 ? (
        <p className="text-sm text-ink-muted">Todavía no hay evoluciones.</p>
      ) : (
        <ol className="space-y-3">
          {notes.map((note) => (
            <li
              key={note.id ?? `${note.encounteredAt}-${note.content.slice(0, 24)}`}
              className="rounded-md border border-border bg-surface-muted px-3 py-3"
            >
              <p className="text-xs font-medium text-ink-muted">
                {formatEncounteredAt(note.encounteredAt)}
              </p>
              <p className="mt-1 whitespace-pre-wrap text-sm">{note.content}</p>
            </li>
          ))}
        </ol>
      )}

      {hasMore ? (
        <button
          type="button"
          onClick={onLoadMore}
          className="text-xs font-medium text-ink-muted underline"
        >
          Ver anteriores
        </button>
      ) : null}
    </section>
  );
}

function Field({
  id,
  label,
  children,
}: {
  id: string;
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <label htmlFor={id} className="block text-xs font-medium text-ink-muted">
        {label}
      </label>
      {children}
    </div>
  );
}
