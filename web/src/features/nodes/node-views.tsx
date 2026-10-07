import { Button } from "../../components/Button";
import { FormEvent, useCallback, useEffect, useState } from "react";
import {
  ArrowLeft,
  Box,
  Globe,
  Plus,
  RefreshCw,
  Save,
  Server,
  Shield,
  Trash,
} from "lucide-react";

import {
  errorMessage,
  api,
  type GameServer,
  type NodeContainer,
  type NodeImage,
  type NodeItem,
  type NodeResources,
  type Config,
} from "../../shared/domain";

import {
  Detail,
  Empty,
  Entity,
  Field,
  formatBytes,
  Modal,
  PageHeading,
  PanelHeading,
  Status,
  ServerRow,
  TableWrap,
  UsageChart,
} from "../../components";
import { useConfirmation } from "../../shared/dialogs";
import { showToast } from "../../shared/toast";
/** List nodes and show their connected servers and health state. */
export function NodesPage({
  nodes,
  servers,
  onCreate,
  onHelp,
  onOpen,
  onOpenServer,
  canManage = false,
}: {
  nodes: NodeItem[];
  servers: GameServer[];
  onCreate: () => void;
  onHelp: (n: NodeItem) => void;
  onOpen: (n: NodeItem) => void;
  onOpenServer: (server: GameServer) => void;
  canManage?: boolean;
}) {
  return (
    <div className="page-stack">
      <PageHeading
        eyebrow="INFRASTRUCTURE"
        title="Nodes"
        description="Register Docker hosts and manage agent connections."
        action={
          canManage ? (
            <Button variant="primary" onClick={onCreate}>
              <Plus size={16} />
              Add node
            </Button>
          ) : undefined
        }
      />
      {nodes.length ? (
        <section className="panel-card table-card">
          <TableWrap>
            <table>
              <thead>
                <tr>
                  <th>NODE</th>
                  <th>STATUS</th>
                  <th>ADDRESS</th>
                  <th>REGION</th>
                  <th>HOSTED SERVERS</th>
                  <th>LAST SEEN</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {nodes.map((n) => (
                  <tr
                    key={n.id}
                    className="click-row"
                    onClick={() => onOpen(n)}
                  >
                    <td>
                      <Entity
                        icon={Globe}
                        title={n.name}
                        sub={n.id.slice(-8)}
                      />
                    </td>
                    <td>
                      <Status status={n.status} />
                    </td>
                    <td>{n.address || "—"}</td>
                    <td>{n.region || "—"}</td>
                    <td>
                      <div className="node-hosted-servers">
                        {servers
                          .filter((server) => server.nodeId === n.id)
                          .map((server) => (
                            <button
                              key={server.id}
                              className="node-hosted-server"
                              onClick={(event) => {
                                event.stopPropagation();
                                onOpenServer(server);
                              }}
                            >
                              <Server size={12} />
                              {server.name}
                            </button>
                          ))}
                        {!servers.some((server) => server.nodeId === n.id) && (
                          <span className="field-hint">None</span>
                        )}
                      </div>
                    </td>
                    <td>
                      {n.lastSeen
                        ? new Date(n.lastSeen).toLocaleString()
                        : "Never connected"}
                    </td>
                    <td>
                      {canManage && (
                        <Button
                          variant="subtle"
                          size="tiny"

                          onClick={(e) => {
                            e.stopPropagation();
                            onHelp(n);
                          }}
                        >
                          Setup info
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </TableWrap>
        </section>
      ) : (
        <Empty
          icon={Globe}
          title="No nodes registered"
          text="A node is a machine running Docker and the Control Plane node agent."
          action={
            canManage ? (
              <Button variant="primary" onClick={onCreate}>
                <Plus size={15} />
                Register first node
              </Button>
            ) : undefined
          }
        />
      )}
      <div className="tip">
        <Shield size={17} />
        Node tokens are shown only once after creation. Store them privately.
      </div>
    </div>
  );
}

/** Show node health, attached servers, and node management actions. */
export function NodeDetail({
  node,
  servers,
  configByID,
  onOpenServer,
  onOpenResources,
  onBack,
  onDeleted,
  canManage = false,
}: {
  node: NodeItem;
  servers: GameServer[];
  configByID: Map<string, Config>;
  onOpenServer: (server: GameServer) => void;
  onOpenResources: () => void;
  onBack: () => void;
  onDeleted: () => void;
  canManage?: boolean;
}) {
  const confirmAction = useConfirmation();
  const portsFingerprint = JSON.stringify(node.portAllocations || []);
  const ipFingerprint = JSON.stringify(node.ips || []);
  const [address, setAddress] = useState(node.address || "");
  const [ports, setPorts] = useState<{ ip: string; port: number }[]>(
    node.portAllocations || [],
  );
  const [nodeIPs, setNodeIPs] = useState(node.ips || []);
  const [portIP, setPortIP] = useState(node.ips?.[0] || node.address || "");
  const [singlePort, setSinglePort] = useState("25565");
  const [portError, setPortError] = useState("");
  const [busy, setBusy] = useState(false);
  const [history, setHistory] = useState<
    { time: string; cpu: number; memory: number }[]
  >([]);
  const [liveStats, setLiveStats] = useState(node.stats);
  useEffect(() => {
    let active = true;
    const updateStats = async () => {
      try {
        const latest = await api<NodeItem>(`/api/nodes/${node.id}`);
        if (active) setLiveStats(latest.stats);
      } catch {
        // Keep the last known sample visible while a node is temporarily offline.
      }
    };
    void updateStats();
    const timer = window.setInterval(() => void updateStats(), 5000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [node.id]);
  useEffect(() => {
    setAddress(node.address || "");
    setPorts(node.portAllocations || []);
    setNodeIPs(node.ips || []);
    setPortIP(node.ips?.[0] || node.address || "");
  }, [node.id, node.address, portsFingerprint, ipFingerprint]);
  useEffect(() => {
    const s = liveStats;
    if (!s?.updatedAt) return;
    const memory = s.memoryTotalBytes
      ? Math.round((s.memoryUsedBytes / s.memoryTotalBytes) * 100)
      : 0;
    setHistory((old) =>
      [
        ...old,
        {
          time: new Date(s.updatedAt).toLocaleTimeString(),
          cpu: s.cpuPercent,
          memory,
        },
      ].slice(-20),
    );
  }, [liveStats?.updatedAt]);
  const save = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      const updated = await api<NodeItem>(`/api/nodes/${node.id}`, {
        method: "PUT",
        body: JSON.stringify({
          address,
          ips: [],
          portAllocations: ports.map((p) => ({ ...p, port: Number(p.port) })),
        }),
      });
      setAddress(updated.address || address);
      setNodeIPs(updated.ips || []);
      setPorts(updated.portAllocations || []);
      setPortIP(updated.ips?.[0] || updated.address || "");
      showToast(
        node.status === "online"
          ? "DNS addresses, port allocations, and UFW rules saved."
          : "DNS addresses and port allocations saved. UFW rules will be added when the node is online and a server uses an allocation.",
        "success",
      );
    } catch (e: unknown) {
      showToast(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const removeNode = async () => {
    if (
      !(await confirmAction({
        title: `Delete node ${node.name}`,
        message:
          "This permanently removes the node, its server deployments, server files, Docker volumes, and backups. The node must be online if it has server data.",
        confirmLabel: "Delete node and data",
        destructive: true,
      }))
    )
      return;
    setBusy(true);
    try {
      await api(`/api/nodes/${node.id}`, { method: "DELETE" });
      onDeleted();
    } catch (e: unknown) {
      showToast(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const ips = nodeIPs.length ? nodeIPs : [address].filter(Boolean);
  const addPort = () => {
    const port = Number(singlePort);
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      setPortError("Enter a valid port from 1 to 65535.");
      return;
    }
    if (ports.some((entry) => entry.ip === portIP && entry.port === port)) {
      setPortError(`Port ${port} is already allocated for this IP.`);
      return;
    }
    setPorts((old) => [...old, { ip: portIP, port }]);
    setPortError("");
  };
  const updatePort = (index: number, key: "ip" | "port", value: string) =>
    setPorts((old) =>
      old.map((p, i) =>
        i === index
          ? { ...p, [key]: key === "port" ? Number(value) : value }
          : p,
      ),
    );
  const stats = liveStats;
  const attachedServers = servers.filter((server) => server.nodeId === node.id);
  const gb = (n: number) => (n ? `${(n / 1024 ** 3).toFixed(1)} GB` : "—");
  const storageGB = (n: number) => `${(n / 1024 ** 3).toFixed(1)} GB`;
  const storagePct = stats?.storageTotalBytes
    ? (100 * stats.storageUsedBytes) / stats.storageTotalBytes
    : 0;
  const storageKnown = Boolean(stats?.storageTotalBytes);
  const storageFreeBytes = Math.max(
    0,
    (stats?.storageTotalBytes || 0) - (stats?.storageUsedBytes || 0),
  );
  return (
    <div className="page-stack">
      <button className="back-link" onClick={onBack}>
        <ArrowLeft size={15} /> Back to nodes
      </button>
      <PageHeading
        eyebrow="NODE DETAILS"
        title={node.name}
        description={`${node.address || "Address not set"} · ${
          node.region || "Region not set"
        }`}
        action={
          <div className="server-actions">
            <Status status={node.status} />
            <Button variant="subtle" onClick={onOpenResources}>
              <Box size={14} />
              Containers and images
            </Button>
            {canManage && (
              <Button
                variant="danger"

                disabled={busy}
                onClick={removeNode}
              >
                <Trash size={14} />
                Delete node
              </Button>
            )}
          </div>
        }
      />
      <div className="node-facts">
        <div>
          <small>CPU THREADS</small>
          <strong>{stats?.cpuThreads ?? "—"}</strong>
        </div>
        <div>
          <small>ARCHITECTURE</small>
          <strong>{stats?.architecture || "—"}</strong>
        </div>
        <div>
          <small>KERNEL</small>
          <strong>{stats?.kernel || "Waiting for agent"}</strong>
        </div>
      </div>
      <section className="panel-card node-server-list">
        <PanelHeading
          title="Attached servers"
          sub={`${attachedServers.length} server${attachedServers.length === 1 ? "" : "s"} assigned to this node`}
        />
        {attachedServers.length ? (
          <div className="node-server-rows">
            {attachedServers.map((server) => (
              <ServerRow
                key={server.id}
                server={server}
                node={node}
                config={configByID.get(server.configId)}
                onClick={() => onOpenServer(server)}
              />
            ))}
          </div>
        ) : (
          <Empty
            icon={Server}
            title="No servers on this node"
            text="Servers assigned to this node will appear here."
          />
        )}
      </section>
      <section className="panel-card node-metrics">
        <PanelHeading
          title="CPU and memory"
          sub={
            stats?.updatedAt
              ? `Updated ${new Date(stats.updatedAt).toLocaleTimeString()}`
              : "Waiting for node telemetry"
          }
        />
        <div className="metric-current">
          <span>
            CPU <b>{stats ? `${stats.cpuPercent.toFixed(1)}%` : "—"}</b>
          </span>
          <span>
            Memory{" "}
            <b>
              {stats
                ? `${gb(stats.memoryUsedBytes)} / ${gb(stats.memoryTotalBytes)}`
                : "—"}
            </b>
          </span>
        </div>
        <div className="node-chart">
          <NodeChart data={history} />
        </div>
      </section>
      <section className="panel-card storage-card">
        <PanelHeading title="Storage" sub="Node root filesystem" />
        <div className="storage-label">
          <strong>
            {storageKnown
              ? `${storageGB(storageFreeBytes)} free`
              : "Waiting for node storage stats"}
          </strong>
          <span>
            {storageKnown
              ? `${storageGB(stats?.storageUsedBytes || 0)} used · ${storageGB(stats?.storageTotalBytes || 0)} total`
              : "—"}
          </span>
        </div>
        <div className="storage-track">
          <i style={{ width: `${Math.min(100, storagePct)}%` }} />
        </div>
      </section>
      {canManage && (
        <form className="panel-card node-config" onSubmit={save}>
          <PanelHeading
            title="Addresses and allocations"
            sub="Add individual ports available to servers."
          />
          <div className="node-config-fields">
            <Field label="Node hostname or IP">
              <input
                required
                value={address}
                onChange={(e) => setAddress(e.target.value)}
              />
              <small className="field-hint">
                DNS addresses are resolved below. Each saved port becomes
                available in the server configuration dropdown.
              </small>
            </Field>
            <div className="ip-pool-list">
              <div className="allocation-heading">
                <div>
                  <div className="subsection-label">NODE PORT ALLOCATIONS</div>
                  <small>{ports.length} configured</small>
                </div>
              </div>
              <div className="node-port-editor">
                <div className="allocation-heading">
                  <div>
                    <div className="subsection-label">ADD PORT</div>
                    <small>Ports are added individually to the node pool.</small>
                  </div>
                </div>
                <div className="node-port-add">
                  <label>
                    IP address
                    <select
                      value={portIP}
                      onChange={(e) => {
                        setPortIP(e.target.value);
                        setPortError("");
                      }}
                    >
                      {ips.map((ip) => (
                        <option key={ip} value={ip}>
                          {ip}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    Port number
                    <input
                      type="number"
                      min="1"
                      max="65535"
                      step="1"
                      value={singlePort}
                      onChange={(e) => {
                        setSinglePort(e.target.value);
                        setPortError("");
                      }}
                    />
                  </label>
                  <Button
                    variant="subtle"
                    size="tiny"
                    type="button"
                    onClick={addPort}
                  >
                    <Plus size={13} />
                    Add port
                  </Button>
                </div>
                {portError && <div className="form-error">{portError}</div>}
              </div>
              {ports.map((allocation, index) => (
                <div
                  className="allocation-pool-row"
                  key={`${allocation.ip}-${index}`}
                >
                  <label>
                    IP address
                    <select
                      required
                      value={allocation.ip}
                      onChange={(e) => updatePort(index, "ip", e.target.value)}
                    >
                      {ips.map((ip) => (
                        <option key={ip} value={ip}>
                          {ip}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label>
                    Port number
                    <input
                      required
                      type="number"
                      min="1"
                      max="65535"
                      value={allocation.port}
                      onChange={(e) =>
                        updatePort(index, "port", e.target.value)
                      }
                    />
                  </label>
                  <button
                    type="button"
                    className="icon-button"
                    title="Remove allocation"
                    onClick={() =>
                      setPorts((old) => old.filter((_, i) => i !== index))
                    }
                  >
                    <Trash size={14} />
                  </button>
                </div>
              ))}
              {!ports.length && (
                <div className="inline-empty">
                  No ports allocated yet. Add an IP and port before creating
                  servers.
                </div>
              )}
            </div>
            <Button variant="primary" disabled={busy}>
              <Save size={14} />
              Resolve DNS, save & open ports
            </Button>
          </div>
        </form>
      )}
    </div>
  );
}

/** Inspect containers and images reported by a node. */
export function NodeResourcesPage({
  node,
  servers,
  canManage,
  onChanged,
  onBack,
}: {
  node: NodeItem;
  servers: GameServer[];
  canManage: boolean;
  onChanged: () => void;
  onBack: () => void;
}) {
  const [resources, setResources] = useState<NodeResources>({
    containers: [],
    images: [],
  });
  const [selectedResource, setSelectedResource] = useState<
    | { type: "container"; value: NodeContainer }
    | { type: "image"; value: NodeImage }
    | null
  >(null);
  const [busy, setBusy] = useState(false);
  const confirmAction = useConfirmation();
  const load = useCallback(async () => {
    setBusy(true);
    try {
      setResources(await api<NodeResources>(`/api/nodes/${node.id}/resources`));
    } catch (error: unknown) {
      showToast(errorMessage(error));
    } finally {
      setBusy(false);
    }
  }, [node.id]);
  useEffect(() => {
    void load();
  }, [load]);

  const removeResource = async (
    kind: "container" | "image",
    id: string,
    label: string,
    serverName?: string,
  ) => {
    const isContainer = kind === "container";
    if (
      !(await confirmAction({
        title: `Delete ${kind}`,
        message: isContainer
          ? `Permanently remove container ${label}${serverName ? ` attached to ${serverName}` : ""}? Its server files and Docker volumes will be kept.`
          : `Remove image ${label} from this node? Docker will refuse if the image is still in use by a container.`,
        confirmLabel: `Delete ${kind}`,
        destructive: true,
      }))
    )
      return;

    setBusy(true);
    try {
      await api(
        `/api/nodes/${node.id}/${isContainer ? "containers" : "images"}/${encodeURIComponent(id)}`,
        { method: "DELETE" },
      );
      showToast(
        `${kind === "container" ? "Container" : "Image"} deleted.`,
        "success",
      );
      onChanged();
      await load();
    } catch (error: unknown) {
      showToast(errorMessage(error));
    } finally {
      setBusy(false);
    }
  };

  const selectedTitle =
    selectedResource?.type === "container"
      ? selectedResource.value.names?.[0]?.replace(/^\//, "") ||
        selectedResource.value.id.slice(0, 12)
      : selectedResource?.type === "image"
        ? selectedResource.value.tags
            ?.filter((tag) => tag !== "<none>:<none>")
            .join(", ") || "Untagged image"
        : "";

  return (
    <div className="page-stack">
      <button className="back-link" onClick={onBack}>
        <ArrowLeft size={15} />
        Back to {node.name}
      </button>
      <PageHeading
        eyebrow="NODE RESOURCES"
        title={node.name}
        description="Inspect and manage Docker containers and images downloaded on this node."
        action={
          <Button
            variant="subtle"
            disabled={busy || node.status !== "online"}
            onClick={() => void load()}
          >
            <RefreshCw size={14} />
            Refresh
          </Button>
        }
      />
      {node.status !== "online" && (
        <div className="inline-empty">
          This node is offline. Start the node agent to load its Docker
          resources.
        </div>
      )}
      <div className="node-resource-grid">
        <section className="panel-card node-resource-card">
          <PanelHeading
            title="Docker containers"
            sub={`${resources.containers.length} container${resources.containers.length === 1 ? "" : "s"}`}
          />
          {resources.containers.length ? (
            resources.containers.map((container) => {
              const server = servers.find(
                (item) =>
                  item.containerId === container.id ||
                  item.id === container.serverId,
              );
              const containerName =
                container.names?.[0]?.replace(/^\//, "") ||
                container.id.slice(0, 12);
              return (
                <div className="node-resource-row" key={container.id}>
                  <button
                    className="node-resource-select"
                    onClick={() =>
                      setSelectedResource({
                        type: "container",
                        value: container,
                      })
                    }
                  >
                    <div className="node-resource-main">
                      <strong>
                        {container.names
                          ?.map((name) => name.replace(/^\//, ""))
                          .join(", ") || container.id.slice(0, 12)}
                      </strong>
                      <small>{container.image}</small>
                      <small>
                        Server:{" "}
                        {container.serverName ||
                          server?.name ||
                          (container.serverId
                            ? `Unknown server (${container.serverId})`
                            : "Not attached to a managed server")}
                      </small>
                    </div>
                    <span className="role-pill">
                      {container.status || container.state}
                    </span>
                  </button>
                  {canManage && (
                    <button
                      className="icon-button danger-icon"
                      title="Delete container"
                      aria-label={`Delete container ${containerName}`}
                      disabled={busy || node.status !== "online"}
                      onClick={() =>
                        void removeResource(
                          "container",
                          container.id,
                          containerName,
                          container.serverName || server?.name,
                        )
                      }
                    >
                      <Trash size={15} />
                    </button>
                  )}
                </div>
              );
            })
          ) : (
            <div className="inline-empty">No containers.</div>
          )}
        </section>
        <section className="panel-card node-resource-card">
          <PanelHeading
            title="Downloaded images"
            sub={`${resources.images.length} image${resources.images.length === 1 ? "" : "s"}`}
          />
          {resources.images.length ? (
            resources.images.map((image) => (
              <div className="node-resource-row" key={image.id}>
                <button
                  className="node-resource-select"
                  onClick={() =>
                    setSelectedResource({ type: "image", value: image })
                  }
                >
                  <div className="node-resource-main">
                    <strong>
                      {image.tags
                        ?.filter((tag) => tag !== "<none>:<none>")
                        .join(", ") || "Untagged image"}
                    </strong>
                    <small>
                      {image.id.replace(/^sha256:/, "").slice(0, 16)}
                    </small>
                  </div>
                  <span className="role-pill">
                    {formatBytes(image.sizeBytes)}
                  </span>
                </button>
                {canManage && (
                  <button
                    className="icon-button danger-icon"
                    title="Delete image"
                    aria-label={`Delete image ${image.tags?.filter((tag) => tag !== "<none>:<none>").join(", ") || image.id.slice(0, 16)}`}
                    disabled={busy || node.status !== "online"}
                    onClick={() =>
                      void removeResource(
                        "image",
                        image.id,
                        image.tags
                          ?.filter((tag) => tag !== "<none>:<none>")
                          .join(", ") || image.id.slice(0, 16),
                      )
                    }
                  >
                    <Trash size={15} />
                  </button>
                )}
              </div>
            ))
          ) : (
            <div className="inline-empty">No downloaded images.</div>
          )}
        </section>
      </div>
      {selectedResource && (
        <Modal title={selectedTitle} onClose={() => setSelectedResource(null)}>
          <div className="modal-body details-card">
            {selectedResource.type === "container" ? (
              <ContainerResourceDetails
                container={selectedResource.value}
                server={servers.find(
                  (item) =>
                    item.containerId === selectedResource.value.id ||
                    item.id === selectedResource.value.serverId,
                )}
              />
            ) : (
              <ImageResourceDetails
                image={selectedResource.value}
                containers={resources.containers}
              />
            )}
          </div>
        </Modal>
      )}
    </div>
  );
}

function ContainerResourceDetails({
  container,
  server,
}: {
  container: NodeContainer;
  server?: GameServer;
}) {
  const online = container.state === "running";
  return (
    <>
      <Detail label="Owner">{container.ownerName || "Unknown"}</Detail>
      <Detail label="Server">
        {container.serverName ||
          server?.name ||
          "Not attached to a managed server"}
      </Detail>
      <Detail label="Status">
        <Status status={online ? "Online" : "Offline"} />
      </Detail>
      {!online && (
        <Detail label="Last online">
          {container.lastOnlineAt
            ? new Date(container.lastOnlineAt).toLocaleString()
            : "No recorded online time"}
        </Detail>
      )}
      <Detail label="Image">{container.image}</Detail>
      <Detail label="Container ID">
        <code>{container.id}</code>
      </Detail>
      <Detail label="Created">
        {new Date(container.created * 1000).toLocaleString()}
      </Detail>
    </>
  );
}

function ImageResourceDetails({
  image,
  containers,
}: {
  image: NodeImage;
  containers: NodeContainer[];
}) {
  const usingContainers = containers.filter(
    (container) => container.imageId === image.id,
  );
  const owners = [
    ...new Set(
      usingContainers
        .map((container) => container.ownerName)
        .filter((name): name is string => !!name),
    ),
  ];
  return (
    <>
      <Detail label="Tags">
        {image.tags?.filter((tag) => tag !== "<none>:<none>").join(", ") ||
          "Untagged image"}
      </Detail>
      <Detail label="Image ID">
        <code>{image.id}</code>
      </Detail>
      <Detail label="Size">{formatBytes(image.sizeBytes)}</Detail>
      <Detail label="Created">
        {new Date(image.created * 1000).toLocaleString()}
      </Detail>
      <Detail label="Containers using image">
        {usingContainers.length
          ? usingContainers
              .map(
                (container) =>
                  container.serverName ||
                  container.names?.[0]?.replace(/^\//, "") ||
                  container.id.slice(0, 12),
              )
              .join(", ")
          : "None"}
      </Detail>
      <Detail label="Owners">
        {owners.length ? owners.join(", ") : "No managed server owner"}
      </Detail>
    </>
  );
}

/** Plot resource history for a node using its reported metrics. */
export function NodeChart({
  data,
}: {
  data: { time: string; cpu: number; memory: number }[];
}) {
  return (
    <>
      <div className="node-chart-plot">
        <UsageChart data={data} />
      </div>
      <div className="chart-scale-hint">
        Shared scale · hover a point for exact usage
      </div>
      <div className="chart-legend">
        <span>
          <i className="cpu-key" />
          CPU
        </span>
        <span>
          <i className="memory-key" />
          Memory
        </span>
        <small>{data.at(-1)?.time || "No telemetry yet"}</small>
      </div>
    </>
  );
}
