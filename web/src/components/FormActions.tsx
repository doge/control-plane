import { Button } from "./Button";
import { Spinner } from "./Spinner";

/** Keep cancel and submit actions aligned across application forms. */
export function FormActions({
  cancel,
  busy,
  label,
  disabled = false,
}: {
  cancel: () => void;
  busy: boolean;
  label: string;
  disabled?: boolean;
}) {
  return (
    <div className="form-actions">
      <Button variant="subtle" type="button" onClick={cancel}>
        Cancel
      </Button>
      <Button variant="primary" disabled={busy || disabled}>
        {busy && <Spinner />}
        {busy ? "Working…" : label}
      </Button>
    </div>
  );
}
