"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

import { canBookWith } from "@/entities/care/model";
import type { Patient } from "@/entities/patient/model";
import {
  addDays,
  dateTimeLocalToRFC3339,
  formatDayHeading,
  formatSlotTime,
  startOfWeek,
  toDateTimeLocalValue,
  toISODate,
  weekdayLabel,
  type Appointment,
} from "@/entities/schedule/model";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import { getSessionUser } from "@/shared/auth/session";

const DEFAULT_ZONE = "America/Argentina/Buenos_Aires";

/**
 * Weekly agenda of one professional.
 *
 * The window is Monday–Sunday in the practice timezone. Appointments are stored
 * as UTC instants; this screen is where they become civil times again.
 */
export function WeekCalendar({ timeZone }: { timeZone: string }) {
  const zone = timeZone || DEFAULT_ZONE;
  const viewer = getSessionUser();
  const professionalId = viewer?.id ?? "";
  const role = viewer?.role;

  const [anchor, setAnchor] = useState(() => new Date());
  const [appointments, setAppointments] = useState<Appointment[] | null>(null);
  const [patients, setPatients] = useState<Patient[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<Appointment | null>(null);

  const weekStart = useMemo(() => startOfWeek(anchor, zone), [anchor, zone]);
  const weekEnd = useMemo(() => addDays(weekStart, 7), [weekStart]);
  const days = useMemo(() => Array.from({ length: 7 }, (_, index) => addDays(weekStart, index)), [weekStart]);

  const load = useCallback(async () => {
    setError(null);
    const { data, error, response } = await browserApi.GET("/api/v1/appointments", {
      params: {
        query: {
          from: weekStart.toISOString(),
          to: weekEnd.toISOString(),
          professionalId: professionalId || undefined,
        },
      },
    });
    if (!response.ok) {
      setError(apiErrorMessage(error));
      return;
    }
    setAppointments(data?.items ?? []);
  }, [weekStart, weekEnd, professionalId]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    void (async () => {
      const { data } = await browserApi.GET("/api/v1/patients", {
        params: { query: { activeOnly: true, limit: 50, offset: 0 } },
      });
      setPatients(data?.items ?? []);
    })();
  }, []);

  const byDay = useMemo(() => {
    const buckets: Appointment[][] = Array.from({ length: 7 }, () => []);
    for (const slot of appointments ?? []) {
      const local = new Date(slot.startsAt);
      for (let index = 0; index < 7; index += 1) {
        const dayStart = days[index];
        if (!dayStart) continue;
        const dayEnd = days[index + 1] ?? weekEnd;
        if (local >= dayStart && local < dayEnd) {
          buckets[index]?.push(slot);
          break;
        }
      }
    }
    return buckets;
  }, [appointments, days, weekEnd]);

  function patientName(id: string): string {
    return patients.find((patient) => patient.id === id)?.fullName ?? "Paciente";
  }

  // A professional books only with the patients it treats (T38). The roster it reads can
  // also carry granted patients, which it may read but not book, so they are left out here.
  const bookable = useMemo(
    () => patients.filter((patient) => canBookWith(role, patient, professionalId)),
    [patients, role, professionalId],
  );
  const bookingNotice =
    role === "professional" && bookable.length === 0
      ? "No tenés pacientes a tu cargo para agendar. Un paciente queda a tu cargo cuando lo registrás vos o el titular te lo asigna."
      : null;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => setAnchor(addDays(weekStart, -7))}
            className="rounded-md border border-border px-3 py-1.5 text-xs font-medium"
          >
            Semana anterior
          </button>
          <button
            type="button"
            onClick={() => setAnchor(new Date())}
            className="rounded-md border border-border px-3 py-1.5 text-xs font-medium"
          >
            Hoy
          </button>
          <button
            type="button"
            onClick={() => setAnchor(addDays(weekStart, 7))}
            className="rounded-md border border-border px-3 py-1.5 text-xs font-medium"
          >
            Semana siguiente
          </button>
        </div>
        <p className="text-sm text-ink-muted">
          {formatDayHeading(weekStart, zone)} — {formatDayHeading(addDays(weekStart, 6), zone)}
        </p>
      </div>

      {error ? (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      ) : null}

      <BookingForms
        professionalId={professionalId}
        patients={bookable}
        notice={bookingNotice}
        zone={zone}
        onCreated={() => void load()}
        onError={setError}
      />

      <div className="grid gap-3 md:grid-cols-7">
        {days.map((day, index) => (
          <section key={day.toISOString()} className="min-h-48 rounded-[var(--radius-panel)] border border-border bg-surface p-3">
            <h2 className="text-xs font-semibold uppercase tracking-wide text-ink-muted">
              {formatDayHeading(day, zone)}
            </h2>
            <ul className="mt-2 space-y-2">
              {(byDay[index] ?? []).length === 0 ? (
                <li className="text-xs text-ink-muted">Sin turnos</li>
              ) : (
                (byDay[index] ?? []).map((slot) => (
                  <li key={slot.id}>
                    <button
                      type="button"
                      onClick={() => setSelected(slot)}
                      className={`w-full rounded-md border px-2 py-2 text-left text-xs ${
                        slot.status === "cancelled"
                          ? "border-border bg-surface-muted text-ink-muted line-through"
                          : slot.status === "attended"
                            ? "border-success/30 bg-success/5"
                            : "border-brand/30 bg-brand/5"
                      }`}
                    >
                      <span className="block font-medium">
                        {formatSlotTime(slot.startsAt, zone)}–{formatSlotTime(slot.endsAt, zone)}
                      </span>
                      <span className="block">{patientName(slot.patientId)}</span>
                    </button>
                  </li>
                ))
              )}
            </ul>
          </section>
        ))}
      </div>

      {selected ? (
        <AppointmentActions
          appointment={selected}
          patientName={patientName(selected.patientId)}
          zone={zone}
          onClose={() => setSelected(null)}
          onChanged={() => {
            setSelected(null);
            void load();
          }}
          onError={setError}
        />
      ) : null}
    </div>
  );
}

function BookingForms({
  professionalId,
  patients,
  notice,
  zone,
  onCreated,
  onError,
}: {
  professionalId: string;
  patients: Patient[];
  notice: string | null;
  zone: string;
  onCreated: () => void;
  onError: (message: string | null) => void;
}) {
  const [patientId, setPatientId] = useState("");
  const [startsAt, setStartsAt] = useState(() => toDateTimeLocalValue(new Date()));
  const [duration, setDuration] = useState(45);
  const [seriesUntil, setSeriesUntil] = useState("");
  const [busy, setBusy] = useState(false);

  async function bookOne(event: React.FormEvent) {
    event.preventDefault();
    onError(null);
    const start = dateTimeLocalToRFC3339(startsAt);
    if (!start || patientId === "") {
      onError("Elegí un paciente y una hora de inicio.");
      return;
    }
    const ends = new Date(new Date(start).getTime() + duration * 60_000).toISOString();
    setBusy(true);
    try {
      const result = await browserApi.POST("/api/v1/appointments", {
        body: { patientId, professionalUserId: professionalId, startsAt: start, endsAt: ends },
      });
      if (result.error || !result.data) {
        onError(apiErrorMessage(result.error));
        return;
      }
      onCreated();
    } catch {
      onError("No pudimos contactar al servidor.");
    } finally {
      setBusy(false);
    }
  }

  async function bookSeries(event: React.FormEvent) {
    event.preventDefault();
    onError(null);
    const start = dateTimeLocalToRFC3339(startsAt);
    if (!start || patientId === "" || seriesUntil === "") {
      onError("La serie necesita paciente, hora de inicio y fecha de fin.");
      return;
    }
    const startDate = new Date(start);
    const localMinutes = startDate.getHours() * 60 + startDate.getMinutes();
    setBusy(true);
    try {
      const result = await browserApi.POST("/api/v1/appointment-series", {
        body: {
          patientId,
          professionalUserId: professionalId,
          weekday: startDate.getDay(),
          localStartMinute: localMinutes,
          durationMinutes: duration,
          startsOn: toISODate(startDate, zone),
          untilOn: seriesUntil,
        },
      });
      if (result.error || !result.data) {
        onError(apiErrorMessage(result.error));
        return;
      }
      onCreated();
    } catch {
      onError("No pudimos contactar al servidor.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="grid gap-4 rounded-[var(--radius-panel)] border border-border bg-surface p-4 lg:grid-cols-2">
      <form onSubmit={(event) => void bookOne(event)} className="space-y-3">
        <h2 className="text-sm font-semibold">Nuevo turno</h2>
        {notice ? <p className="text-xs text-ink-muted">{notice}</p> : null}
        <PatientSelect patients={patients} value={patientId} onChange={setPatientId} />
        <label className="block text-xs font-medium text-ink-muted">
          Inicio
          <input
            type="datetime-local"
            value={startsAt}
            onChange={(event) => setStartsAt(event.target.value)}
            className="mt-1 w-full rounded-md border border-border px-3 py-2 text-sm"
          />
        </label>
        <DurationField value={duration} onChange={setDuration} />
        <button
          type="submit"
          disabled={busy}
          className="rounded-md bg-brand px-4 py-2 text-sm font-medium text-brand-ink disabled:opacity-60"
        >
          {busy ? "Guardando…" : "Reservar"}
        </button>
      </form>

      <form onSubmit={(event) => void bookSeries(event)} className="space-y-3">
        <h2 className="text-sm font-semibold">Serie semanal</h2>
        <p className="text-xs text-ink-muted">
          Se emiten turnos concretos cada {weekdayLabel(new Date(startsAt).getDay())} a la misma hora
          local, hasta la fecha de fin. Un turno ya atendido no se reescribe.
        </p>
        <label className="block text-xs font-medium text-ink-muted">
          Hasta
          <input
            type="date"
            value={seriesUntil}
            onChange={(event) => setSeriesUntil(event.target.value)}
            className="mt-1 w-full rounded-md border border-border px-3 py-2 text-sm"
          />
        </label>
        <button
          type="submit"
          disabled={busy}
          className="rounded-md border border-border px-4 py-2 text-sm font-medium disabled:opacity-60"
        >
          {busy ? "Guardando…" : "Crear serie"}
        </button>
      </form>
    </div>
  );
}

function AppointmentActions({
  appointment,
  patientName,
  zone,
  onClose,
  onChanged,
  onError,
}: {
  appointment: Appointment;
  patientName: string;
  zone: string;
  onClose: () => void;
  onChanged: () => void;
  onError: (message: string | null) => void;
}) {
  const [busy, setBusy] = useState(false);
  const scheduled = appointment.status === "scheduled";

  async function transition(path: "attend" | "cancel") {
    onError(null);
    setBusy(true);
    try {
      const result =
        path === "attend"
          ? await browserApi.POST("/api/v1/appointments/{id}/attend", {
              params: { path: { id: appointment.id ?? "" } },
            })
          : await browserApi.POST("/api/v1/appointments/{id}/cancel", {
              params: { path: { id: appointment.id ?? "" } },
            });
      if (result.error || !result.data) {
        onError(apiErrorMessage(result.error));
        return;
      }
      onChanged();
    } catch {
      onError("No pudimos contactar al servidor.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <aside className="rounded-[var(--radius-panel)] border border-border bg-surface p-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">{patientName}</h2>
          <p className="text-xs text-ink-muted">
            {formatSlotTime(appointment.startsAt, zone)}–{formatSlotTime(appointment.endsAt, zone)} ·{" "}
            {appointment.status}
          </p>
        </div>
        <button type="button" onClick={onClose} className="text-xs text-ink-muted" aria-label="Cerrar">
          ✕
        </button>
      </div>
      {scheduled ? (
        <div className="mt-3 flex gap-2">
          <button
            type="button"
            disabled={busy}
            onClick={() => void transition("attend")}
            className="rounded-md bg-brand px-3 py-1.5 text-xs font-medium text-brand-ink disabled:opacity-60"
          >
            Marcar atendido
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() => void transition("cancel")}
            className="rounded-md border border-border px-3 py-1.5 text-xs font-medium disabled:opacity-60"
          >
            Cancelar turno
          </button>
        </div>
      ) : null}
    </aside>
  );
}

function PatientSelect({
  patients,
  value,
  onChange,
}: {
  patients: Patient[];
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <label className="block text-xs font-medium text-ink-muted">
      Paciente
      <select
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className="mt-1 w-full rounded-md border border-border px-3 py-2 text-sm"
        required
      >
        <option value="">Elegí un paciente</option>
        {patients.map((patient) => (
          <option key={patient.id} value={patient.id}>
            {patient.fullName}
          </option>
        ))}
      </select>
    </label>
  );
}

function DurationField({ value, onChange }: { value: number; onChange: (value: number) => void }) {
  return (
    <label className="block text-xs font-medium text-ink-muted">
      Duración (minutos)
      <input
        type="number"
        min={5}
        max={480}
        value={value}
        onChange={(event) => onChange(Number(event.target.value))}
        className="mt-1 w-full rounded-md border border-border px-3 py-2 text-sm"
      />
    </label>
  );
}
