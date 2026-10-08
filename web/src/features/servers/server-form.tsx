import { FormEvent, useEffect, useState } from "react";
import {
  api,
  errorMessage,
  type Config,
  type GameServer,
  type NodeItem,
  isLoopbackIP,
} from "../../shared/domain";
import { Field, FormActions, ResourceSlider } from "../../components";

const CPU_STEP = 0.25;
const CPU_UNITS_PER_CORE = 1 / CPU_STEP;
const MEMORY_STEP_MB = 256;
const STORAGE_STEP_BYTES = 128 * 1024 ** 2;
const STORAGE_VOLUME_OVERHEAD = STORAGE_STEP_BYTES;

/** Format quarter-core units for the resource slider. */
function formatCores(units: number) {
  const cores = units * CPU_STEP;
  return `${cores.toFixed(2).replace(/\.00$/, "")} ${cores === 1 ? "core" : "cores"}`;
}

/** Format 256 MB units with compact GB labels for larger values. */
function formatMemory(units: number) {
  const gib = (units * MEMORY_STEP_MB) / 1024;
  return gib >= 1
    ? `${gib.toFixed(2).replace(/0+$/, "").replace(/\.$/, "")} GB`
    : `${units * MEMORY_STEP_MB} MB`;
}

/** Format 128 MB storage units for the server disk slider. */
function formatStorage(units: number) {
  const gib = (units * STORAGE_STEP_BYTES) / 1024 ** 3;
  return `${gib.toFixed(gib % 1 ? 3 : 0).replace(/0+$/, "").replace(/\.$/, "")} GB`;
}

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
  const [cpuUnits, setCpuUnits] = useState(2 * CPU_UNITS_PER_CORE);
  const [memoryUnits, setMemoryUnits] = useState(4096 / MEMORY_STEP_MB);
  const [storageUnits, setStorageUnits] = useState(4 * 1024 / 128);
  const [variables, setVariables] = useState<Record<string, string>>({});
  const [portSelections, setPortSelections] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const selected = configs.find((t) => t.id === configId);
  const selectedNode = nodes.find((node) => node.id === nodeId);
  const usage = selectedNode?.resourceUsage || {
    cpuCores: 0,
    memoryMB: 0,
    storageBytes: 0,
  };
  const cpuTotalUnits = Math.floor(
    (selectedNode?.stats?.cpuThreads || 0) * CPU_UNITS_PER_CORE,
  );
  const cpuAvailableUnits = Math.max(
    0,
    Math.floor(
      ((selectedNode?.stats?.cpuThreads || 0) - usage.cpuCores) *
        CPU_UNITS_PER_CORE +
        1e-7,
    ),
  );
  const memoryTotalMB = Math.floor((selectedNode?.stats?.memoryTotalBytes || 0) / 1024 ** 2);
  const memoryTotalUnits = Math.floor(memoryTotalMB / MEMORY_STEP_MB);
  const memoryUsedMB = Math.floor((selectedNode?.stats?.memoryUsedBytes || 0) / 1024 ** 2);
  const memoryAvailableUnits = Math.max(
    0,
    Math.floor(
      Math.min(memoryTotalMB - usage.memoryMB, memoryTotalMB - memoryUsedMB) /
        MEMORY_STEP_MB,
    ),
  );
  const storageTotalBytes = selectedNode?.stats?.storageTotalBytes || 0;
  const storageUsedBytes = selectedNode?.stats?.storageUsedBytes || 0;
  const storagePhysicalFree = Math.max(storageTotalBytes - storageUsedBytes, 0);
  const storageReservedFree = Math.max(storageTotalBytes - usage.storageBytes, 0);
  const storageAvailableBytes = Math.max(
    Math.min(storagePhysicalFree, storageReservedFree) - STORAGE_VOLUME_OVERHEAD,
    0,
  );
  const storageTotalUnits = Math.floor(storageTotalBytes / STORAGE_STEP_BYTES);
  const storageAvailableUnits = Math.floor(storageAvailableBytes / STORAGE_STEP_BYTES);
  const capacityKnown = Boolean(
    selectedNode?.resourceUsage &&
      selectedNode.stats?.cpuThreads &&
      memoryTotalUnits > 0 &&
      storageTotalUnits > 0,
  );
  const resourceCapacityMissing =
    !capacityKnown ||
    cpuAvailableUnits < 1 ||
    memoryAvailableUnits < 1 ||
    storageAvailableUnits < 1;
  const defs: Array<{
    name: string;
    type?: string;
    default?: string;
    required?: boolean;
    ui?: boolean;
    description?: string;
  }> = selected?.spec?.variables || [];
  const shownDefs = defs.filter((v) => v.required === true || v.ui === true);
  const portSpecs = selected?.spec?.ports || [];
  const reportedBindIPs = selectedNode?.stats?.bindIPs || [];
  const selectableBindIPs = reportedBindIPs.filter((ip) => !isLoopbackIP(ip));
  function allocationKey(ip: string, port: number) {
    return `${ip}|${port}`;
  }
  const occupiedPorts = new Set(
    servers
      .filter((server) => server.nodeId === nodeId && server.status !== "error")
      .flatMap((server) =>
        (server.allocations || []).map((allocation) =>
          allocationKey(allocation.ip, allocation.host),
        ),
      ),
  );
  const portChoices = (selectedNode?.portAllocations || []).filter(
    (allocation) =>
      !occupiedPorts.has(allocationKey(allocation.ip, allocation.port)) &&
      (!reportedBindIPs.length || selectableBindIPs.includes(allocation.ip)),
  );
  const usedDefaultChoices = new Set<string>();
  const selectedPortEntries = portSpecs.map((_, index) => {
    const selectedKey = portSelections[index];
    const explicit = selectedKey
      ? portChoices.find(
          (allocation) =>
            allocationKey(allocation.ip, allocation.port) === selectedKey &&
            !usedDefaultChoices.has(selectedKey),
        )
      : undefined;
    const allocation =
      explicit ||
      portChoices.find(
        (candidate) =>
          !usedDefaultChoices.has(
            allocationKey(candidate.ip, candidate.port),
          ),
      );
    if (allocation) {
      usedDefaultChoices.add(allocationKey(allocation.ip, allocation.port));
    }
    return allocation;
  });
  const hasPortSelections = selectedPortEntries.every(Boolean);
  const allocationWarning = !selectedNode
    ? ""
    : reportedBindIPs.length > 0 && selectableBindIPs.length === 0
      ? "The node has only reported loopback interfaces. Wait for its network details or check the node agent before creating a server."
      : portChoices.length < portSpecs.length
        ? reportedBindIPs.length
          ? `This Config needs ${portSpecs.length} distinct ports, but only ${portChoices.length} are free on the node’s detected bind IPs (${selectableBindIPs.join(", ")}). Add available IP and port allocations in node settings.`
          : `The node has not reported its local interfaces yet, and only ${portChoices.length} free ports are available. Verify the selected IP belongs to the node; a public NAT address may not be bindable.`
        : reportedBindIPs.length === 0
          ? "The node has not reported its local interfaces yet. Verify the selected IP belongs to the node; a public NAT address may not be bindable."
          : "";
  const selectedAllocations = selectedPortEntries.flatMap((allocation, index) =>
    allocation
      ? [
          {
            name: portSpecs[index].name,
            ip: allocation.ip,
            host: allocation.port,
            container: portSpecs[index].container,
            protocol: portSpecs[index].protocol,
          },
        ]
      : [],
  );
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
    const defaultCpu = Math.max(
      1,
      Math.round((resources.cpuLimit ?? 2) * CPU_UNITS_PER_CORE),
    );
    const defaultMemory = Math.max(
      1,
      Math.round((resources.memoryMB ?? 4096) / MEMORY_STEP_MB),
    );
    setCpuUnits(
      cpuAvailableUnits > 0 ? Math.min(defaultCpu, cpuAvailableUnits) : defaultCpu,
    );
    setMemoryUnits(
      memoryAvailableUnits > 0
        ? Math.min(defaultMemory, memoryAvailableUnits)
        : defaultMemory,
    );
  }, [configId]);
  useEffect(() => {
    if (cpuAvailableUnits > 0 && cpuUnits > cpuAvailableUnits) setCpuUnits(cpuAvailableUnits);
    if (memoryAvailableUnits > 0 && memoryUnits > memoryAvailableUnits) setMemoryUnits(memoryAvailableUnits);
    if (storageAvailableUnits > 0 && storageUnits > storageAvailableUnits) setStorageUnits(storageAvailableUnits);
  }, [
    nodeId,
    cpuAvailableUnits,
    memoryAvailableUnits,
    storageAvailableUnits,
    cpuUnits,
    memoryUnits,
    storageUnits,
  ]);
  useEffect(() => setPortSelections([]), [nodeId, configId]);
  const online = nodes.filter((n) => n.status === "online");
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (duplicateName) {
      setError("A server with this name already exists.");
      return;
    }
    if (
      !capacityKnown ||
      cpuUnits > cpuAvailableUnits ||
      memoryUnits > memoryAvailableUnits ||
      storageUnits > storageAvailableUnits
    ) {
      setError("The selected resource limits exceed the node's unallocated capacity.");
      return;
    }
    if (!hasPortSelections) {
      setError("Choose a node bind IP and host port for every Config port.");
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
          cpuLimit: cpuUnits * CPU_STEP,
          memoryMB: memoryUnits * MEMORY_STEP_MB,
          diskSizeBytes: storageUnits * STORAGE_STEP_BYTES,
          variables,
          allocations: selectedAllocations,
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
      {portSpecs.length > 0 && (
        <div className="variable-list">
          <div className="subsection-label">PORT ALLOCATIONS</div>
          <small className="field-hint">
            Players connect to {selectedNode?.address || "the node address"};
            Docker binds each port to the selected local IP. For a node behind
            NAT, choose its private interface IP here.
          </small>
          {portSpecs.map((port, index) => {
            const current = selectedPortEntries[index];
            const otherSelections = new Set(
              selectedPortEntries.flatMap((entry, otherIndex) =>
                entry && otherIndex !== index
                  ? [allocationKey(entry.ip, entry.port)]
                  : [],
              ),
            );
            return (
              <Field
                key={`${port.name}-${port.container}-${index}`}
                label={`${port.name || "Game"} · ${port.container}/${port.protocol}`}
              >
                <select
                  required
                  value={
                    current
                      ? allocationKey(current.ip, current.port)
                      : ""
                  }
                  disabled={!portChoices.length}
                  onChange={(event) =>
                    setPortSelections((previous) => {
                      const next = [...previous];
                      next[index] = event.target.value;
                      return next;
                    })
                  }
                >
                  {!current && (
                    <option value="">
                      {portChoices.length
                        ? "Choose an IP and port"
                        : "No eligible node ports"}
                    </option>
                  )}
                  {portChoices
                    .filter(
                      (allocation) =>
                        !otherSelections.has(
                          allocationKey(allocation.ip, allocation.port),
                        ),
                    )
                    .map((allocation) => (
                      <option
                        key={allocationKey(allocation.ip, allocation.port)}
                        value={allocationKey(allocation.ip, allocation.port)}
                      >
                        {allocation.ip}:{allocation.port}
                      </option>
                    ))}
                </select>
              </Field>
            );
          })}
          {allocationWarning && (
            <div className="form-hint warning">{allocationWarning}</div>
          )}
        </div>
      )}
      <div className="resource-slider-grid">
        <ResourceSlider
          label="CPU limit"
          value={cpuUnits}
          minimum={1}
          total={cpuTotalUnits}
          available={cpuAvailableUnits}
          capacityKnown={capacityKnown}
          format={formatCores}
          onChange={setCpuUnits}
        />
        <ResourceSlider
          label="Memory limit"
          value={memoryUnits}
          minimum={1}
          total={memoryTotalUnits}
          available={memoryAvailableUnits}
          capacityKnown={capacityKnown}
          format={formatMemory}
          onChange={setMemoryUnits}
        />
        <ResourceSlider
          label="Server data disk"
          value={storageUnits}
          minimum={1}
          total={storageTotalUnits}
          available={storageAvailableUnits}
          capacityKnown={capacityKnown}
          format={formatStorage}
          onChange={setStorageUnits}
        />
      </div>
      <small className="resource-slider-note">
        Values snap to 0.25 CPU cores, 256 MB memory, and 128 MB storage. The muted end of each range is unavailable due to existing reservations or current use. {selectedNode?.stats?.storageMode === "docker-volume"
          ? "Docker Desktop uses dynamic volumes, so its storage limit is a reservation rather than an enforced disk quota."
          : "Linux nodes enforce each server disk limit."}
      </small>
      {!capacityKnown && selectedNode && (
        <div className="form-hint warning">
          Waiting for complete CPU, memory, and storage capacity from the node.
        </div>
      )}
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
        disabled={
          !online.length ||
          !configs.length ||
          !selectedNode ||
          resourceCapacityMissing ||
          duplicateName ||
          !hasPortSelections
        }
      />
    </form>
  );
}
