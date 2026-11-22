"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

import { canBookWith } from "@/entities/care/model";
import type { Patient } from "@/entities/patient/model";
import {
  addDays,
  formatDayHeading,
  formatSlotTime,
  modalityLabel,
  morningDateTimeLocal,
  startOfWeek,
  toDateTimeLocalValue,
  type Appointment,
} from "@/entities/schedule/model";
import { AppointmentBookingModal } from "@/features/agenda/components/appointment-booking-modal";
import { AppointmentDetailModal } from "@/features/agenda/components/appointment-detail-modal";
import { loadStaff, type StaffMember } from "@/features/patients/staff";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import { getSessionUser } from "@/shared/auth/session";
import { Button } from "@/shared/ui/button";
import { FilterBar } from "@/shared/ui/filter-bar";
import { LoadingState } from "@/shared/ui/states";

const DEFAULT_ZONE = "America/Argentina/Buenos_Aires";

/**
 * Weekly agenda (T68): filter bar always visible, board of the week, booking and detail in popups.
 */
export function WeekCalendar({ timeZone }: { timeZone: string }) {
  const zone = timeZone || DEFAULT_ZONE;
  const viewer = getSessionUser();
  const viewerId = viewer?.id ?? "";
  const role = viewer?.role;
  const seesEveryone = role !== "professional";

  const [anchor, setAnchor] = useState(() => new Date());
  const [professionalFilter, setProfessionalFilter] = useState<string>(() =>
    seesEveryone ? "" : viewerId,
  );
  const [staff, setStaff] = useState<StaffMember[]>([]);
  const [appointments, setAppointments] = useState<Appointment[] | null>(null);
  const [patients, setPatients] = useState<Patient[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [selected, setSelected] = useState<Appointment | null>(null);
  const [bookingOpen, setBookingOpen] = useState(false);
  const [bookingStartsAt, setBookingStartsAt] = useState(() => toDateTimeLocalValue(new Date()));

  const professionals = useMemo(
    () => staff.filter((member) => member.role === "professional"),
    [staff],
  );

  const bookingProfessional =
    professionalFilter !== "" ? professionalFilter : professionals[0]?.id ?? viewerId;

  const weekStart = useMemo(() => startOfWeek(anchor, zone), [anchor, zone]);
  const weekEnd = useMemo(() => addDays(weekStart, 7), [weekStart]);
  const days = useMemo(() => Array.from({ length: 7 }, (_, index) => addDays(weekStart, index)), [weekStart]);

  const load = useCallback(async () => {
    setError(null);
    setAppointments(null);
    const { data, error: apiError, response } = await browserApi.GET("/api/v1/appointments", {
      params: {
        query: {
          from: weekStart.toISOString(),
          to: weekEnd.toISOString(),
          professionalId: professionalFilter || undefined,
        },
      },
    });
    if (!response.ok) {
      setError(apiErrorMessage(apiError));
      return;
    }
    setAppointments(data?.items ?? []);
  }, [weekStart, weekEnd, professionalFilter]);

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

  useEffect(() => {
    void (async () => {
      const list = await loadStaff(role);
      setStaff(list ?? []);
    })();
  }, [role]);

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

  function staffName(id: string | undefined): string {
    if (!id) return "";
    return staff.find((member) => member.id === id)?.fullName ?? "";
  }

  const bookable = useMemo(
    () => patients.filter((patient) => canBookWith(role, patient, bookingProfessional)),
    [patients, role, bookingProfessional],
  );
  const bookingNotice =
    role === "professional" && bookable.length === 0
      ? "No tenés pacientes a tu cargo para agendar. Un paciente queda a tu cargo cuando lo registrás vos o el titular te lo asigna."
      : null;

  function openBooking(day?: Date) {
    setBookingStartsAt(day ? morningDateTimeLocal(day, zone) : toDateTimeLocalValue(new Date()));
    setBookingOpen(true);
  }

  return (
    <div className="space-y-4">
      <FilterBar
        label="Filtros de la agenda"
        sticky
        actions={
          <Button size="sm" onClick={() => openBooking()}>
            Nuevo turno
          </Button>
        }
      >
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="secondary" size="sm" onClick={() => setAnchor(addDays(weekStart, -7))}>
            Semana anterior
          </Button>
          <Button variant="secondary" size="sm" onClick={() => setAnchor(new Date())}>
            Hoy
          </Button>
          <Button variant="secondary" size="sm" onClick={() => setAnchor(addDays(weekStart, 7))}>
            Semana siguiente
          </Button>
        </div>

        {seesEveryone ? (
          <label className="flex flex-col gap-0.5 text-xs font-medium text-ink-muted">
            Profesional
            <select
              value={professionalFilter}
              onChange={(event) => {
                setSelected(null);
                setProfessionalFilter(event.target.value);
              }}
              className="rounded-control border border-border-strong bg-surface px-2 py-1.5 text-sm text-ink"
            >
              <option value="">Todos</option>
              {professionals.map((member) => (
                <option key={member.id} value={member.id}>
                  {member.fullName}
                </option>
              ))}
            </select>
          </label>
        ) : null}

        <p className="text-sm text-ink-muted self-center">
          {formatDayHeading(weekStart, zone)} — {formatDayHeading(addDays(weekStart, 6), zone)}
        </p>
      </FilterBar>

      {error ? (
        <p role="alert" className="text-sm text-danger">
          {error}
        </p>
      ) : null}

      {error !== null ? null : appointments === null ? (
        <LoadingState label="Cargando la semana…" rows={7} />
      ) : (
        <div className="grid gap-3 md:grid-cols-7">
          {days.map((day, index) => (
            <section
              key={day.toISOString()}
              className="flex min-h-48 flex-col rounded-[var(--radius-panel)] border border-border bg-surface p-3"
            >
              <h2 className="text-xs font-semibold uppercase tracking-wide text-ink-muted">
                {formatDayHeading(day, zone)}
              </h2>
              <ul className="mt-2 flex flex-1 flex-col gap-2">
                {(byDay[index] ?? []).length === 0 ? (
                  <li>
                    <button
                      type="button"
                      onClick={() => openBooking(day)}
                      className="w-full rounded-md border border-dashed border-border px-2 py-6 text-xs text-ink-muted hover:border-brand hover:bg-brand/5 hover:text-ink"
                    >
                      Sin turnos — agendar
                    </button>
                  </li>
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
                        {professionalFilter === "" ? (
                          <span className="block text-ink-muted">
                            {staffName(slot.professionalUserId)}
                          </span>
                        ) : null}
                        <span className="block text-ink-muted">{modalityLabel(slot.modality)}</span>
                      </button>
                    </li>
                  ))
                )}
              </ul>
            </section>
          ))}
        </div>
      )}

      <AppointmentBookingModal
        open={bookingOpen}
        professionalId={bookingProfessional}
        patients={bookable}
        notice={bookingNotice}
        zone={zone}
        initialStartsAt={bookingStartsAt}
        onClose={() => setBookingOpen(false)}
        onCreated={() => void load()}
      />

      <AppointmentDetailModal
        appointment={selected}
        patientName={selected ? patientName(selected.patientId) : ""}
        zone={zone}
        open={selected !== null}
        onClose={() => setSelected(null)}
        onChanged={() => {
          setSelected(null);
          void load();
        }}
      />
    </div>
  );
}