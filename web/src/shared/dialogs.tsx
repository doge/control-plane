import { Button } from "../components/Button";
import React, { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { Shield } from "lucide-react";

export type Confirmation = {
  title: string;
  message: string;
  confirmLabel: string;
  destructive: boolean;
};

const ConfirmationContext = React.createContext<
  (options: Confirmation) => Promise<boolean>
>(async () => false);

/** Provide the shared styled confirmation dialog to descendant views. */
export function ConfirmationProvider({
  children,
}: {
  children: React.ReactNode;
}) {
  const [confirmation, setConfirmation] = useState<Confirmation | null>(null);
  const [closing, setClosing] = useState(false);
  const resolver = useRef<((accepted: boolean) => void) | null>(null);
  const closingRef = useRef(false);
  const closeTimer = useRef<number | undefined>(undefined);

  const ask = useCallback((options: Confirmation) => {
    closingRef.current = false;
    setClosing(false);
    setConfirmation(options);
    return new Promise<boolean>((resolve) => {
      resolver.current = resolve;
    });
  }, []);

  const finish = (accepted: boolean) => {
    if (closingRef.current) return;
    closingRef.current = true;
    setClosing(true);
    closeTimer.current = window.setTimeout(() => {
      resolver.current?.(accepted);
      resolver.current = null;
      setConfirmation(null);
      closingRef.current = false;
    }, 180);
  };

  useEffect(
    () => () => {
      if (closeTimer.current !== undefined) {
        window.clearTimeout(closeTimer.current);
      }
    },
    [],
  );

  return (
    <ConfirmationContext.Provider value={ask}>
      {children}
      {confirmation &&
        createPortal(
        <div
          className={`modal-backdrop confirmation-backdrop${closing ? " closing" : ""}`}
          role="presentation"
        >
          <section
            className="confirm-dialog"
            role="alertdialog"
            aria-modal="true"
            aria-labelledby="confirm-title"
            aria-describedby="confirm-message"
          >
            <div className="confirm-dialog-mark">
              <Shield size={19} />
            </div>
            <h2 id="confirm-title">{confirmation.title}</h2>
            <p id="confirm-message">{confirmation.message}</p>
            <div className="form-actions">
              <Button variant="subtle" onClick={() => finish(false)}>
                Cancel
              </Button>
              <button
                className={`button ${
                  confirmation.destructive ? "danger" : "primary"
                }`}
                onClick={() => finish(true)}
              >
                {confirmation.confirmLabel}
              </button>
            </div>
          </section>
        </div>,
        document.body,
      )}
    </ConfirmationContext.Provider>
  );
}

/** Return the promise-based confirmation function from the nearest provider. */
export function useConfirmation() {
  return React.useContext(ConfirmationContext);
}
