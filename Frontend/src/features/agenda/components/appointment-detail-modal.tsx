"use client";

import { useEffect, useState } from "react";

import {
  canManageAppointment,
  dateTimeLocalToRFC3339,
  formatSlotTime,
  modalityLabel,
  toDateTimeLocalValue,
  type Appointment,
  type Modality,
} from "@/entities/schedule/model";
import { ModalityField, StartsAtField } from "@/features/agenda/components/appointment-fields";
import { apiErrorMessage, browserApi } from "@/shared/api/client";
import { useWritesLocked } from "@/shared/auth/practice";
import { getSessionUser } from "@/shared/auth/session";
import { Button } from "@/shared/ui/button";
import { ConfirmDialog } from "@/shared/ui/confirm-dialog";
import { FormModal } from "@/shared/ui/form-modal";
import { Modal } from "@/shared/ui/modal";

export function AppointmentDetailModal({
  appointment,
  patientName,
  zone,
  open,
  onClose,
  onChanged,
}: {
  appointment: Appointment | null;
  patientName: string;
  zone: string;
  open: boolean;
  onClose: () => void;
  onChanged: () => void;
}) {
  const viewer = getSessionUser();
  const writesLocked = useWritesLocked();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [confirmingCancel, setConfirmingCancel] = useState(false);
  const [rescheduleOpen, setRescheduleOpen] = useState(false);

  const scheduled = appointment?.status === "scheduled";
  const canAct =
    appointment !== null &&
    scheduled &&
    canManageAppointment(viewer?.role, viewer?.id ?? "", appointment.professionalUserId);

  async function transition(path: "attend" | "cancel") {
    if (!appointment?.id) return;
    setError(null);
    setBusy(true);
    try {
      const result =
        path === "attend"
          ? await browserApi.POST("/api/v1/appointments/{id}/attend", {
              params: { path: { id: appointment.id } },
            })
          : await browserApi.POST("/api/v1/appointments/{id}/cancel", {
              params: { path: { id: appointment.id } },
            });
      if (result.error || !result.data) {
        setError(apiErrorMessage(result.error));
        return;
      }
      onChanged();
      onClose();
    } catch {
      setError("No pudimos contactar al servidor.");
    } finally {
      setBusy(false);
    }
  }

  if (!appointment) return null;

  return (
    <>
      <Modal
        open={open && !rescheduleOpen}
        title={patientName}
        description={`${formatSlotTime(appointment.startsAt, zone)}–${formatSlotTime(appointment.endsAt, zone)} · ${modalityLabel(appointment.modality)} · ${appointment.status}`}
        onClose={onClose}
        footer={() => (
          <div className="flex flex-wrap gap-2">
            {canAct && !writesLocked ? (
              <>
                <Button variant="secondary" size="sm" onClick={() => setRescheduleOpen(true)} disabled={busy}>
                  Cambiar fecha
                </Button>
                <Button variant="primary" size="sm" onClick={() => void transition("attend")} busy={busy}>
                  Marcar atendido
                </Button>
                <Button
                  variant="danger"
                  size="sm"
                  onClick={() => setConfirmingCancel(true)}
                  disabled={busy}
                >
                  Cancelar turno
                </Button>
              </>
            ) : null}
            <Button variant="ghost" size="sm" onClick={onClose}>
              Cerrar
            </Button>
          </div>
        )}
      >
        {error ? (
          <p role="alert" className="text-sm text-danger">
            {error}
          </p>
        ) : null}
        {appointment.notes ? (
          <p className="text-sm text-ink-muted whitespace-pre-line">{appointment.notes}</p>
        ) : (
          <p className="text-sm text-ink-muted">Sin notas.</p>
        )}
      </Modal>

      <RescheduleModal
        open={rescheduleOpen}
        appointment={appointment}
        zone={zone}
        onClose={() => setRescheduleOpen(false)}
        onSaved={() => {
          setRescheduleOpen(false);
          onChanged();
          onClose();
        }}
      />

      <ConfirmDialog
        open={confirmingCancel}
        title={`¿Cancelar el turno de ${patientName}?`}
        description="El horario queda libre y el turno pasa a cancelado. No se borra: sigue en la semana, tachado."
        confirmLabel="Cancelar turno"
        cancelLabel="Volver"
        tone="danger"
        busy={busy}
        onCancel={() => setConfirmingCancel(false)}
        onConfirm={() => {
          setConfirmingCancel(false);
          void transition("cancel");
        }}
      />
    </>
  );
}

function RescheduleModal({
  open,
  appointment,
  zone,
  onClose,
  onSaved,
}: {
  open: boolean;
  appointment: Appointment;
  zone: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [startsAt, setStartsAt] = useState("");
  const [endsAt, setEndsAt] = useState("");
  const [modality, setModality] = useState<Modality | "">("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const writesLocked = useWritesLocked();

  useEffect(() => {
    if (!open) return;
    setStartsAt(toDateTimeLocalValue(new Date(appointment.startsAt)));
    setEndsAt(toDateTimeLocalValue(new Date(appointment.endsAt)));
    setModality(appointment.modality);
    setError(null);
  }, [open, appointment]);

  async function submit() {
    if (writesLocked || !appointment.id) return;
    const start = dateTimeLocalToRFC3339(startsAt);
    const end = dateTimeLocalToRFC3339(endsAt);
    if (!start || !end) {
      setError("La hora de inicio y de fin tienen que ser válidas.");
      return;
    }
    if (modality === "") {
      setError("Elegí la modalidad.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const result = await browserApi.POST("/api/v1/appointments/{id}/reschedule", {
        params: { path: { id: appointment.id } },
        body: { startsAt: start, endsAt: end, modality },
      });
      if (result.error || !result.data) {
        setError(apiErrorMessage(result.error));
        return;
      }
      onSaved();
    } catch {
      setError("No pudimos contactar al servidor.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <FormModal
      open={open}
      title="Cambiar fecha del turno"
      description={`Horario en la zona del consultorio (${zone}).`}
      submitLabel="Guardar"
      busy={busy}
      error={error}
      onClose={onClose}
      onSubmit={submit}
    >
      <StartsAtField value={startsAt} onChange={setStartsAt} disabled={writesLocked} />
      <label className="block text-xs font-medium text-ink-muted">
        Fin
        <input
          type="datetime-local"
          value={endsAt}
          onChange={(event) => setEndsAt(event.target.value)}
          disabled={writesLocked}
          className="mt-1 w-full rounded-md border border-border px-3 py-2 text-sm disabled:opacity-60"
          required
        />
      </label>
      <ModalityField value={modality} onChange={setModality} disabled={writesLocked} />
    </FormModal>
  );
}