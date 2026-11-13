import { AcceptInvitationForm } from "@/features/invitations/components/accept-invitation-form";
import { isValidSlug } from "@/entities/tenant/slug";

type SearchParams = Promise<{ token?: string | string[]; consultorio?: string | string[] }>;

/**
 * The public invitation page. It is a Server Component only to read the link's query
 * string, which is then handed to the Client Component as plain props.
 *
 * The practice slug comes from the link and is checked before use: it is shown to the
 * invitee and placed in a sign-in link, so a value that is not a slug is dropped.
 */
export default async function InvitationPage({ searchParams }: { searchParams: SearchParams }) {
  const params = await searchParams;
  const token = single(params.token) ?? "";
  const slug = single(params.consultorio)?.toLowerCase() ?? null;
  const practiceSlug = slug !== null && isValidSlug(slug) ? slug : null;

  return (
    <main className="mx-auto flex min-h-full w-full max-w-md flex-col justify-center gap-6 px-6 py-16">
      <header>
        <p className="text-sm font-semibold text-brand">Aveniq</p>
      </header>

      <AcceptInvitationForm token={token} practiceSlug={practiceSlug} />
    </main>
  );
}

function single(value: string | string[] | undefined): string | undefined {
  return typeof value === "string" ? value : undefined;
}
