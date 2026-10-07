import type { FormEvent } from "react";
import { Move } from "lucide-react";
import { Button } from "../../components/Button";
import { Field } from "../../components/Field";
import { Modal } from "../../components/Modal";
import type { FileEntry } from "../../shared/domain";

/** Collect the destination folder and confirm a file move. */
export function FileMoveDialog({
  entry,
  destination,
  root,
  busy,
  onDestinationChange,
  onClose,
  onMove,
}: {
  entry: FileEntry;
  destination: string;
  root: string;
  busy: boolean;
  onDestinationChange: (destination: string) => void;
  onClose: () => void;
  onMove: (event: FormEvent<HTMLFormElement>) => void;
}) {
  return (
    <Modal title={`Move ${entry.name}`} onClose={onClose}>
      <form className="form-stack modal-body" onSubmit={onMove}>
        <Field label="Destination folder path">
          <input
            autoFocus
            required
            value={destination}
            onChange={(event) => onDestinationChange(event.target.value)}
            placeholder={root}
          />
          <small className="field-hint">
            Enter an existing folder path inside this server's files. The item
            will keep its current name.
          </small>
        </Field>
        <div className="form-actions">
          <Button variant="subtle" type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button disabled={busy || !destination.trim()}>
            <Move size={14} />
            {busy ? "Moving…" : "Move"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
