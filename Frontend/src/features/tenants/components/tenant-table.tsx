import type { Tenant, TenantStatus } from "@/entities/tenant/model";
import {
  TENANT_STATUS_LABEL,
  TENANT_STATUS_TONE,
  formatCreatedAt,
} from "@/entities/tenant/model";
import { Notice } from "@/shared/ui/notice";

const TONE_CLASSES: Record<string, string> = {
  neutral: "bg-surface-muted text-ink-muted border-border",
  positive: "bg-success/5 text-success border-success/30",
  negative: "bg-danger/5 text-danger border-danger/30",
};

function StatusPill({ status }: { status: TenantStatus | undefined }) {
  const key = status ?? "trial";
  const label = TENANT_STATUS_LABEL[key] ?? key;
  const tone = TENANT_STATUS_TONE[key] ?? "neutral";

  return (
    <span
      className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium ${TONE_CLASSES[tone]}`}
    >
      {label}
    </span>
  );
}

/**
 * Presentational table. It receives data and renders it: no fetching, no state.
 * That keeps it testable and reusable from any container.
 */
export function TenantTable({ tenants }: { tenants: Tenant[] }) {
  if (tenants.length === 0) {
    return (
      <Notice title="Todavía no hay consultorios">
        Creá el primero con el formulario de arriba para empezar a probar el alta.
      </Notice>
    );
  }

  return (
    <div className="overflow-hidden rounded-[var(--radius-panel)] border border-border bg-surface">
      <table className="w-full text-left text-sm">
        <caption className="sr-only">Consultorios registrados</caption>
        <thead className="bg-surface-muted text-xs tracking-wide text-ink-muted uppercase">
          <tr>
            <th scope="col" className="px-4 py-3 font-medium">
              Consultorio
            </th>
            <th scope="col" className="px-4 py-3 font-medium">
              Subdominio
            </th>
            <th scope="col" className="px-4 py-3 font-medium">
              Estado
            </th>
            <th scope="col" className="px-4 py-3 font-medium">
              Alta
            </th>
          </tr>
        </thead>
        <tbody>
          {tenants.map((tenant) => (
            <tr key={tenant.id} className="border-t border-border">
              <td className="px-4 py-3 font-medium">{tenant.name}</td>
              <td className="px-4 py-3 font-mono text-xs text-ink-muted">{tenant.slug}</td>
              <td className="px-4 py-3">
                <StatusPill status={tenant.status} />
              </td>
              <td className="px-4 py-3 text-ink-muted">{formatCreatedAt(tenant.createdAt)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
