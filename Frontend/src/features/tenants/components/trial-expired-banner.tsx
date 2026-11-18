import { formatCreatedAt } from "@/entities/tenant/model";
import { Notice } from "@/shared/ui/notice";

/**
 * Shown on every authenticated screen once the practice's trial has ended.
 *
 * The date is the one the API published (trialEndsAt). The copy does not promise
 * conversion or payment: those are out of the MVP (L58).
 */
export function TrialExpiredBanner({ trialEndsAt }: { trialEndsAt: string }) {
  return (
    <Notice tone="warning" title="La prueba de este consultorio venció">
      Terminó el {formatCreatedAt(trialEndsAt)}. Podés seguir leyendo. No se puede crear, editar ni
      anular nada.
    </Notice>
  );
}
