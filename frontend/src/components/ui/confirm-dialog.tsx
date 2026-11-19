'use client';

import { useEffect, useId, useRef, type ReactNode } from 'react';

interface ConfirmDialogProps {
  open: boolean;
  title: string;
  description?: string;
  children?: ReactNode;
  confirmLabel?: string;
  onConfirm: () => void;
  onClose: () => void;
}

export function ConfirmDialog({
  open,
  title,
  description,
  children,
  confirmLabel = 'Подтвердить',
  onConfirm,
  onClose,
}: ConfirmDialogProps) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  useEffect(() => {
    if (open) ref.current?.showModal();
    else ref.current?.close();
  }, [open]);
  return (
    <dialog
      className="confirm-dialog"
      ref={ref}
      onCancel={onClose}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
      aria-labelledby={titleId}
    >
      <div className="stack-lg">
        <h2 id={titleId}>{title}</h2>
        <p className="muted">{description || children}</p>
        <div className="button-row">
          <button className="button" onClick={onConfirm}>
            {confirmLabel}
          </button>
          <button className="button secondary" onClick={onClose} autoFocus>
            Отмена
          </button>
        </div>
      </div>
    </dialog>
  );
}
