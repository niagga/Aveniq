import { LogoutButton } from "@/features/auth/components/logout-button";
import { CreateTenantForm } from "@/features/tenants/components/create-tenant-form";
import { TenantTable } from "@/features/tenants/components/tenant-table";
import { serverApi } from "@/shared/api/server";
import { SessionGate } from "@/shared/auth/session-gate";
import { Notice } from "@/shared/ui/notice";

/**
 * The dashboard is a Server Component: it reads from the API on the server, so
 * the browser never receives an API token, a database DSN or an internal URL.
 *
 * SessionGate is mounted inside it and is a UX boundary only — the server has
 * already fetched the data by the time the gate runs in the browser. The API is
 * what has to require a token, and that is T19.
 */
export const dynamic = "force-dynamic";

export default async function DashboardPage() {
  const { data, error, response } = await serverApi.GET("/api/v1/tenants", {
    params: { query: { limit: 50, offset: 0 } },
  });

  return (
    <main className="mx-auto flex min-h-full w-full max-w-5xl flex-col gap-6 px-6 py-12">
      <SessionGate>
        <header className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 className="text-2xl font-semibold tracking-tight">Aveniq</h1>
            <p className="mt-1 text-sm text-ink-muted">
              Gestión de consultorios: pacientes, agenda y facturación.
            </p>
          </div>
          <LogoutButton />
        </header>

        <CreateTenantForm />

        {error ? (
          <Notice tone="danger" title="No pudimos leer los consultorios">
            {response.status === 0
              ? "La API no responde. Verificá que el backend esté corriendo en 127.0.0.1:8080."
              : `La API respondió ${response.status}.`}
          </Notice>
        ) : (
          <TenantTable tenants={data?.items ?? []} />
        )}
      </SessionGate>
    </main>
  );
}
