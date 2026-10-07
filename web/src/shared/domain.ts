export type User = {
  id: string;
  username: string;
  email: string;
  role: string;
  permissions?: string[];
  disabled?: boolean;
  managedServers?: Pick<
    GameServer,
    "id" | "name" | "status" | "nodeId" | "configId"
  >[];
  totpEnabled?: boolean;
};
export type Role = {
  id: string;
  name: string;
  description: string;
  permissions: string[];
  createdAt?: string;
  updatedAt?: string;
};
export type PermissionDefinition = {
  key: string;
  label: string;
  group: string;
};
export type NodeItem = {
  id: string;
  name: string;
  region: string;
  address: string;
  ips?: string[];
  portAllocations?: { ip: string; port: number }[];
  status: string;
  lastSeen?: string;
  stats?: {
    cpuThreads: number;
    architecture: string;
    kernel: string;
    cpuPercent: number;
    memoryUsedBytes: number;
    memoryTotalBytes: number;
    storageUsedBytes: number;
    storageTotalBytes: number;
    storageMode?: "ext4" | "docker-volume";
    updatedAt: string;
  };
};
export type NodeContainer = {
  id: string;
  names: string[];
  image: string;
  imageId?: string;
  serverId?: string;
  serverName?: string;
  serverStatus?: string;
  ownerName?: string;
  lastOnlineAt?: string;
  state: string;
  status: string;
  created: number;
};
export type NodeImage = {
  id: string;
  tags?: string[];
  sizeBytes: number;
  created: number;
};
export type NodeResources = {
  containers: NodeContainer[];
  images: NodeImage[];
};
export type ConfigVariable = {
  name: string;
  type?: "text" | "integer" | "secret";
  default?: string;
  required?: boolean;
  ui?: boolean;
  secret?: boolean;
  description?: string;
};
export type ConfigPort = {
  name: string;
  container: number;
  protocol: "tcp" | "udp" | "sctp";
};
export type ConfigSpec = {
  version: number;
  dockerImages: string[];
  startup?: string;
  stop?: string;
  user?: string;
  workingDirectory?: string;
  install?: { image: string; entrypoint?: string; script: string };
  environment?: Record<string, string>;
  variables?: ConfigVariable[];
  ports?: ConfigPort[];
  dataDirectory?: string;
  resources?: { cpuLimit?: number; memoryMB?: number };
  supportedArchitectures?: string[];
};
export type Config = {
  id: string;
  name: string;
  slug: string;
  description: string;
  spec: ConfigSpec;
};
export type GameServer = {
  id: string;
  name: string;
  status: string;
  error?: string;
  cpuLimit: number;
  memoryMB: number;
  diskSizeBytes?: number;
  volumes?: ServerVolume[];
  nodeId: string;
  managerId?: string;
  configId: string;
  containerId?: string;
  address?: string;
  allocations?: {
    name: string;
    ip: string;
    host: number;
    container: number;
    protocol: string;
  }[];
  variables?: Record<string, string>;
  launchCommand?: string;
  createdAt: string;
};
export type ServerLaunchInfo = {
  entrypoint?: string[];
  command?: string[];
  configCommand?: string;
  gameCommand?: string;
  running?: boolean;
  cpuPercent?: number;
  memoryUsedBytes?: number;
  memoryLimitBytes?: number;
};
export type ServerVolume = {
  id: string;
  name: string;
  mountPath: string;
  sizeBytes: number;
  createdAt?: string;
};
export type BackupItem = {
  id: string;
  image: string;
  createdAt: string;
  sizeBytes: number;
};
export type UploadProgress = {
  loaded: number;
  total: number;
  remainingSeconds: number | null;
  phase: "uploading" | "saving";
};
export type FileEntry = {
  name: string;
  directory: boolean;
  size: number;
  modifiedAt: string;
};
export type AuditItem = {
  id: string;
  action: string;
  detail: string;
  createdAt: string;
};
export type ModalKind = "node" | "config" | "server" | "user" | "role" | null;
export type NodeEnrollment = {
  node: NodeItem;
  token: string;
  panelURL: string;
};

/** Send a JSON request and return its decoded response or a useful error. */
export async function api<T = unknown>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers || {}) },
  });
  if (!response.ok) {
    const body = await response.text();
    let message = body || response.statusText;
    try {
      message = JSON.parse(body).error || message;
    } catch {
      /* plain response */
    }
    throw new Error(message);
  }
  return response.json() as Promise<T>;
}

/** Safely extract an actionable message from an unknown thrown value. */
export function errorMessage(error: unknown): string {
  return error instanceof Error
    ? error.message
    : "An unexpected error occurred.";
}
