import type { components } from "@/shared/api/schema";

export type Appointment = components["schemas"]["AppointmentBody"];
export type AppointmentSeries = components["schemas"]["SeriesBody"];

const WEEKDAY_LABELS = ["Domingo", "Lunes", "Martes", "Miércoles", "Jueves", "Viernes", "Sábado"];

export function weekdayLabel(weekday: number): string {
  return WEEKDAY_LABELS[weekday] ?? String(weekday);
}

/** Monday 00:00 of the week containing `anchor`, in the given IANA zone. */
export function startOfWeek(anchor: Date, timeZone: string): Date {
  const parts = civilDate(anchor, timeZone);
  const asUTC = Date.UTC(parts.year, parts.month - 1, parts.day);
  const weekday = new Date(asUTC).getUTCDay(); // 0 Sunday
  const daysFromMonday = (weekday + 6) % 7;
  return zonedDate(parts.year, parts.month, parts.day - daysFromMonday, 0, 0, timeZone);
}

export function addDays(date: Date, days: number): Date {
  return new Date(date.getTime() + days * 24 * 60 * 60 * 1000);
}

export function formatDayHeading(date: Date, timeZone: string): string {
  return date.toLocaleDateString("es-AR", {
    weekday: "short",
    day: "numeric",
    month: "short",
    timeZone,
  });
}

export function formatSlotTime(value: string, timeZone: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleTimeString("es-AR", {
    hour: "2-digit",
    minute: "2-digit",
    timeZone,
  });
}

export function toDateTimeLocalValue(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

export function dateTimeLocalToRFC3339(value: string): string | null {
  if (value.trim() === "") return null;
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return null;
  return parsed.toISOString();
}

export function toISODate(date: Date, timeZone: string): string {
  const parts = civilDate(date, timeZone);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${parts.year}-${pad(parts.month)}-${pad(parts.day)}`;
}

function civilDate(date: Date, timeZone: string): { year: number; month: number; day: number } {
  const formatted = new Intl.DateTimeFormat("en-US", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(date);
  const lookup = Object.fromEntries(formatted.map((part) => [part.type, part.value]));
  return {
    year: Number(lookup.year),
    month: Number(lookup.month),
    day: Number(lookup.day),
  };
}

function zonedDate(year: number, month: number, day: number, hour: number, minute: number, timeZone: string): Date {
  // Approximate: construct a UTC instant and shift until the civil fields match.
  const guess = new Date(Date.UTC(year, month - 1, day, hour, minute));
  const parts = civilDate(guess, timeZone);
  const deltaDays = Date.UTC(year, month - 1, day) - Date.UTC(parts.year, parts.month - 1, parts.day);
  return new Date(guess.getTime() + deltaDays);
}
