import { useState, type FormEvent } from "react";
import { createPortal } from "react-dom";
import { ArrowLeft, ChevronRight, Globe, Link, Upload, X } from "lucide-react";
import { Button } from "../../components/Button";
import { Field, formatBytes } from "../../components";
import type { UploadProgress } from "../../shared/domain";

/** Format an upload estimate as seconds or minutes and seconds. */
function formatUploadETA(seconds: number): string {
  const total = Math.max(0, Math.ceil(seconds));
  if (total < 60) return `${total}s`;
  return `${Math.floor(total / 60)}m ${total % 60}s`;
}

/** Offer local and URL upload flows with progress and an estimated finish time. */
/** Upload files from the computer or a URL and display transfer progress. */
export function FileUploadDialog({
  busy,
  error,
  progress,
  onClose,
  onUploadComputer,
  onUploadWeb,
}: {
  busy: boolean;
  error: string;
  progress: UploadProgress | null;
  onClose: () => void;
  onUploadComputer: (file: File) => Promise<void>;
  onUploadWeb: (url: string) => Promise<void>;
}) {
  const [method, setMethod] = useState<"computer" | "web" | null>(null);
  const [file, setFile] = useState<File | null>(null);
  const [url, setUrl] = useState("");
  const [closing, setClosing] = useState(false);

  const close = () => {
    if (closing) return;
    setClosing(true);
    window.setTimeout(onClose, 180);
  };

  const submitURL = (event: FormEvent) => {
    event.preventDefault();
    void onUploadWeb(url);
  };

  return createPortal(
    <div
      className={`modal-backdrop upload-backdrop${closing ? " closing" : ""}`}
      role="presentation"
    >
      <section
        className="modal-card upload-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="upload-dialog-title"
      >
        <div className="modal-header">
          <div>
            <div className="eyebrow">SERVER FILES</div>
            <h2 id="upload-dialog-title">Upload a file</h2>
          </div>
          <button className="icon-button" title="Close" onClick={close}>
            <X size={16} />
          </button>
        </div>
        <div className="upload-dialog-body">
          {!method ? (
            <div className="upload-methods">
              <button
                className="upload-method"
                onClick={() => setMethod("computer")}
              >
                <span className="upload-method-icon">
                  <Upload size={19} />
                </span>
                <span>
                  <strong>From your computer</strong>
                  <small>Choose a file from this device.</small>
                </span>
                <ChevronRight size={17} />
              </button>
              <button
                className="upload-method"
                onClick={() => setMethod("web")}
              >
                <span className="upload-method-icon">
                  <Globe size={19} />
                </span>
                <span>
                  <strong>From the web</strong>
                  <small>Download directly to the server from a URL.</small>
                </span>
                <ChevronRight size={17} />
              </button>
            </div>
          ) : method === "computer" ? (
            <div className="upload-method-content">
              <button className="back-link" onClick={() => setMethod(null)}>
                <ArrowLeft size={14} />
                Choose another method
              </button>
              <label className="upload-file-picker">
                <Upload size={19} />
                <strong>{file?.name || "Choose a file"}</strong>
                <small>
                  {file
                    ? formatBytes(file.size)
                    : "Select a file from your computer"}
                </small>
                <input
                  type="file"
                  onChange={(event) => setFile(event.target.files?.[0] || null)}
                />
              </label>
              {progress && (
                <div className="upload-progress" aria-live="polite">
                  <div
                    className="upload-progress-track"
                    role="progressbar"
                    aria-label="File upload progress"
                    aria-valuemin={0}
                    aria-valuemax={100}
                    aria-valuenow={
                      progress.total
                        ? Math.min(
                            100,
                            (progress.loaded / progress.total) * 100,
                          )
                        : 0
                    }
                  >
                    <div
                      className="upload-progress-bar"
                      style={{
                        width: `${
                          progress.total
                            ? Math.min(
                                100,
                                (progress.loaded / progress.total) * 100,
                              )
                            : 0
                        }%`,
                      }}
                    />
                  </div>
                  <div className="upload-progress-meta">
                    <span>
                      {progress.phase === "saving"
                        ? "Upload sent · Saving on the server…"
                        : `${formatBytes(progress.loaded)} of ${formatBytes(progress.total)}`}
                    </span>
                    <span>
                      {progress.phase === "saving"
                        ? "Finishing…"
                        : progress.remainingSeconds === null
                          ? "Estimating time…"
                          : `About ${formatUploadETA(progress.remainingSeconds)} left`}
                    </span>
                  </div>
                </div>
              )}
              {error && <div className="form-error">{error}</div>}
              <div className="form-actions">
                <Button variant="subtle" onClick={close}>
                  Cancel
                </Button>
                <Button
                  variant="primary"

                  disabled={!file || busy}
                  onClick={() => file && void onUploadComputer(file)}
                >
                  <Upload size={14} />
                  {busy ? "Uploading…" : "Upload file"}
                </Button>
              </div>
            </div>
          ) : (
            <form className="upload-method-content" onSubmit={submitURL}>
              <button
                className="back-link"
                type="button"
                onClick={() => setMethod(null)}
              >
                <ArrowLeft size={14} />
                Choose another method
              </button>
              <Field label="File URL">
                <input
                  autoFocus
                  type="url"
                  placeholder="https://example.com/server-config.yml"
                  value={url}
                  onChange={(event) => setUrl(event.target.value)}
                  required
                />
                <small className="field-hint">
                  The node downloads the file directly into the current server
                  folder.
                </small>
              </Field>
              {error && <div className="form-error">{error}</div>}
              <div className="form-actions">
                <Button
                  variant="subtle"

                  type="button"
                  onClick={close}
                >
                  Cancel
                </Button>
                <Button variant="primary" disabled={!url || busy}>
                  <Link size={14} />
                  {busy ? "Downloading…" : "Download to server"}
                </Button>
              </div>
            </form>
          )}
        </div>
      </section>
    </div>,
    document.body,
  );
}
