import { FormEvent, useEffect, useState } from "react";
import {
  api,
  errorMessage,
  type Config,
  type GameServer,
  type NodeItem,
} from "../../shared/domain";
import { Field, FormActions } from "../../components";

/** Collect the node, configuration, limits, and required variables for a server. */
export function ServerForm({
  nodes,
  configs,
  servers,
  onCreated,
  onCancel,
}: {
  nodes: NodeItem[];
  configs: Config[];
  servers: GameServer[];
  onCreated: (server: GameServer) => void;
  onCancel: () => void;
}) {
  const [name, setName] = useState("");
  const [nodeId, setNodeId] = useState(
    nodes.find((n) => n.status === "online")?.id || "",
  );
  const [configId, setConfigId] = useState(configs[0]?.id || "");
  const [cpu, setCpu] = useState("2");
  const [memory, setMemory] = useState("4096");
  const [diskGB, setDiskGB] = useState("4");
  const [variables, setVariables] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const selected = configs.find((t) => t.id === configId);
  const selectedNode = nodes.find((node) => node.id === nodeId);
  const defs: Array<{
    name: string;
    type?: string;
    default?: string;
    required?: boolean;
    ui?: boolean;
    description?: string;
  }> = selected?.spec?.variables || [];
  const shownDefs = defs.filter((v) => v.required === true || v.ui === true);
  const duplicateName =
    name.trim() !== "" &&
    servers.some(
      (server) =>
        server.name.trim().toLowerCase() === name.trim().toLowerCase(),
    );
  useEffect(() => {
    setVariables(
      Object.fromEntries(defs.map((v) => [v.name, String(v.default ?? "")])),
    );
    const resources = selected?.spec?.resources || {};
    setCpu(String(resources.cpuLimit ?? 2));
    setMemory(String(resources.memoryMB ?? 4096));
  }, [configId]);
  const online = nodes.filter((n) => n.status === "online");
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (duplicateName) {
      setError("A server with this name already exists.");
      return;
    }
    setBusy(true);
    setError("");
    try {
      const server = await api<GameServer>("/api/servers", {
        method: "POST",
        body: JSON.stringify({
          name,
          nodeId,
          configId,
          cpuLimit: Number(cpu),
          memoryMB: Number(memory),
          diskSizeBytes: Math.round(Number(diskGB) * 1024 ** 3),
          variables,
        }),
      });
      onCreated(server);
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="form-stack modal-body" onSubmit={submit}>
      <p className="modal-intro">
        The node runs the Config install script against persistent server files,
        then starts the game container with the Config command and port
        mappings.
      </p>
      <Field label="Server name">
        <input
          autoFocus
          required
          placeholder="e.g. survival-01"
          value={name}
          onChange={(e) => {
            setName(e.target.value);
            setError("");
          }}
        />
      </Field>
      {duplicateName && (
        <div className="form-error">
          A server with this name already exists.
        </div>
      )}
      <div className="form-two">
        <Field label="Online node">
          <select
            required
            value={nodeId}
            onChange={(e) => setNodeId(e.target.value)}
          >
            <option value="">Choose node</option>
            {online.map((n) => (
              <option key={n.id} value={n.id}>
                {n.name}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Config">
          <select
            required
            value={configId}
            onChange={(e) => setConfigId(e.target.value)}
          >
            <option value="">Choose config</option>
            {configs.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name}
              </option>
            ))}
          </select>
        </Field>
      </div>
      <Field label="Server data disk (GB)">
        <input
          type="number"
          min="0.125"
          step="0.125"
          value={diskGB}
          onChange={(e) => setDiskGB(e.target.value)}
          required
        />
        <small className="field-hint">
          {selectedNode?.stats?.storageMode === "docker-volume"
            ? "Mac local testing uses a Docker volume; Docker Desktop does not enforce this size. Linux nodes enforce the configured disk limit."
            : "Allocated as a size-limited virtual disk on the selected node. Free space is checked when deployment starts."}
        </small>
      </Field>
      <div className="form-two">
        <Field label="CPU limit (cores)">
          <input
            type="number"
            min="0.25"
            step="0.25"
            value={cpu}
            onChange={(e) => setCpu(e.target.value)}
          />
        </Field>
        <Field label="Memory (MB)">
          <input
            type="number"
            min="256"
            step="256"
            value={memory}
            onChange={(e) => setMemory(e.target.value)}
          />
        </Field>
      </div>
      {shownDefs.length > 0 && (
        <div className="variable-list">
          <div className="subsection-label">CONFIG VARIABLES</div>
          {shownDefs.map((v) => (
            <Field
              key={v.name}
              label={v.name + (v.required ? " · required" : "")}
            >
              <input
                type={
                  v.type === "secret"
                    ? "password"
                    : v.type === "integer"
                      ? "number"
                      : "text"
                }
                required={v.required === true}
                value={variables[v.name] ?? ""}
                onChange={(e) =>
                  setVariables({ ...variables, [v.name]: e.target.value })
                }
              />
              {v.description && (
                <small className="field-hint">{v.description}</small>
              )}
            </Field>
          ))}
        </div>
      )}
      {!online.length && (
        <div className="form-hint warning">
          Connect a node agent before deploying a server.
        </div>
      )}
      {error && <div className="form-error">{error}</div>}
      <FormActions
        cancel={onCancel}
        busy={busy}
        label="Create and install"
        disabled={!online.length || !configs.length || duplicateName}
      />
    </form>
  );
}
