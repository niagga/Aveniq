import type { components } from "@/shared/api/schema";

/**
 * The tenant as the API publishes it. Derived from the generated schema instead
 * of being declared again, so a backend change breaks the build here rather than
 * silently at runtime.
 */
export type Tenant = components["schemas"]["TenantBody"];

export type TenantStatus = NonNullable<Tenant["status"]>;

/** User-facing labels live in one place; components must not inline them. */
export const TENANT_STATUS_LABEL: Record<string, string> = {
  trial: "Prueba",
  active: "Activo",
  suspended: "Suspendido",
};

export const TENANT_STATUS_TONE: Record<string, "neutral" | "positive" | "negative"> = {
  trial: "neutral",
  active: "positive",
  suspended: "negative",
};

export function formatCreatedAt(value: string | undefined): string {
  if (!value) return "—";
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return "—";
  // Explicit locale: the UI is Spanish for Argentine practices, independent of
  // the browser's configured language.
  return new Intl.DateTimeFormat("es-AR", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(parsed);
}
