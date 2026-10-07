import { Button } from "../../components/Button";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { Folder, Plus, Server as ServerIcon, Trash } from "lucide-react";
import {
  api,
  errorMessage,
  type Config,
  type GameServer,
  type NodeItem,
} from "../../shared/domain";
import { useConfirmation } from "../../shared/dialogs";
import { FileBrowser } from "../files/server-files";
import { showToast } from "../../shared/toast";
import {
  Field,
  FormActions,
  Modal,
  PageHeading,
  PanelHeading,
  Status,
  formatBytes,
} from "../../components";

type VolumeRow = {
  server: GameServer;
  id: string;
  name: string;
  mountPath: string;
  sizeBytes: number;
  primary: boolean;
};

const minVolumeBytes = 128 * 1024 * 1024;
const reservedBytes = 128 * 1024 * 1024;
const gib = 1024 ** 3;

function freeBytes(node?: NodeItem) {
  if (!node?.stats) return 0;
  return Math.max(
    0,
    node.stats.storageTotalBytes - node.stats.storageUsedBytes,
  );
}

function hasStorageStats(node?: NodeItem) {
  return !!node?.stats && node.stats.storageTotalBytes > 0;
}

function sizeLabel(bytes: number) {
  return bytes > 0 ? formatBytes(bytes) : "No disk size configured";
}

/** List the volumes available to the current user and manage their attachments. */
export function VolumesPage({
  servers,
  nodes,
  configs,
  canManage,
  canExtend,
  canBrowseFiles,
  canManageFiles,
  isRoot,
  userId,
  onRefresh,
  onOpenServer,
}: {
  servers: GameServer[];
  nodes: NodeItem[];
  configs: Config[];
  canManage: boolean;
  canExtend: boolean;
  canBrowseFiles: boolean;
  canManageFiles: boolean;
  isRoot: boolean;
  userId: string;
  onRefresh: () => Promise<void>;
  onOpenServer: (server: GameServer) => void;
}) {
  const confirmAction = useConfirmation();
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<VolumeRow | null>(null);
  const [browsing, setBrowsing] = useState<VolumeRow | null>(null);
  const [serverId, setServerId] = useState(servers[0]?.id || "");
  const [name, setName] = useState("");
  const [mountPath, setMountPath] = useState("/mnt/data");
  const [sizeGB, setSizeGB] = useState("4");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const visibleServers = isRoot
    ? servers
    : servers.filter((server) => server.managerId === userId);
  useEffect(() => {
    if (!visibleServers.some((server) => server.id === serverId)) {
      setServerId(visibleServers[0]?.id || "");
    }
  }, [visibleServers, serverId]);

  const rows = useMemo<VolumeRow[]>(
    () =>
      visibleServers.flatMap((server) => {
        const config = configs.find((item) => item.id === server.configId);
        const result: VolumeRow[] = [
          {
            server,
            id: "data",
            name: "Server data",
            mountPath: config?.spec?.dataDirectory || "/home/container",
            sizeBytes: server.diskSizeBytes || 0,
            primary: true,
          },
        ];
        for (const volume of server.volumes || []) {
          result.push({ ...volume, server, primary: false });
        }
        return result;
      }),
    [visibleServers, configs],
  );

  const currentServer = visibleServers.find((server) => server.id === serverId);
  const currentNode = nodes.find((node) => node.id === currentServer?.nodeId);
  const available = freeBytes(currentNode);
  const dockerStorage = currentNode?.stats?.storageMode === "docker-volume";

  const resetCreate = () => {
    setCreating(false);
    setError("");
    setName("");
    setMountPath("/mnt/data");
    setSizeGB("4");
  };

  const createVolume = async (event: FormEvent) => {
    event.preventDefault();
    if (!currentServer) return;
    setBusy(true);
    setError("");
    try {
      await api(`/api/servers/${currentServer.id}/volumes`, {
        method: "POST",
        body: JSON.stringify({
          name,
          mountPath,
          sizeBytes: Math.round(Number(sizeGB) * gib),
        }),
      });
      showToast("Volume created and attached.", "success");
      resetCreate();
      await onRefresh();
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const resizeVolume = async (event: FormEvent) => {
    event.preventDefault();
    if (!editing) return;
    const sizeBytes = Math.round(Number(sizeGB) * gib);
    if (sizeBytes <= editing.sizeBytes) {
      setError("Volumes can be extended but cannot be reduced.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api(`/api/servers/${editing.server.id}/volumes/${editing.id}`, {
        method: "PATCH",
        body: JSON.stringify({ sizeBytes }),
      });
      showToast("Volume extended.", "success");
      setEditing(null);
      await onRefresh();
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  const deleteVolume = async (row: VolumeRow) => {
    if (
      !(await confirmAction({
        title: "Delete volume",
        message: `Permanently delete ${row.name} mounted at ${row.mountPath}? All data on this volume will be erased. The server will be redeployed to detach it.`,
        confirmLabel: "Delete volume and data",
        destructive: true,
      }))
    )
      return;
    setBusy(true);
    try {
      await api(`/api/servers/${row.server.id}/volumes/${row.id}`, {
        method: "DELETE",
      });
      showToast("Volume and its data deleted.", "success");
      await onRefresh();
    } catch (err: unknown) {
      showToast(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="page-stack">
      <PageHeading
        eyebrow="SERVER STORAGE"
        title="Volumes"
        description="Create and attach server volumes, then extend them as needed. Linux nodes enforce disk sizes; Mac local testing uses Docker volumes without size quotas."
        action={
          canManage ? (
            <Button
              variant="primary"
              disabled={!servers.length || busy}
              onClick={() => setCreating(true)}
            >
              <Plus size={15} />
              Create volume
            </Button>
          ) : undefined
        }
      />
      {!rows.length ? (
        <div className="inline-empty">
          {visibleServers.length
            ? "No volumes are available."
            : "No servers are assigned to your account."}
        </div>
      ) : (
        rows.map((row) => {
          const node = nodes.find((item) => item.id === row.server.nodeId);
          const nodeOnline = node?.status === "online";
          const maxSize = hasStorageStats(node)
            ? row.sizeBytes + freeBytes(node) - reservedBytes
            : Number.POSITIVE_INFINITY;
          return (
            <section
              className="panel-card volume-server-card"
              key={`${row.server.id}:${row.id}`}
            >
              <PanelHeading
                title={row.name}
                sub={`${row.server.name} · ${node?.name || "Unknown node"} · ${row.mountPath}`}
                action={
                  <Button
                    variant="subtle"
                    size="tiny"
                    onClick={() => onOpenServer(row.server)}
                  >
                    <ServerIcon size={13} />
                    Open server
                  </Button>
                }
              />
              <div className="volume-row">
                <div className="volume-row-info">
                  <strong>{row.primary ? "Server data disk" : row.name}</strong>
                  <small>{row.mountPath}</small>
                  <span className="role-pill">{sizeLabel(row.sizeBytes)}</span>
                </div>
                <div className="volume-row-actions">
                  <Status status={nodeOnline ? "Online" : "Offline"} />
                  {canBrowseFiles && (
                    <Button
                      variant="subtle"
                      size="tiny"
                      disabled={!nodeOnline || !row.server.containerId}
                      onClick={() => setBrowsing(row)}
                    >
                      <Folder size={13} />
                      Files
                    </Button>
                  )}
                  {canExtend &&
                    node?.stats?.storageMode !== "docker-volume" && (
                      <Button
                        variant="subtle"
                        size="tiny"

                        disabled={
                          busy ||
                          (!!node && node.status !== "online") ||
                          maxSize < minVolumeBytes
                        }
                        onClick={() => {
                          setEditing(row);
                          setSizeGB(
                            String(
                              Math.max(0.125, (row.sizeBytes || 4 * gib) / gib),
                            ),
                          );
                          setError("");
                        }}
                      >
                        Extend
                      </Button>
                    )}
                  {!row.primary && canManage && (
                    <button
                      className="icon-button danger-icon"
                      title="Delete volume"
                      aria-label={`Delete volume ${row.name}`}
                      disabled={busy || node?.status !== "online"}
                      onClick={() => void deleteVolume(row)}
                    >
                      <Trash size={14} />
                    </button>
                  )}
                </div>
              </div>
            </section>
          );
        })
      )}

      {creating && (
        <Modal title="Create and attach volume" onClose={resetCreate}>
          <form className="form-stack modal-body" onSubmit={createVolume}>
            <Field label="Server">
              <select
                value={serverId}
                onChange={(event) => setServerId(event.target.value)}
                required
              >
                {visibleServers.map((server) => (
                  <option key={server.id} value={server.id}>
                    {server.name}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Volume name">
              <input
                value={name}
                onChange={(event) => setName(event.target.value)}
                required
                maxLength={64}
                placeholder="Backups or mods"
              />
            </Field>
            <Field label="Container mount path">
              <input
                value={mountPath}
                onChange={(event) => setMountPath(event.target.value)}
                required
                placeholder="/mnt/data"
              />
            </Field>
            <Field label="Size (GB)">
              <input
                type="number"
                min="0.125"
                step="0.125"
                max={
                  hasStorageStats(currentNode)
                    ? Math.max(0, (available - reservedBytes) / gib)
                    : undefined
                }
                value={sizeGB}
                onChange={(event) => setSizeGB(event.target.value)}
                required
              />
              <small className="field-hint">
                {hasStorageStats(currentNode)
                  ? `Available on ${currentNode?.name}: ${formatBytes(available)}.`
                  : "Current free space is unavailable."}{" "}
                {dockerStorage
                  ? "Mac local Docker volumes do not enforce a disk quota."
                  : "The node reserves the full size and enforces it with an ext4 virtual disk."}
              </small>
            </Field>
            {dockerStorage && (
              <p className="field-hint">
                This Mac node uses a Docker named volume. Docker Desktop does
                not enforce the configured size; Linux nodes use size-limited
                disks.
              </p>
            )}
            {error && <div className="form-error">{error}</div>}
            <FormActions
              cancel={resetCreate}
              busy={busy}
              label="Create and attach"
            />
          </form>
        </Modal>
      )}

      {editing && (
        <Modal
          title={`Extend ${editing.name}`}
          onClose={() => {
            setEditing(null);
            setError("");
          }}
        >
          <form className="form-stack modal-body" onSubmit={resizeVolume}>
            <p className="modal-intro">
              Current size: {sizeLabel(editing.sizeBytes)}. Volumes can grow but
              cannot be shrunk.
            </p>
            <Field label="New size (GB)">
              <input
                type="number"
                min={Math.max(0.125, editing.sizeBytes / gib)}
                step="0.125"
                max={
                  hasStorageStats(
                    nodes.find((node) => node.id === editing.server.nodeId),
                  )
                    ? Math.max(0, maxSizeFor(editing, nodes) / gib)
                    : undefined
                }
                value={sizeGB}
                onChange={(event) => setSizeGB(event.target.value)}
                required
              />
              <small className="field-hint">
                {hasStorageStats(
                  nodes.find((node) => node.id === editing.server.nodeId),
                )
                  ? `Additional node space available: ${formatBytes(freeBytes(nodes.find((node) => node.id === editing.server.nodeId)))}.`
                  : "Current free space is unavailable."}
              </small>
            </Field>
            {error && <div className="form-error">{error}</div>}
            <FormActions
              cancel={() => {
                setEditing(null);
                setError("");
              }}
              busy={busy}
              label="Extend volume"
            />
          </form>
        </Modal>
      )}

      {browsing && (
        <Modal
          title={`${browsing.server.name} · ${browsing.name}`}
          onClose={() => setBrowsing(null)}
        >
          <div className="modal-body">
            <p className="modal-intro">Files in {browsing.mountPath}</p>
            <FileBrowser
              key={`${browsing.server.id}:${browsing.id}`}
              server={browsing.server}
              root={browsing.mountPath}
              basePath={`/api/servers/${browsing.server.id}/volumes/${browsing.id}/files`}
              canManage={canManageFiles}
            />
          </div>
        </Modal>
      )}
    </div>
  );
}

function maxSizeFor(row: VolumeRow, nodes: NodeItem[]) {
  return (
    row.sizeBytes +
    freeBytes(nodes.find((node) => node.id === row.server.nodeId)) -
    reservedBytes
  );
}
