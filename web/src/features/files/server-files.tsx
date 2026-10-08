import { Button } from "../../components/Button";
import React, { useCallback, useEffect, useState } from "react";
import {
  Archive,
  ChevronRight,
  File as FileIcon,
  Folder,
  RefreshCw,
  Save,
  Terminal,
  Trash,
  Upload,
  X,
} from "lucide-react";

import {
  errorMessage,
  api,
  type FileEntry,
  type UploadProgress,
  type GameServer,
  type NodeItem,
} from "../../shared/domain";

import { useConfirmation } from "../../shared/dialogs";
import { CodeEditor, type CodeLanguage, detectSyntax } from "../../shared/code-editor";
import { showToast } from "../../shared/toast";
import { Console } from "../servers/server-console";
import { FileUploadDialog } from "./FileUploadDialog";
import { FileContextMenu } from "./FileContextMenu";
import { FileMoveDialog } from "./FileMoveDialog";
import { BackupManager } from "../servers/BackupManager";

/** Switch between a server's console, files, and backups based on permissions. */
export function ServerWorkspace({
  server,
  node,
  root = "/data",
  permissions = [],
  deploying = false,
}: {
  server: GameServer;
  node?: NodeItem;
  root?: string;
  permissions?: string[];
  deploying?: boolean;
}) {
  const tabs = [
    { name: "Console", permission: "servers.console" },
    { name: "Files", permission: "servers.files.view" },
    { name: "Backups", permission: "servers.backups.view" },
  ].filter((item) => permissions.includes(item.permission));
  const [tab, setTab] = useState(tabs[0]?.name || "");
  if (!tabs.length) return null;
  return (
    <section className="server-workspace">
      <div className="workspace-tabs">
        {tabs.map(({ name: t }) => (
          <button
            className={tab === t ? "active" : ""}
            key={t}
            onClick={() => setTab(t)}
          >
            {t === "Files" ? (
              <Folder size={15} />
            ) : t === "Backups" ? (
              <Save size={15} />
            ) : (
              <Terminal size={15} />
            )}{" "}
            {t}
          </button>
        ))}
      </div>
      {tab === "Console" ? (
        <Console server={server} node={node} deploying={deploying} />
      ) : tab === "Files" ? (
        <FileBrowser
          server={server}
          root={root}
          canManage={permissions.includes("servers.files.manage")}
        />
      ) : (
        <BackupManager
          server={server}
          canManage={permissions.includes("servers.backups.manage")}
        />
      )}
    </section>
  );
}
/** Browse, edit, upload, move, archive, download, and delete server files. */
export function FileBrowser({
  server,
  root = "/data",
  canManage = false,
  basePath = `/api/servers/${server.id}/files`,
}: {
  server: GameServer;
  root?: string;
  canManage?: boolean;
  basePath?: string;
}) {
  const confirmAction = useConfirmation();
  const [dir, setDir] = useState(root);
  const [entries, setEntries] = useState<FileEntry[]>([]);
  const [selected, setSelected] = useState("");
  const [content, setContent] = useState("");
  const [busy, setBusy] = useState(true);
  const [uploadOpen, setUploadOpen] = useState(false);
  const [uploadError, setUploadError] = useState("");
  const [uploadProgress, setUploadProgress] = useState<UploadProgress | null>(
    null,
  );
  const [moving, setMoving] = useState<FileEntry | null>(null);
  const [moveDestination, setMoveDestination] = useState(dir);
  const [contextMenu, setContextMenu] = useState<{
    entry: FileEntry;
    x: number;
    y: number;
  } | null>(null);
  const [dropTarget, setDropTarget] = useState("");
  const [sortBy, setSortBy] = useState<"name" | "modifiedAt">("name");
  const [sortDirection, setSortDirection] = useState<"asc" | "desc">("asc");
  const [syntax, setSyntax] = useState<CodeLanguage>("plaintext");
  const load = useCallback(
    async (p = dir) => {
      p = p === "/" ? root : p;
      setBusy(true);
      try {
        const r = await api<{ entries: FileEntry[] }>(
          `${basePath}?path=${encodeURIComponent(p)}`,
        );
        setEntries(r.entries || []);
        setDir(p);
        setSelected("");
        setContent("");
      } catch (e: unknown) {
        showToast(errorMessage(e));
      } finally {
        setBusy(false);
      }
    },
    [basePath, dir, root],
  );
  const entryPath = (name: string) => `${dir.replace(/\/$/, "")}/${name}`;
  const moveEntry = async (source: string, destination: string) => {
    setBusy(true);
    try {
      await api(`${basePath}/move`, {
        method: "POST",
        body: JSON.stringify({ source, destination: destination.trim() }),
      });
      showToast(
        `Moved ${source.split("/").pop()} to ${destination}.`,
        "success",
      );
      setMoving(null);
      await load();
    } catch (e: unknown) {
      showToast(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const unzipEntry = async (entry: FileEntry) => {
    setBusy(true);
    try {
      const response = await api<{ path: string }>(`${basePath}/unzip`, {
        method: "POST",
        body: JSON.stringify({ path: entryPath(entry.name) }),
      });
      showToast(`Extracted ${entry.name} into ${response.path}.`, "success");
      await load();
    } catch (e: unknown) {
      showToast(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const zipEntry = async (entry: FileEntry) => {
    setBusy(true);
    try {
      const response = await api<{ path: string }>(`${basePath}/zip`, {
        method: "POST",
        body: JSON.stringify({ path: entryPath(entry.name) }),
      });
      showToast(`Created ${response.path}.`, "success");
      await load();
    } catch (e: unknown) {
      showToast(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const downloadEntry = (entry: FileEntry) => {
    const url = `${basePath}/download?path=${encodeURIComponent(entryPath(entry.name))}`;
    window.location.assign(url);
    setContextMenu(null);
  };
  useEffect(() => {
    void load(root);
  }, [server.id, root]);
  const open = async (name: string) => {
    const p = dir + "/" + name;
    if (entries.find((e) => e.name === name)?.directory) {
      await load(p);
      return;
    }
    setBusy(true);
    try {
      const r = await api<{ content: string }>(
        `${basePath}/content?path=${encodeURIComponent(p)}`,
      );
      if (typeof r.content !== "string") {
        throw new Error("The file read returned no content.");
      }
      setSelected(p);
      setContent(r.content);
      setSyntax(detectSyntax(name));
    } catch (e: unknown) {
      setSelected("");
      setContent("");
      showToast(`Could not open ${name}: ${errorMessage(e)}`);
    } finally {
      setBusy(false);
    }
  };
  const remove = async (p: string) => {
    if (
      !(await confirmAction({
        title: "Delete file",
        message: `Permanently delete ${p}?`,
        confirmLabel: "Delete",
        destructive: true,
      }))
    )
      return;
    setBusy(true);
    try {
      await api(`${basePath}/delete?path=${encodeURIComponent(p)}`, {
        method: "DELETE",
      });
      if (selected === p) {
        setSelected("");
        setContent("");
      }
      showToast(`Deleted ${p.split("/").pop()}.`, "success");
      await load();
    } catch (e: unknown) {
      showToast(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const save = async () => {
    if (!selected) return;
    setBusy(true);
    try {
      await api(`${basePath}/content?path=${encodeURIComponent(selected)}`, {
        method: "PUT",
        body: JSON.stringify({ content }),
      });
      showToast("File saved.", "success");
    } catch (e: unknown) {
      showToast(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const formatJSON = () => {
    try {
      setContent(JSON.stringify(JSON.parse(content), null, 2));
    } catch (e: unknown) {
      showToast(`Cannot format JSON: ${errorMessage(e)}`);
    }
  };
  const uploadFromComputer = async (file: File) => {
    setBusy(true);
    setUploadError("");
    setUploadProgress({
      loaded: 0,
      total: file.size,
      remainingSeconds: null,
      phase: "uploading",
    });
    const form = new FormData();
    form.append("file", file);
    form.append("path", dir + "/" + file.name);
    try {
      await new Promise<void>((resolve, reject) => {
        const startedAt = Date.now();
        const xhr = new XMLHttpRequest();
        xhr.open("POST", `${basePath}/upload`);
        xhr.upload.onprogress = (event) => {
          const total = event.lengthComputable ? event.total : file.size;
          const elapsedSeconds = (Date.now() - startedAt) / 1000;
          const bytesPerSecond =
            elapsedSeconds > 0 ? event.loaded / elapsedSeconds : 0;
          setUploadProgress({
            loaded: event.loaded,
            total,
            remainingSeconds:
              bytesPerSecond > 0
                ? Math.max(0, total - event.loaded) / bytesPerSecond
                : null,
            phase: "uploading",
          });
        };
        xhr.upload.onload = () => {
          setUploadProgress({
            loaded: file.size,
            total: file.size,
            remainingSeconds: null,
            phase: "saving",
          });
        };
        xhr.onload = () => {
          let result: { error?: string } = {};
          try {
            result = JSON.parse(xhr.responseText || "{}");
          } catch {
            /* use the status text below */
          }
          if (xhr.status >= 200 && xhr.status < 300) resolve();
          else
            reject(
              new Error(result.error || xhr.statusText || "Upload failed."),
            );
        };
        xhr.onerror = () =>
          reject(
            new Error("Upload failed. Check the connection and try again."),
          );
        xhr.onabort = () => reject(new Error("Upload cancelled."));
        xhr.send(form);
      });
      setUploadOpen(false);
      showToast(`Uploaded ${file.name}.`, "success");
      setUploadProgress(null);
      await load();
    } catch (e: unknown) {
      setUploadError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const uploadFromWeb = async (url: string) => {
    setBusy(true);
    setUploadError("");
    setUploadProgress(null);
    const name =
      new URL(url).pathname.split("/").filter(Boolean).pop() || "download";
    try {
      await api(`${basePath}/from-url`, {
        method: "POST",
        body: JSON.stringify({ url, path: dir + "/" + name }),
      });
      setUploadOpen(false);
      showToast(`Downloaded ${name} to the server.`, "success");
      await load();
    } catch (e: unknown) {
      setUploadError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  if (!server.containerId) {
    return (
      <section className="panel-card file-browser">
        <p>Deploy this server before browsing its files.</p>
      </section>
    );
  }
  const lines = content.split("\n");
  const sortedEntries = [...entries].sort((a, b) => {
    if (a.directory !== b.directory) return a.directory ? -1 : 1;
    const compare =
      sortBy === "name"
        ? a.name.localeCompare(b.name, undefined, {
            numeric: true,
            sensitivity: "base",
          })
        : Date.parse(a.modifiedAt) - Date.parse(b.modifiedAt);
    return sortDirection === "asc" ? compare : -compare;
  });
  const normalizedRoot = root.replace(/\/+$/, "") || "/";
  const normalizedDir = dir.replace(/\/+$/, "") || "/";
  const rootName = normalizedRoot.split("/").filter(Boolean).pop() || "Files";
  const breadcrumbs = [{ name: rootName, path: root }];
  const relativePath =
    normalizedDir === normalizedRoot
      ? ""
      : normalizedDir.slice(normalizedRoot.length).replace(/^\/+/, "");
  let breadcrumbPath = normalizedRoot;
  for (const segment of relativePath.split("/").filter(Boolean)) {
    breadcrumbPath = `${breadcrumbPath.replace(/\/$/, "")}/${segment}`;
    breadcrumbs.push({ name: segment, path: breadcrumbPath });
  }
  return (
    <section className="panel-card file-browser">
      <div className="file-toolbar">
        <nav className="file-breadcrumbs" aria-label="Server file path">
          <Folder size={15} aria-hidden="true" />
          {breadcrumbs.map((crumb, index) => (
            <React.Fragment key={crumb.path}>
              {index > 0 && (
                <ChevronRight
                  className="file-breadcrumb-separator"
                  size={13}
                  aria-hidden="true"
                />
              )}
              {index < breadcrumbs.length - 1 ? (
                <button
                  className={`file-breadcrumb${dropTarget === crumb.path ? " drop-target" : ""}`}
                  disabled={busy}
                  title={`Open or drop here: ${crumb.path}`}
                  onClick={() => void load(crumb.path)}
                  onDragOver={(event) => {
                    if (!canManage || busy) return;
                    event.preventDefault();
                    event.dataTransfer.dropEffect = "move";
                    setDropTarget(crumb.path);
                  }}
                  onDragLeave={(event) => {
                    if (
                      event.currentTarget === event.target ||
                      !event.currentTarget.contains(event.relatedTarget as Node)
                    ) {
                      setDropTarget("");
                    }
                  }}
                  onDrop={(event) => {
                    if (!canManage || busy) return;
                    event.preventDefault();
                    event.stopPropagation();
                    setDropTarget("");
                    const source = event.dataTransfer.getData("text/plain");
                    if (source) void moveEntry(source, crumb.path);
                  }}
                >
                  {crumb.name}
                </button>
              ) : (
                <span className="file-breadcrumb current" aria-current="page">
                  {crumb.name}
                </span>
              )}
            </React.Fragment>
          ))}
        </nav>
        <Button
          variant="subtle"
          size="tiny"

          disabled={busy}
          onClick={() => load()}
        >
          <RefreshCw size={14} />
          Refresh
        </Button>
        {canManage && (
          <Button
            variant="primary"
            size="tiny"

            disabled={busy}
            onClick={() => {
              setUploadError("");
              setUploadOpen(true);
            }}
          >
            <Upload size={14} />
            Upload
          </Button>
        )}
      </div>
      <div className="file-list-heading">
        <button
          className={sortBy === "name" ? "active" : ""}
          onClick={() => {
            if (sortBy === "name") {
              setSortDirection((old) => (old === "asc" ? "desc" : "asc"));
            } else {
              setSortBy("name");
              setSortDirection("asc");
            }
          }}
        >
          Name {sortBy === "name" ? (sortDirection === "asc" ? "↑" : "↓") : ""}
        </button>
        <button
          className={sortBy === "modifiedAt" ? "active" : ""}
          onClick={() => {
            if (sortBy === "modifiedAt") {
              setSortDirection((old) => (old === "asc" ? "desc" : "asc"));
            } else {
              setSortBy("modifiedAt");
              setSortDirection("desc");
            }
          }}
        >
          Date modified{" "}
          {sortBy === "modifiedAt" ? (sortDirection === "asc" ? "↑" : "↓") : ""}
        </button>
        <span />
      </div>
      <div className="file-list">
        {sortedEntries.map((e) => (
          <div
            className={`file-row${dropTarget === entryPath(e.name) ? " file-row-drop-target" : ""}`}
            key={e.name}
            draggable={canManage && !busy}
            onDragStart={(event) => {
              event.dataTransfer.effectAllowed = "move";
              event.dataTransfer.setData("text/plain", entryPath(e.name));
            }}
            onDragOver={(event) => {
              if (!canManage || !e.directory || busy) return;
              event.preventDefault();
              event.dataTransfer.dropEffect = "move";
              setDropTarget(entryPath(e.name));
            }}
            onDragLeave={(event) => {
              if (
                event.currentTarget === event.target ||
                !event.currentTarget.contains(event.relatedTarget as Node)
              ) {
                setDropTarget("");
              }
            }}
            onDrop={(event) => {
              if (!canManage || !e.directory || busy) return;
              event.preventDefault();
              event.stopPropagation();
              setDropTarget("");
              const source = event.dataTransfer.getData("text/plain");
              if (source) void moveEntry(source, entryPath(e.name));
            }}
            onContextMenu={(event) => {
              if (e.directory && !canManage) return;
              event.preventDefault();
              const menuItems =
                (e.directory ? Number(canManage) : 1) + Number(canManage);
              setContextMenu({
                entry: e,
                x: Math.max(
                  8,
                  Math.min(event.clientX, window.innerWidth - 168),
                ),
                y: Math.max(
                  8,
                  Math.min(
                    event.clientY,
                    window.innerHeight - (menuItems * 34 + 16),
                  ),
                ),
              });
            }}
          >
            <button onClick={() => open(e.name)} disabled={busy}>
              {e.directory ? <Folder size={15} /> : <FileIcon size={15} />}
              <span>{e.name}</span>
              {!e.directory && <small>{e.size} B</small>}
            </button>
            <time className="file-modified" dateTime={e.modifiedAt}>
              {e.modifiedAt ? new Date(e.modifiedAt).toLocaleString() : "—"}
            </time>
            {canManage && (
              <div className="file-row-actions">
                {!e.directory && e.name.toLowerCase().endsWith(".zip") && (
                  <button
                    className="icon-button file-unzip"
                    aria-label={`Unzip ${e.name}`}
                    title={`Unzip to ${e.name.slice(0, -4)}`}
                    disabled={busy}
                    onClick={() => void unzipEntry(e)}
                  >
                    <Archive size={14} />
                  </button>
                )}
                <button
                  className="icon-button danger-icon file-delete"
                  aria-label={`Delete ${e.directory ? "folder" : "file"} ${e.name}`}
                  title={`Delete ${e.directory ? "folder" : "file"}`}
                  disabled={busy}
                  onClick={() => remove(entryPath(e.name))}
                >
                  <Trash size={14} />
                </button>
              </div>
            )}
          </div>
        ))}
        {!entries.length &&
          (busy ? (
            <div
              className="file-loading-state"
              role="status"
              aria-live="polite"
            >
              <span className="spinner" aria-hidden="true" />
              <span>Loading files…</span>
            </div>
          ) : (
            <div className="console-placeholder">This directory is empty.</div>
          ))}
      </div>
      {selected && (
        <div className="file-editor">
          <div className="file-editor-toolbar">
            <strong>{selected}</strong>
            <div className="file-editor-actions">
              <label>
                Syntax
                <select
                  value={syntax}
                  onChange={(e) => setSyntax(e.target.value as CodeLanguage)}
                >
                  <option value="plaintext">Plain text</option>
                  <option value="json">JSON</option>
                  <option value="yaml">YAML</option>
                  <option value="properties">Properties</option>
                  <option value="javascript">JavaScript</option>
                  <option value="typescript">TypeScript</option>
                  <option value="java">Java</option>
                  <option value="shell">Shell</option>
                  <option value="xml">XML</option>
                </select>
              </label>
              {syntax === "json" && (
                <Button
                  variant="subtle"
                  size="tiny"

                  disabled={busy}
                  onClick={formatJSON}
                >
                  Format JSON
                </Button>
              )}
              <span className="file-line-count">{lines.length} lines</span>
              {canManage && (
                <Button
                  variant="primary"
                  size="tiny"

                  disabled={busy}
                  onClick={save}
                >
                  <Save size={14} />
                  {busy ? "Saving…" : "Save"}
                </Button>
              )}
              <button
                className="icon-button"
                title="Close file"
                onClick={() => setSelected("")}
              >
                <X size={15} />
              </button>
            </div>
          </div>
          {!content && (
            <div className="file-empty-note">This file is empty.</div>
          )}
          <CodeEditor
            value={content}
            onChange={setContent}
            language={syntax}
            ariaLabel="File contents"
          />
        </div>
      )}
      {contextMenu && (
        <FileContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          isFolder={contextMenu.entry.directory}
          canManage={canManage}
          onClose={() => setContextMenu(null)}
          onZip={() => {
            const entry = contextMenu.entry;
            setContextMenu(null);
            void zipEntry(entry);
          }}
          onDownload={() => downloadEntry(contextMenu.entry)}
          onMove={() => {
            setMoveDestination(dir);
            setMoving(contextMenu.entry);
            setContextMenu(null);
          }}
        />
      )}
      {moving && (
        <FileMoveDialog
          entry={moving}
          destination={moveDestination}
          root={root}
          busy={busy}
          onDestinationChange={setMoveDestination}
          onClose={() => setMoving(null)}
          onMove={(event) => {
            event.preventDefault();
            void moveEntry(entryPath(moving.name), moveDestination);
          }}
        />
      )}
      {uploadOpen && (
        <FileUploadDialog
          busy={busy}
          error={uploadError}
          progress={uploadProgress}
          onClose={() => setUploadOpen(false)}
          onUploadComputer={uploadFromComputer}
          onUploadWeb={uploadFromWeb}
        />
      )}
    </section>
  );
}
