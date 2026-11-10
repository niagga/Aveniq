import { serverApi } from "@/shared/api/server";
import { CreateTenantForm } from "@/features/tenants/components/create-tenant-form";
import { TenantTable } from "@/features/tenants/components/tenant-table";
import { Notice } from "@/shared/ui/notice";

/**
 * The dashboard is a Server Component: it reads from the API on the server, so
 * the browser never receives an API token, a database DSN or an internal URL.
 */
export const dynamic = "force-dynamic";

export default async function DashboardPage() {
  const { data, error, response } = await serverApi.GET("/api/v1/tenants", {
    params: { query: { limit: 50, offset: 0 } },
  });

  return (
    <main className="mx-auto flex min-h-full w-full max-w-5xl flex-col gap-6 px-6 py-12">
      <header>
        <h1 className="text-2xl font-semibold tracking-tight">Aveniq</h1>
        <p className="mt-1 text-sm text-ink-muted">
          Gestión de consultorios: pacientes, agenda y facturación.
        </p>
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
    </main>
  );
}
