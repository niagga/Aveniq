import { useEffect, useState } from "react";

import { canManageAccounts } from "@/entities/iam/model";
import { browserApi } from "@/shared/api/client";

/** A staff account as the patient screens name it: a name and a role, nothing else. */
export type StaffMember = { id: string; fullName: string; role: string };

const FIRST_PAGE = { limit: 100, offset: 0 };

/**
 * Reads the names the patient screens show, the way the viewer is allowed to read them.
 * Owner and admin read the account list (users:read) and keep the active accounts. The
 * other roles read the staff directory (users:directory), which carries names and roles.
 *
 * Returns null when the read fails, so a caller can fall back instead of showing nobody.
 */
export async function loadStaff(role: string | undefined): Promise<StaffMember[] | null> {
  if (canManageAccounts(role)) {
    const { data, response } = await browserApi.GET("/api/v1/users", {
      params: { query: FIRST_PAGE },
    });
    if (!response.ok || !data) return null;
    return (data.items ?? [])
      .filter((account) => account.isActive)
      .map((account) => ({ id: account.id, fullName: account.fullName, role: account.role }));
  }

  const { data, response } = await browserApi.GET("/api/v1/users/directory", {
    params: { query: FIRST_PAGE },
  });
  if (!response.ok || !data) return null;
  return (data.items ?? []).map((entry) => ({
    id: entry.id,
    fullName: entry.fullName,
    role: entry.role,
  }));
}

/** The staff the viewer can name, loaded once per role. Empty until the read answers. */
export function useStaff(role: string | undefined): StaffMember[] {
  const [members, setMembers] = useState<StaffMember[]>([]);

  useEffect(() => {
    let active = true;
    void loadStaff(role).then((loaded) => {
      if (active) setMembers(loaded ?? []);
    });
    return () => {
      active = false;
    };
  }, [role]);

  return members;
}

/** The name of a staff account, or a neutral label when the viewer cannot read it. */
export function staffName(members: StaffMember[], id: string | undefined): string {
  if (!id) return "Sin asignar";
  return members.find((member) => member.id === id)?.fullName ?? "Profesional asignado";
}
