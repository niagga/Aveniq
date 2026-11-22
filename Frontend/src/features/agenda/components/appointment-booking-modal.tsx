"use client";

import { useEffect, useState } from "react";

import type { Patient } from "@/entities/patient/model";
import {
  dateTimeLocalToRFC3339,
  toDateTimeLocalValue,
  toISODate,
  weekdayLabel,
  type Modality,
} from "@/entities/schedule/model";
import {
  DurationField,
  ModalityField,
  PatientSelect,
  StartsAtField,
} from "@/features/agenda/components/appointment-fields";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { FormModal } from "@/shared/ui/form-modal";
import { Segmented } from "@/shared/ui/segmented";

type BookingMode = "one" | "series";

export function AppointmentBookingModal({
  open,
  professionalId,
  patients,
  notice,
  zone,
  initialStartsAt,
  onClose,
  onCreated,
}: {
  open: boolean;
  professionalId: string;
  patients: Patient[];
  notice: string | null;
  zone: string;
  initialStartsAt: string;
  onClose: () => void;
  onCreated: () => void;
}) {
  const [mode, setMode] = useState<BookingMode>("one");
  const [patientId, setPatientId] = useState("");
  const [startsAt, setStartsAt] = useState(initialStartsAt);
  const [duration, setDuration] = useState(45);
  const [modality, setModality] = useState<Modality | "">("");
  const [seriesUntil, setSeriesUntil] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const writesLocked = useWritesLocked();

  useEffect(() => {
    if (open) {
      setStartsAt(initialStartsAt);
      setError(null);
      setMode("one");
    }
  }, [open, initialStartsAt]);

  async function submit() {
    if (writesLocked) return;
    setError(null);
    const start = dateTimeLocalToRFC3339(startsAt);
    if (!start || patientId === "") {
      setError("Elegí un paciente y una hora de inicio.");
      return;
    }
    if (modality === "") {
      setError("Elegí si el turno es online o presencial.");
      return;
    }

    setBusy(true);
    try {
      if (mode === "one") {
        const ends = new Date(new Date(start).getTime() + duration * 60_000).toISOString();
        const result = await browserApi.POST("/api/v1/appointments", {
          body: {
            patientId,
            professionalUserId: professionalId,
            startsAt: start,
            endsAt: ends,
            modality,
          },
        });
        if (result.error || !result.data) {
          setError(apiErrorMessage(result.error));
          return;
        }
      } else {
        if (seriesUntil === "") {
          setError("La serie necesita una fecha de fin.");
          return;
        }
        const startDate = new Date(start);
        const localMinutes = startDate.getHours() * 60 + startDate.getMinutes();
        const result = await browserApi.POST("/api/v1/appointment-series", {
          body: {
            patientId,
            professionalUserId: professionalId,
            weekday: startDate.getDay(),
            localStartMinute: localMinutes,
            durationMinutes: duration,
            modality,
            startsOn: toISODate(startDate, zone),
            untilOn: seriesUntil,
          },
        });
        if (result.error || !result.data) {
          setError(apiErrorMessage(result.error));
          return;
        }
      }
      onCreated();
      onClose();
    } catch {
      setError("No pudimos contactar al servidor.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <FormModal
      open={open}
      title="Nuevo turno"
      description={notice ?? "Un turno puntual o una serie que repite la misma hora cada semana."}
      size="md"
      submitLabel={mode === "one" ? "Reservar" : "Crear serie"}
      busy={busy}
      error={error}
      onClose={onClose}
      onSubmit={submit}
    >
      <Segmented
        label="Tipo de reserva"
        options={[
          { value: "one", label: "Un turno" },
          { value: "series", label: "Serie semanal" },
        ]}
        value={mode}
        onChange={setMode}
      />

      <PatientSelect
        patients={patients}
        value={patientId}
        onChange={setPatientId}
        disabled={writesLocked}
      />
      <StartsAtField value={startsAt} onChange={setStartsAt} disabled={writesLocked} />
      <DurationField value={duration} onChange={setDuration} disabled={writesLocked} />
      <ModalityField value={modality} onChange={setModality} disabled={writesLocked} />

      {mode === "series" ? (
        <>
          <p className="text-xs text-ink-muted">
            Se crean turnos cada {weekdayLabel(new Date(startsAt).getDay())} a la misma hora local,
            hasta la fecha de fin. Los turnos ya atendidos no se reescriben.
          </p>
          <label className="block text-xs font-medium text-ink-muted">
            Hasta
            <input
              type="date"
              value={seriesUntil}
              onChange={(event) => setSeriesUntil(event.target.value)}
              disabled={writesLocked}
              className="mt-1 w-full rounded-md border border-border px-3 py-2 text-sm disabled:opacity-60"
            />
          </label>
        </>
      ) : null}
    </FormModal>
  );
}