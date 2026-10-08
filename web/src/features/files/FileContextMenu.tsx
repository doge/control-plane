import { createPortal } from "react-dom";
import { Archive, Download, Move } from "lucide-react";

/** Show the file actions at the cursor position that opened the menu. */
export function FileContextMenu({
  x,
  y,
  onClose,
  onMove,
  onZip,
  onDownload,
  isFolder,
  canManage,
}: {
  x: number;
  y: number;
  onClose: () => void;
  onMove: () => void;
  onZip: () => void;
  onDownload: () => void;
  isFolder: boolean;
  canManage: boolean;
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
        {isFolder && canManage && (
          <button role="menuitem" onClick={onZip}>
            <Archive size={14} />
            Create ZIP
          </button>
        )}
        {!isFolder && (
          <button role="menuitem" onClick={onDownload}>
            <Download size={14} />
            Download
          </button>
        )}
        {canManage && (
          <button role="menuitem" onClick={onMove}>
            <Move size={14} />
            Move to folder…
          </button>
        )}
      </div>
    </div>,
    document.body,
  );
}
