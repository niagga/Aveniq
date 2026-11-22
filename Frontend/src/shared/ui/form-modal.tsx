"use client";

import { useEffect, useId, useState, type ReactNode } from "react";

import { Button } from "./button";
import { Modal } from "./modal";

/**
 * A popup whose body is a form: "Nuevo paciente", "Editar mutual", "Emitir comprobante" (E8).
 *
 * The popup knows whether the person typed anything. It listens to the form's input and change
 * events, so no screen has to keep a dirty flag of its own: the first edit arms the
 * unsaved-changes guard in `Modal`, and closing the popup disarms it.
 *
 * The submit button lives in the footer, outside the `<form>`, and points at it by id. The
 * caller owns the save: `onSubmit` runs the request and the caller closes the popup on success.
 */
export function FormModal({
  open,
  title,
  description,
  size,
  submitLabel,
  busy = false,
  error = null,
  onClose,
  onSubmit,
  children,
}: {
  open: boolean;
  title: string;
  description?: ReactNode;
  size?: "sm" | "md" | "lg";
  submitLabel: string;
  busy?: boolean;
  error?: string | null;
  onClose: () => void;
  onSubmit: () => void | Promise<void>;
  children: ReactNode;
}) {
  const formId = useId();
  const [edited, setEdited] = useState(false);

  useEffect(() => {
    if (!open) setEdited(false);
  }, [open]);

  return (
    <Modal
      open={open}
      title={title}
      description={description}
      size={size}
      dirty={edited}
      busy={busy}
      onClose={onClose}
      footer={(requestClose) => (
        <>
          <Button variant="secondary" onClick={requestClose} disabled={busy}>
            Cancelar
          </Button>
          <Button type="submit" form={formId} variant="primary" busy={busy}>
            {busy ? "Guardando…" : submitLabel}
          </Button>
        </>
      )}
    >
      <form
        id={formId}
        noValidate
        onSubmit={(event) => {
          event.preventDefault();
          void onSubmit();
        }}
        onInput={() => setEdited(true)}
        onChange={() => setEdited(true)}
        className="grid gap-4"
      >
        {children}
        {error ? (
          <p role="alert" className="text-sm text-danger">
            {error}
          </p>
        ) : null}
      </form>
    </Modal>
  );
}
