import React, { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { X } from "lucide-react";

/** Render a dismissible modal with escape-key and backdrop support. */
export function Modal({
  title,
  onClose,
  children,
}: {
  title: string;
  onClose: () => void;
  children: React.ReactNode;
}) {
  const [closing, setClosing] = useState(false);
  const closingRef = useRef(false);
  const closeTimer = useRef<number | undefined>(undefined);

  const close = useCallback(() => {
    if (closingRef.current) return;
    closingRef.current = true;
    setClosing(true);
    closeTimer.current = window.setTimeout(onClose, 180);
  }, [onClose]);

  useEffect(() => {
    const key = (e: KeyboardEvent) => e.key === "Escape" && close();
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, [close]);

  useEffect(
    () => () => {
      if (closeTimer.current !== undefined) {
        window.clearTimeout(closeTimer.current);
      }
    },
    [],
  );

  return createPortal(
    <div
      className={`modal-backdrop${closing ? " closing" : ""}`}
      onMouseDown={(e) => e.target === e.currentTarget && close()}
    >
      <section className="modal-card" role="dialog" aria-modal="true">
        <header className="modal-header">
          <div>
            <span className="auth-kicker">CONTROL PLANE</span>
            <h2>{title}</h2>
          </div>
          <button className="icon-button" onClick={close}>
            <X size={18} />
          </button>
        </header>
        {children}
      </section>
    </div>,
    document.body,
  );
}
