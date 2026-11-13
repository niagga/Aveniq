import type { components } from "@/shared/api/schema";

/**
 * Accounts and invitations as the API describes them. The wire types are the schema,
 * not copies, so the screens cannot drift from the contract silently.
 */
export type User = components["schemas"]["UserBody"];
export type Invitation = components["schemas"]["InvitationBody"];
export type Role = User["role"];

/** Every role, in the order the permission matrix lists them (T33). */
export const ROLES: readonly Role[] = ["owner", "admin", "professional", "assistant", "accountant"];

const ROLE_LABEL: Record<Role, string> = {
  owner: "Titular",
  admin: "Administración",
  professional: "Profesional",
  assistant: "Recepción",
  accountant: "Contador",
};

export function roleLabel(role: string): string {
  return ROLE_LABEL[role as Role] ?? role;
}

/** The API enforces this for every chosen password (iam.ValidatePassword). */
export const PASSWORD_MIN_LENGTH = 12;

/** Owner and admin hold users:read and users:manage; the other roles hold neither. */
export function canManageAccounts(role: string | undefined): boolean {
  return role === "owner" || role === "admin";
}

/**
 * Whether a viewer may grant `role` or act on an account that holds it. Mirrors
 * iam.AuthorizeChange and iam.AuthorizeInvitation so the screen does not offer what
 * the API will refuse. The API stays the authority.
 */
export function mayGrant(viewerRole: string, role: string): boolean {
  return viewerRole === "owner" || role !== "owner";
}

/**
 * The path of a public invitation page. The practice slug rides along so the invitee
 * can be told which practice to sign in to: the acceptance answer does not name it.
 */
export function invitationPath(token: string, practiceSlug: string | null): string {
  const params = new URLSearchParams({ token });
  if (practiceSlug !== null) params.set("consultorio", practiceSlug);
  return `/invitacion?${params.toString()}`;
}

/** Formats an RFC 3339 instant for a screen, tolerating a value it cannot parse. */
export function formatDate(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleDateString("es-AR", { day: "2-digit", month: "2-digit", year: "numeric" });
}
