import { createPortal } from "react-dom";
import { Move } from "lucide-react";

/** Show the file actions at the cursor position that opened the menu. */
export function FileContextMenu({
  x,
  y,
  onClose,
  onMove,
}: {
  x: number;
  y: number;
  onClose: () => void;
  onMove: () => void;
}) {
  return createPortal(
    <div
      className="file-context-backdrop"
      onClick={onClose}
      onContextMenu={(event) => {
        event.preventDefault();
        onClose();
      }}
    >
      <div
        className="file-context-menu"
        role="menu"
        style={{ left: x, top: y }}
        onClick={(event) => event.stopPropagation()}
      >
        <button role="menuitem" onClick={onMove}>
          <Move size={14} />
          Move to folder…
        </button>
      </div>
    </div>,
    document.body,
  );
}
