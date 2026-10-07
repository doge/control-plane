import { Button } from "../../components/Button";
import { FormEvent, useEffect, useState } from "react";
import {
  Activity,
  ArrowLeft,
  Box,
  ChevronRight,
  Cpu,
  FileCode2,
  Globe,
  MemoryStick,
  Pause,
  Play,
  Plus,
  RefreshCw,
  Save,
  Server,
  Settings,
  Shield,
  Square,
  Terminal,
  Trash,
  X,
} from "lucide-react";

import {
  errorMessage,
  api,
  type GameServer,
  type ConfigVariable,
  type NodeItem,
  type ServerLaunchInfo,
  type Config,
} from "../../shared/domain";

import { useConfirmation } from "../../shared/dialogs";
import { showToast } from "../../shared/toast";
import { ServerWorkspace } from "../files/server-files";
import {
  Detail,
  Empty,
  Entity,
  Field,
  formatBytes,
  PageHeading,
  PanelHeading,
  Quick,
  ServerRow,
  Stat,
  Status,
  TableWrap,
  UsageChart,
  type UsagePoint,
} from "../../components";

type DashboardProps = {
  servers: GameServer[];
  nodes: NodeItem[];
  configs: Config[];
  nodeByID: Map<string, NodeItem>;
  configByID: Map<string, Config>;
  onCreate: (kind: "server" | "node" | "config") => void;
  onOpen: (server: GameServer) => void;
  onNavigate: (page: string) => void;
  onNode: (node: NodeItem) => void;
  canCreateServer?: boolean;
  canManageNodes?: boolean;
  canManageConfigs?: boolean;
  canViewNodes?: boolean;
  canViewServers?: boolean;
};

/** Summarize server, node, and configuration health for the signed-in user. */
export function Dashboard({
  servers,
  nodes,
  configs,
  nodeByID,
  configByID,
  onCreate,
  onOpen,
  onNavigate,
  onNode,
  canCreateServer = false,
  canManageNodes = false,
  canManageConfigs = false,
  canViewNodes = true,
  canViewServers = true,
}: DashboardProps) {
  const running = servers.filter(
    (s: GameServer) => s.status === "running",
  ).length;
  const online = nodes.filter((n) => n.status === "online").length;
  const memory = servers
    .filter((s) => s.status === "running")
    .reduce((sum, s) => sum + (s.memoryMB || 0), 0);
  return (
    <div className="page-stack">
      <PageHeading
        eyebrow="OVERVIEW"
        title="Dashboard"
        description="A clear view of your game server fleet."
        action={
          canCreateServer ? (
            <Button
              variant="primary"

              onClick={() => onCreate("server")}
            >
              <Plus size={16} />
              New server
            </Button>
          ) : undefined
        }
      />
      <div className="stat-grid">
        <Stat
          icon={Activity}
          label="Running servers"
          value={running}
          detail={servers.length + " total"}
          tone="blue"
        />
        <Stat
          icon={Globe}
          label="Online nodes"
          value={online}
          detail={nodes.length + " configured"}
          tone="green"
        />
        <Stat
          icon={Box}
          label="Configs"
          value={configs.length}
          detail="Ready to deploy"
          tone="purple"
        />
        <Stat
          icon={MemoryStick}
          label="Allocated memory"
          value={memory.toLocaleString() + " MB"}
          detail="Running servers"
          tone="orange"
        />
      </div>
      <div className="dashboard-grid">
        {canViewServers && (
          <section className="panel-card">
            <PanelHeading
              title="Game servers"
              sub="Your latest deployments"
              action={
                canCreateServer ? (
                  <Button
                    variant="subtle"

                    onClick={() => onNavigate("Servers")}
                  >
                    View all <ChevronRight size={15} />
                  </Button>
                ) : undefined
              }
            />
            {servers.length ? (
              <div className="data-list">
                {servers.slice(0, 6).map((s: GameServer) => (
                  <ServerRow
                    key={s.id}
                    server={s}
                    node={nodeByID.get(s.nodeId)}
                    config={configByID.get(s.configId)}
                    onClick={() => onOpen(s)}
                  />
                ))}
              </div>
            ) : (
              <Empty
                icon={Server}
                title="No servers yet"
                text="Create a node, choose a Config, and deploy your first game server."
                action={
                  canCreateServer ? (
                    <Button
                      variant="primary"

                      onClick={() => onCreate("server")}
                    >
                      <Plus size={15} />
                      Create server
                    </Button>
                  ) : undefined
                }
              />
            )}
          </section>
        )}
        {(canManageNodes || canManageConfigs || canCreateServer) && (
          <section className="panel-card">
            <PanelHeading title="Quick actions" sub="Set up your fleet" />
            {canManageNodes && (
              <Quick
                icon={Globe}
                title="Add a node"
                text="Connect a Docker host"
                onClick={() => onCreate("node")}
              />
            )}
            {canManageConfigs && (
              <Quick
                icon={FileCode2}
                title="Create a Config"
                text="Define an image and ports"
                onClick={() => onCreate("config")}
              />
            )}
            {canCreateServer && (
              <Quick
                icon={Server}
                title="Deploy a server"
                text="Launch a Config on a node"
                onClick={() => onCreate("server")}
              />
            )}
            {(canManageNodes || canManageConfigs) && (
              <div className="tip">
                <Shield size={17} />
                Node tokens are shown once. Store them privately on the node
                host.
              </div>
            )}
          </section>
        )}
      </div>
      {canViewNodes && (
        <section className="panel-card">
          <PanelHeading
            title="Nodes"
            sub="Hosts connected to your panel"
            action={
              <Button
                variant="subtle"

                onClick={() => onNavigate("Nodes")}
              >
                Manage nodes <ChevronRight size={15} />
              </Button>
            }
          />
          {nodes.length ? (
            <div className="node-strip">
              {nodes.slice(0, 5).map((n: NodeItem) => (
                <button
                  className="node-chip node-chip-button"
                  key={n.id}
                  onClick={() => onNode(n)}
                >
                  <span
                    className={
                      "status-dot " + (n.status === "online" ? "online" : "")
                    }
                  />
                  <div>
                    <strong>{n.name}</strong>
                    <small>{n.region || "Region not set"}</small>
                  </div>
                  <Status status={n.status} />
                </button>
              ))}
            </div>
          ) : (
            <div className="inline-empty">
              No nodes connected. Add a node to host servers.
            </div>
          )}
        </section>
      )}
    </div>
  );
}

type ServersPageProps = {
  servers: GameServer[];
  nodes: NodeItem[];
  configs: Config[];
  nodeByID: Map<string, NodeItem>;
  configByID: Map<string, Config>;
  onCreate: () => void;
  onOpen: (server: GameServer) => void;
  onEdit: (server: GameServer) => void;
  canCreate?: boolean;
  canUpdate?: boolean;
  canConsole?: boolean;
};

/** List managed servers and provide navigation to their details. */
/** List servers and expose permitted create, edit, and detail actions. */
export function ServersPage({
  servers,
  nodes,
  configs,
  nodeByID,
  configByID,
  onCreate,
  onOpen,
  onEdit,
  canCreate = false,
  canUpdate = false,
  canConsole = false,
}: ServersPageProps) {
  return (
    <div className="page-stack">
      <PageHeading
        eyebrow="FLEET"
        title="Servers"
        description="Deploy and inspect game servers across your nodes."
        action={
          canCreate ? (
            <Button variant="primary" onClick={onCreate}>
              <Plus size={16} />
              Create server
            </Button>
          ) : undefined
        }
      />
      {servers.length ? (
        <section className="panel-card table-card">
          <TableWrap>
            <table>
              <thead>
                <tr>
                  <th>SERVER</th>
                  <th>STATUS</th>
                  <th>NODE</th>
                  <th>ADDRESS</th>
                  <th>CONFIG</th>
                  <th>MEMORY</th>
                  <th>CPU</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {servers.map((s: GameServer) => {
                  const addr =
                    s.allocations?.[0]?.ip ||
                    s.address ||
                    nodeByID.get(s.nodeId)?.ips?.[0] ||
                    nodeByID.get(s.nodeId)?.address ||
                    "—";
                  const port = s.allocations?.[0]?.host;
                  return (
                    <tr
                      key={s.id}
                      className="click-row"
                      onClick={() => onOpen(s)}
                    >
                      <td>
                        <Entity
                          icon={Server}
                          title={s.name}
                          sub={s.id.slice(-8)}
                        />
                      </td>
                      <td>
                        <Status
                          status={
                            nodeByID.get(s.nodeId)?.status === "online"
                              ? s.status
                              : "offline"
                          }
                        />
                      </td>
                      <td>{nodeByID.get(s.nodeId)?.name || "Unknown node"}</td>
                      <td>
                        <code>
                          {addr}
                          {port ? `:${port}` : ""}
                        </code>
                      </td>
                      <td>
                        {configByID.get(s.configId)?.name || "Unknown config"}
                      </td>
                      <td>{(s.memoryMB || 0).toLocaleString()} MB</td>
                      <td>{s.cpuLimit} cores</td>
                      <td className="server-row-actions">
                        {canUpdate && (
                          <Button
                            variant="subtle"
                            size="tiny"

                            onClick={(e) => {
                              e.stopPropagation();
                              onEdit(s);
                            }}
                          >
                            <Settings size={13} />
                            Edit
                          </Button>
                        )}
                        {canConsole && (
                          <Button
                            variant="subtle"
                            size="tiny"

                            onClick={(e) => {
                              e.stopPropagation();
                              onOpen(s);
                            }}
                          >
                            <Terminal size={14} />
                            Console
                          </Button>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </TableWrap>
        </section>
      ) : (
        <Empty
          icon={Server}
          title="No servers configured"
          text={
            !nodes.length
              ? "Register and connect a node before deploying."
              : !configs.length
                ? "Create a game Config before deploying."
                : "Create your first server from a Config."
          }
          action={
            canCreate ? (
              <Button variant="primary" onClick={onCreate}>
                <Plus size={15} />
                Create server
              </Button>
            ) : undefined
          }
        />
      )}
    </div>
  );
}
/** Show server status, controls, resource usage, and its workspace. */
export function ServerDetail({
  server,
  node,
  config,
  editConfig,
  onEditConfig,
  onBack,
  onDeleted,
  permissions = [],
}: {
  server: GameServer;
  node?: NodeItem;
  config?: Config;
  editConfig: boolean;
  onEditConfig: (v: boolean) => void;
  onBack: () => void;
  onDeleted: () => void;
  permissions?: string[];
}) {
  const confirmAction = useConfirmation();
  const [current, setCurrent] = useState(server);
  const [busy, setBusy] = useState(false);
  const [deploying, setDeploying] = useState(false);
  useEffect(
    () => setCurrent(server),
    [server.id, server.status, server.containerId],
  );
  const action = async (name: string) => {
    setBusy(true);
    const isDeployment = name === "deploy";
    if (isDeployment) setDeploying(true);
    try {
      const updated = await api<GameServer>(
        `/api/servers/${current.id}/actions`,
        { method: "POST", body: JSON.stringify({ action: name }) },
      );
      setCurrent(updated);
    } catch (e: unknown) {
      if (isDeployment) {
        setCurrent({
          ...current,
          status: "error",
          containerId: undefined,
          error: errorMessage(e),
        });
      }
      showToast(errorMessage(e));
    } finally {
      if (isDeployment) setDeploying(false);
      setBusy(false);
    }
  };
  const noContainer = !current.containerId;
  const deleteServer = async () => {
    if (
      !(await confirmAction({
        title: "Delete server deployment",
        message: `Delete ${current.name} and its backups? This removes its container and backup images.`,
        confirmLabel: "Delete deployment",
        destructive: true,
      }))
    )
      return;
    setBusy(true);
    try {
      await api(`/api/servers/${current.id}`, { method: "DELETE" });
      onDeleted();
    } catch (e: unknown) {
      showToast(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const address =
    current.allocations?.[0]?.ip ||
    current.address ||
    node?.ips?.[0] ||
    node?.address ||
    "IP not configured";
  const nodeOnline = node?.status === "online";
  const displayStatus = nodeOnline ? current.status : "offline";
  const portList =
    current.allocations?.map((a) => `${a.ip}:${a.host}`).join(", ") ||
    "No port allocations";
  return (
    <div className="page-stack">
      <button className="back-link" onClick={onBack}>
        <ArrowLeft size={15} /> Back to servers
      </button>
      <PageHeading
        eyebrow="GAME SERVER"
        title={current.name}
        description={
          (config?.name || "Game server") +
          " · " +
          (node?.name || "Unknown node")
        }
        action={<Status status={displayStatus} />}
      />
      {nodeOnline && (current.status === "installing" || deploying) && (
        <div className="server-install-state" role="status" aria-live="polite">
          <span className="spinner" aria-hidden="true" />
          <div>
            <strong>
              {deploying ? "Deploying server" : "Installing server"}
            </strong>
            <p>
              {deploying
                ? "The node is preparing the server and starting its container. Console output will appear when it is running."
                : "The node is preparing the game files and starting the container. The console will connect when it is running."}
            </p>
          </div>
        </div>
      )}
      <div className="server-actions">
        {permissions.includes("servers.control") &&
          (noContainer ? (
            <Button
              variant="primary"

              disabled={
                busy ||
                deploying ||
                current.status === "installing" ||
                node?.status !== "online"
              }
              onClick={() => action("deploy")}
            >
              {deploying ? (
                <span className="spinner" aria-hidden="true" />
              ) : (
                <Play size={15} />
              )}
              {deploying ? "Deploying…" : "Deploy server"}
            </Button>
          ) : (
            <>
              <Button
                variant="primary"

                disabled={busy || !nodeOnline || current.status === "running"}
                onClick={() => action("start")}
              >
                <Play size={15} />
                Start
              </Button>
              {current.status === "paused" ? (
                <Button
                  variant="subtle"

                  disabled={busy || !nodeOnline}
                  onClick={() => action("resume")}
                >
                  <Play size={15} />
                  Resume
                </Button>
              ) : (
                <Button
                  variant="subtle"

                  disabled={busy || !nodeOnline || current.status !== "running"}
                  onClick={() => action("pause")}
                >
                  <Pause size={15} />
                  Pause
                </Button>
              )}
              <Button
                variant="danger"

                disabled={busy || !nodeOnline || current.status === "stopped"}
                onClick={() => action("kill")}
              >
                <Square size={15} />
                Kill
              </Button>
              <Button
                variant="subtle"

                title="Recreate the container while preserving server files"
                disabled={busy || node?.status !== "online"}
                onClick={() => action("deploy")}
              >
                {deploying ? (
                  <span className="spinner" aria-hidden="true" />
                ) : (
                  <RefreshCw size={15} />
                )}
                {deploying ? "Deploying…" : "Recreate"}
              </Button>
            </>
          ))}
        {permissions.includes("servers.delete") && (
          <Button
            variant="danger"

            disabled={busy || current.status === "installing"}
            onClick={deleteServer}
          >
            <Trash size={15} />
            Delete deployment
          </Button>
        )}
      </div>
      <div className="server-overview-grid">
        <section className="panel-card server-overview">
          <PanelHeading
            title="Connection"
            sub="Use an allocation address and port from your game client"
          />
          <Detail label="Address">{address}</Detail>
          <Detail label="Ports">{portList}</Detail>
          <Detail label="Node">{node?.name || "Unknown node"}</Detail>
          {current.allocations?.some(
            (a) => a.ip === "127.0.0.1" || a.ip === "::1",
          ) && (
            <div className="loopback-note">
              Loopback addresses only accept connections from the node itself.
            </div>
          )}
          {permissions.includes("servers.update") && (
            <div className="server-config-trigger">
              <Button
                variant="subtle"

                onClick={() => onEditConfig(true)}
              >
                <Settings size={14} />
                Edit configuration
              </Button>
            </div>
          )}
        </section>
        <ServerUsagePanel server={current} config={config} node={node} />
      </div>
      {editConfig && permissions.includes("servers.update") && (
        <ServerConfiguration
          server={current}
          node={node}
          config={config}
          onSaved={(updated) => {
            setCurrent(updated);
            onEditConfig(false);
          }}
          onCancel={() => onEditConfig(false)}
        />
      )}
      {(permissions.includes("servers.console") ||
        permissions.includes("servers.files.view") ||
        permissions.includes("servers.backups.view")) && (
        <div className="server-detail-grid">
          <ServerWorkspace
            server={current}
            node={node}
            root={config?.spec?.dataDirectory || "/home/container"}
            permissions={permissions}
            deploying={deploying}
          />
        </div>
      )}
    </div>
  );
}
/** Poll live resource information and display recent server usage. */
export function ServerUsagePanel({
  server,
  config,
  node,
}: {
  server: GameServer;
  config?: Config;
  node?: NodeItem;
}) {
  const [info, setInfo] = useState<ServerLaunchInfo>({});
  const [error, setError] = useState("");
  const [history, setHistory] = useState<UsagePoint[]>([]);
  useEffect(() => {
    let live = true;
    if (!server.containerId) {
      setInfo({});
      setHistory([]);
      return;
    }
    const load = () =>
      api<ServerLaunchInfo>(`/api/servers/${server.id}/launch`)
        .then((v) => {
          if (live) {
            setInfo(v);
            setError("");
            const memoryLimit =
              v.memoryLimitBytes || server.memoryMB * 1024 * 1024;
            setHistory((old) =>
              [
                ...old,
                {
                  time: new Date().toLocaleTimeString(),
                  cpu: Math.min(
                    100,
                    (v.cpuPercent || 0) / Math.max(0.25, server.cpuLimit || 1),
                  ),
                  memory: memoryLimit
                    ? Math.min(
                        100,
                        ((v.memoryUsedBytes || 0) / memoryLimit) * 100,
                      )
                    : 0,
                },
              ].slice(-60),
            );
          }
        })
        .catch((e) => live && setError(errorMessage(e)));
    void load();
    const timer = window.setInterval(() => void load(), 5000);
    return () => {
      live = false;
      window.clearInterval(timer);
    };
  }, [
    server.id,
    server.containerId,
    server.status,
    server.cpuLimit,
    server.memoryMB,
    node?.status,
  ]);
  const entry = Array.isArray(info.entrypoint) ? info.entrypoint.join(" ") : "";
  const cmd = Array.isArray(info.command) ? info.command.join(" ") : "";
  const launch =
    server.launchCommand ||
    info.gameCommand ||
    config?.spec?.startup ||
    info.configCommand ||
    [entry, cmd].filter(Boolean).join(" ") ||
    "Waiting for the game process command";
  const memoryLimit = info.memoryLimitBytes || server.memoryMB * 1024 * 1024;
  const cpuPercent = Math.min(
    100,
    (info.cpuPercent || 0) / Math.max(0.25, server.cpuLimit || 1),
  );
  return (
    <section className="panel-card launch-summary server-usage-panel">
      <PanelHeading
        title="CPU and memory usage"
        sub={
          info.running
            ? `Live · updated ${history.at(-1)?.time || "just now"}`
            : "Server resource usage"
        }
      />
      <div className="server-live-usage">
        <span>
          <Cpu size={14} />
          CPU <b>{info.running ? `${cpuPercent.toFixed(1)}%` : "0%"}</b>
        </span>
        <span>
          <MemoryStick size={14} />
          Memory{" "}
          <b>
            {formatBytes(info.memoryUsedBytes || 0)} /{" "}
            {formatBytes(memoryLimit)}
          </b>
        </span>
      </div>
      <div className="server-usage-chart">
        <UsageChart data={history} />
      </div>
      <div className="chart-scale-hint">
        Shared scale · hover a point for exact usage
      </div>
      <details className="launch-command-details">
        <summary>Launch command</summary>
        <pre>{launch}</pre>
      </details>
      {error && <small className="field-hint">{error}</small>}
    </section>
  );
}
/** Edit a server's resource limits, variables, allocations, and launch command. */
export function ServerConfiguration({
  server,
  node,
  config,
  onSaved,
  onCancel,
}: {
  server: GameServer;
  node?: NodeItem;
  config?: Config;
  onSaved: (s: GameServer) => void;
  onCancel: () => void;
}) {
  const confirmAction = useConfirmation();
  const [cpu, setCpu] = useState(String(server.cpuLimit || 2));
  const [memory, setMemory] = useState(String(server.memoryMB || 4096));
  const [variables, setVariables] = useState<Record<string, string>>(
    server.variables || {},
  );
  const [allocations, setAllocations] = useState<
    NonNullable<GameServer["allocations"]>
  >(server.allocations || []);
  const [launchCommand, setLaunchCommand] = useState(
    server.launchCommand || config?.spec?.startup || "",
  );
  const [launchEdited, setLaunchEdited] = useState(false);
  const [busy, setBusy] = useState(false),
    [loading, setLoading] = useState(true),
    [error, setError] = useState("");
  const defs = config?.spec?.variables || [];
  const nodePorts = node?.portAllocations || [];
  const ips = [...new Set(nodePorts.map((p) => p.ip).concat(node?.ips || []))];
  useEffect(() => {
    let live = true;
    setLoading(true);
    api<GameServer>(`/api/servers/${server.id}/config`)
      .then(async (fresh) => {
        if (!live) return;
        setCpu(String(fresh.cpuLimit || 2));
        setMemory(String(fresh.memoryMB || 4096));
        setVariables(fresh.variables || {});
        setAllocations(fresh.allocations || []);
        const effectiveCommand =
          fresh.launchCommand || config?.spec?.startup || "";
        if (live) {
          setLaunchCommand(effectiveCommand);
          setLaunchEdited(false);
        }
      })
      .catch((e) => live && setError(errorMessage(e)))
      .finally(() => live && setLoading(false));
    return () => {
      live = false;
    };
  }, [server.id, config?.id]);
  type Allocation = NonNullable<GameServer["allocations"]>[number];
  const updateAllocation = (
    index: number,
    key: keyof Allocation,
    value: string | number,
  ) =>
    setAllocations((old) =>
      old.map((a, i) => (i === index ? { ...a, [key]: value } : a)),
    );
  const addAllocation = () => {
    const next =
      nodePorts.find(
        (p) =>
          !allocations.some(
            (a) =>
              a.ip === p.ip &&
              Number(a.host) === p.port &&
              (a.protocol || "tcp") === "tcp",
          ),
      ) || nodePorts[0];
    setAllocations((old) => [
      ...old,
      {
        name: "Additional",
        ip: next?.ip || ips[0] || "",
        host: next?.port || 0,
        container: 25565,
        protocol: "tcp",
      },
    ]);
  };
  const save = async (e: FormEvent) => {
    e.preventDefault();
    if (
      server.containerId &&
      !(await confirmAction({
        title: "Apply server configuration",
        message:
          "This recreates the container and interrupts a running game. Server files are preserved.",
        confirmLabel: "Save and apply",
        destructive: false,
      }))
    )
      return;
    setBusy(true);
    setError("");
    try {
      const result = await api<GameServer>(`/api/servers/${server.id}/config`, {
        method: "PUT",
        body: JSON.stringify({
          cpuLimit: Number(cpu),
          memoryMB: Number(memory),
          variables,
          launchCommand: launchEdited
            ? launchCommand
            : server.launchCommand || "",
          allocations: allocations.map((a) => ({
            ...a,
            container: Number(a.container),
            host: Number(a.host),
          })),
        }),
      });
      onSaved(result);
    } catch (e: unknown) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="panel-card server-config-form" onSubmit={save}>
      <div className="config-title-row">
        <PanelHeading
          title="Edit server configuration"
          sub="Changes recreate the container and preserve server data"
        />
        <button
          type="button"
          className="icon-button"
          title="Close editor"
          onClick={onCancel}
        >
          <X size={16} />
        </button>
      </div>
      {loading ? (
        <div className="console-placeholder">Loading server settings…</div>
      ) : (
        <>
          <Field label="Game process command">
            <textarea
              className="launch-command-input"
              rows={4}
              value={launchCommand}
              placeholder="Use the config default or enter the game executable/JAR command"
              onChange={(e) => {
                setLaunchCommand(e.target.value);
                setLaunchEdited(true);
              }}
              spellCheck={false}
            />
            <small className="field-hint">
              Shows the game process command, separate from the Docker image’s
              bootstrap entrypoint. Editing it replaces that entrypoint and runs
              this command directly; any JAR or setup files it needs must
              already exist in the container data volume.
            </small>
          </Field>
          <div className="form-two">
            <Field label="Memory limit (MB)">
              <input
                type="number"
                min="256"
                step="256"
                required
                value={memory}
                onChange={(e) => setMemory(e.target.value)}
              />
            </Field>
            <Field label="CPU limit (cores)">
              <input
                type="number"
                min="0.25"
                step="0.25"
                required
                value={cpu}
                onChange={(e) => setCpu(e.target.value)}
              />
            </Field>
          </div>
          {defs.length > 0 && (
            <div className="variable-list">
              <div className="subsection-label">CONFIG VARIABLES</div>
              {defs.map((v: ConfigVariable) => (
                <Field key={v.name} label={v.name}>
                  <input
                    type={
                      v.type === "secret"
                        ? "password"
                        : v.type === "integer"
                          ? "number"
                          : "text"
                    }
                    value={variables[v.name] ?? v.default ?? ""}
                    onChange={(e) =>
                      setVariables({
                        ...variables,
                        [v.name]: e.target.value,
                      })
                    }
                  />
                </Field>
              ))}
            </div>
          )}
          <div className="ip-pool-list">
            <div className="allocation-heading">
              <div>
                <div className="subsection-label">PORT ALLOCATIONS</div>
                <small>
                  Every host IP and port is checked against the node pool.
                </small>
              </div>
              <Button
                variant="subtle"
                size="tiny"
                type="button"

                onClick={addAllocation}
              >
                <Plus size={13} />
                Add allocation
              </Button>
            </div>
            {allocations.map((a: Allocation, i: number) => (
              <div
                className="server-allocation-row"
                key={`${a.ip}-${a.host}-${i}`}
              >
                <Field label="Allocation name">
                  <input
                    value={a.name || ""}
                    onChange={(e) =>
                      updateAllocation(i, "name", e.target.value)
                    }
                  />
                </Field>
                <Field label="Node IP">
                  <select
                    required
                    value={a.ip}
                    onChange={(e) => {
                      const ip = e.target.value;
                      const port = nodePorts.find(
                        (p) =>
                          p.ip === ip &&
                          !allocations.some(
                            (x, n) =>
                              n !== i &&
                              x.ip === ip &&
                              Number(x.host) === p.port,
                          ),
                      );
                      setAllocations((old) =>
                        old.map((x, n) =>
                          n === i ? { ...x, ip, host: port?.port || 0 } : x,
                        ),
                      );
                    }}
                  >
                    {ips.map((ip) => (
                      <option key={ip} value={ip}>
                        {ip}
                      </option>
                    ))}
                  </select>
                </Field>
                <Field label="Host port">
                  <select
                    required
                    value={a.host}
                    onChange={(e) =>
                      updateAllocation(i, "host", Number(e.target.value))
                    }
                  >
                    {nodePorts
                      .filter((p) => p.ip === a.ip)
                      .map((p) => (
                        <option key={p.port} value={p.port}>
                          {p.port}
                        </option>
                      ))}
                  </select>
                </Field>
                <Field label="Container port">
                  <input
                    required
                    type="number"
                    min="1"
                    max="65535"
                    value={a.container}
                    onChange={(e) =>
                      updateAllocation(i, "container", Number(e.target.value))
                    }
                  />
                </Field>
                <Field label="Protocol">
                  <select
                    value={a.protocol || "tcp"}
                    onChange={(e) =>
                      updateAllocation(i, "protocol", e.target.value)
                    }
                  >
                    <option value="tcp">TCP</option>
                    <option value="udp">UDP</option>
                    <option value="sctp">SCTP</option>
                  </select>
                </Field>
                <button
                  type="button"
                  className="icon-button allocation-remove"
                  title="Remove allocation"
                  onClick={() =>
                    setAllocations((old) => old.filter((_, n) => n !== i))
                  }
                >
                  <Trash size={15} />
                </button>
              </div>
            ))}
            {!allocations.length && (
              <div className="inline-empty">
                No ports assigned. Add an allocation to make this server
                reachable.
              </div>
            )}
          </div>
          {error && <div className="form-error">{error}</div>}
          <div className="config-actions">
            <Button
              variant="subtle"
              type="button"

              onClick={onCancel}
            >
              Cancel
            </Button>
            <Button
              variant="primary"

              disabled={busy || loading || !node || node.status !== "online"}
            >
              <Save size={14} />
              {busy ? "Applying…" : "Save and apply"}
            </Button>
          </div>
        </>
      )}
    </form>
  );
}
