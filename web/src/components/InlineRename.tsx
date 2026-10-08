import { useEffect, useRef, useState } from "react";
import { errorMessage } from "../shared/domain";

/** Let authorized users rename a heading in place with Enter to save. */
export function InlineRename({
  value,
  label,
  editable,
  maxLength,
  pattern,
  onSave,
}: {
  value: string;
  label: string;
  editable: boolean;
  maxLength?: number;
  pattern?: string;
  onSave: (name: string) => Promise<string | void>;
}) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(value);
  const [displayName, setDisplayName] = useState(value);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => setDisplayName(value), [value]);
  useEffect(() => {
    if (editing) inputRef.current?.focus();
  }, [editing]);

  const beginEditing = () => {
    setDraft(displayName);
    setError("");
    setEditing(true);
  };

  const save = async () => {
    const name = draft.trim();
    if (!name) {
      setError(`${label} name cannot be empty.`);
      return;
    }
    if (name === displayName.trim()) {
      setEditing(false);
      return;
    }
    setBusy(true);
    setError("");
    try {
      const savedName = await onSave(name);
      setDisplayName(savedName || name);
      setEditing(false);
    } catch (cause: unknown) {
      setError(errorMessage(cause));
    } finally {
      setBusy(false);
    }
  };

  if (!editable) return <>{displayName}</>;
  if (!editing) {
    return (
      <button
        type="button"
        className="inline-rename-trigger"
        title={`Click to rename ${label.toLowerCase()}`}
        onClick={beginEditing}
      >
        {displayName}
      </button>
    );
  }

  return (
    <span className="inline-rename-edit">
      <input
        ref={inputRef}
        className="inline-rename-input"
        aria-label={`${label} name`}
        value={draft}
        maxLength={maxLength}
        pattern={pattern}
        disabled={busy}
        aria-invalid={Boolean(error)}
        onChange={(event) => setDraft(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter") {
            event.preventDefault();
            void save();
          } else if (event.key === "Escape" && !busy) {
            setDraft(displayName);
            setError("");
            setEditing(false);
          }
        }}
      />
      <small
        className={`inline-rename-hint${error ? " inline-rename-error" : ""}`}
        role="status"
        aria-live="polite"
      >
        {error || (busy ? "Saving…" : "Press Enter to save · Esc to cancel")}
      </small>
    </span>
  );
}
