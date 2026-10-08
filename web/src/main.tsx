import { Button } from "./components/Button";
import React, { useCallback, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  Box,
  Check,
  ChevronRight,
  Globe,
  LayoutDashboard,
  LogOut,
  RefreshCw,
  Server,
  Settings,
  Users,
  X,
  HardDrive,
} from "lucide-react";
import {
  errorMessage,
  api,
  type AuditItem,
  type GameServer,
  type ModalKind,
  type NodeEnrollment,
  type NodeItem,
  type Config,
  type PermissionDefinition,
  type Role,
  type User,
} from "./shared/domain";
import { ConfirmationProvider, useConfirmation } from "./shared/dialogs";
import { subscribeToToasts } from "./shared/toast";
import {
  AdminPage,
  Brand,
  Dashboard,
  Login,
  Modal,
  NodeDetail,
  NodeResourcesPage,
  NodeForm,
  NodesPage,
  NodeTokenCard,
  ServerDetail,
  ServerForm,
  ServersPage,
  SettingsPage,
  Setup,
  ConfigForm,
  ConfigsPage,
  RoleForm,
  UserForm,
  VolumesPage,
} from "./app/views";
import "./styles.css";

const navItems = [
  { name: "Dashboard", icon: LayoutDashboard, permission: "dashboard.view" },
  { name: "Servers", icon: Server, permission: "servers.view" },
  { name: "Volumes", icon: HardDrive, permission: "servers.view" },
  { name: "Nodes", icon: Globe, permission: "nodes.view" },
  { name: "Configs", icon: Box, permission: "configs.view" },
  { name: "Administration", icon: Users, permission: "users.view" },
  { name: "Settings", icon: Settings },
];

const administrationPermissions = [
  "users.view",
  "users.manage",
  "users.delete",
  "roles.view",
  "roles.create",
  "roles.update",
  "roles.delete",
  "audit.view",
];

function ToastRegion({
  notice,
  error,
  noticeFading,
  errorFading,
  setNotice,
  setError,
}: {
  notice: string;
  error: string;
  noticeFading: boolean;
  errorFading: boolean;
  setNotice: React.Dispatch<React.SetStateAction<string>>;
  setError: React.Dispatch<React.SetStateAction<string>>;
}) {
  if (!notice && (!error || error.startsWith("Panel unavailable:")))
    return null;
  return (
    <div className="toast-region" aria-live="polite" aria-relevant="additions">
      {notice && (
        <div
          className={`toast toast-success${noticeFading ? " toast-fading" : ""}`}
          role="status"
        >
          <Check size={17} />
          <span>{notice}</span>
          <button
            className="icon-button"
            aria-label="Dismiss notification"
            onClick={() => setNotice("")}
          >
            <X size={15} />
          </button>
        </div>
      )}
      {error && !error.startsWith("Panel unavailable:") && (
        <div
          className={`toast toast-error${errorFading ? " toast-fading" : ""}`}
          role="alert"
        >
          <span>{error}</span>
          <button
            className="icon-button"
            aria-label="Dismiss error"
            onClick={() => setError("")}
          >
            <X size={15} />
          </button>
        </div>
      )}
    </div>
  );
}

function App() {
  const confirmAction = useConfirmation();
  const [user, setUser] = useState<User | null>(null);
  const [setup, setSetup] = useState<boolean | null>(null);
  const [pathname, setPathname] = useState(() => window.location.pathname);
  const [tab, setTab] = useState("Dashboard");
  const [modal, setModal] = useState<ModalKind>(null);
  const [editing, setEditing] = useState<Config | null>(null);
  const [editingRole, setEditingRole] = useState<Role | null>(null);
  const [editingUser, setEditingUser] = useState<User | null>(null);
  const [createdNode, setCreatedNode] = useState<NodeEnrollment | null>(null);
  const [selectedServer, setSelectedServer] = useState<GameServer | null>(null);
  const [editServerConfig, setEditServerConfig] = useState(false);
  const [selectedNode, setSelectedNode] = useState<NodeItem | null>(null);
  const [theme, setTheme] = useState(
    localStorage.getItem("control-plane-theme") === "nord" ? "nord" : "dark",
  );
  const [servers, setServers] = useState<GameServer[]>([]);
  const [nodes, setNodes] = useState<NodeItem[]>([]);
  const [configs, setConfigs] = useState<Config[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [audit, setAudit] = useState<AuditItem[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [permissionCatalog, setPermissionCatalog] = useState<
    PermissionDefinition[]
  >([]);
  const [manualRefreshing, setManualRefreshing] = useState(false);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const [noticeFading, setNoticeFading] = useState(false);
  const [errorFading, setErrorFading] = useState(false);

  useEffect(() => {
    return subscribeToToasts(({ message, kind }) => {
      if (kind === "success") {
        setError("");
        setNotice(message);
      } else {
        setNotice("");
        setError(message);
      }
    });
  }, []);

  useEffect(() => {
    if (!notice) return;
    setNoticeFading(false);
    const fadeTimer = window.setTimeout(() => setNoticeFading(true), 4200);
    const removeTimer = window.setTimeout(() => setNotice(""), 4500);
    return () => {
      window.clearTimeout(fadeTimer);
      window.clearTimeout(removeTimer);
    };
  }, [notice]);

  useEffect(() => {
    if (!error || error.startsWith("Panel unavailable:")) return;
    setErrorFading(false);
    const fadeTimer = window.setTimeout(() => setErrorFading(true), 6700);
    const removeTimer = window.setTimeout(() => setError(""), 7000);
    return () => {
      window.clearTimeout(fadeTimer);
      window.clearTimeout(removeTimer);
    };
  }, [error]);

  const navigate = useCallback((path: string, replace = false) => {
    if (window.location.pathname !== path) {
      if (replace) window.history.replaceState({}, "", path);
      else window.history.pushState({}, "", path);
    }
    setPathname(path);
  }, []);

  useEffect(() => {
    const onPopState = () => setPathname(window.location.pathname);
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);

  const pathParts = pathname.split("/").filter(Boolean);
  const routeSection = pathParts[0] || "dashboard";
  const routeEntityID = pathParts[1];
  const routeSubsection = pathParts[2] || "";
  const sectionNames: Record<string, string> = {
    dashboard: "Dashboard",
    servers: "Servers",
    volumes: "Volumes",
    nodes: "Nodes",
    configs: "Configs",
    administration: "Administration",
    settings: "Settings",
  };

  useEffect(() => {
    if (!user) return;
    if (pathname === "/" || routeSection === "login") {
      navigate("/dashboard", true);
      return;
    }
    if (
      routeSection === "administration" &&
      user.role !== "root" &&
      !user.permissions?.some((permission) =>
        administrationPermissions.includes(permission),
      )
    ) {
      navigate("/dashboard", true);
      return;
    }
    setTab(sectionNames[routeSection] || "Dashboard");
    if (routeSection === "servers" && routeEntityID) {
      setSelectedServer(
        servers.find((item) => item.id === routeEntityID) || null,
      );
      setSelectedNode(null);
    } else if (routeSection === "nodes" && routeEntityID) {
      setSelectedNode(nodes.find((item) => item.id === routeEntityID) || null);
      setSelectedServer(null);
    } else {
      setSelectedServer(null);
      setSelectedNode(null);
      setEditServerConfig(false);
    }
  }, [pathname, routeSection, routeEntityID, user, servers, nodes, navigate]);

  const goToSection = (name: string) => {
    setError("");
    navigate(`/${name.toLowerCase()}`);
  };

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    localStorage.setItem("control-plane-theme", theme);
  }, [theme]);

  useEffect(() => {
    let live = true;
    api<{ setupRequired: boolean }>("/api/setup/status")
      .then(async (result) => {
        if (!live) return;
        setSetup(result.setupRequired);
        if (!result.setupRequired) {
          try {
            setUser(await api<User>("/api/auth/me"));
          } catch {
            /* sign in */
          }
        }
      })
      .catch((e) => live && setError("Panel unavailable: " + errorMessage(e)));
    return () => {
      live = false;
    };
  }, []);

  const refresh = useCallback(async () => {
    if (!user) return;
    const can = (permission: string) =>
      user.role === "root" || user.permissions?.includes(permission);
    const jobs: Promise<unknown>[] = [];
    const keys: string[] = [];
    const add = (key: string, promise: Promise<unknown>) => {
      keys.push(key);
      jobs.push(promise);
    };
    if (can("servers.view")) add("servers", api<GameServer[]>("/api/servers"));
    if (can("nodes.view")) add("nodes", api<NodeItem[]>("/api/nodes"));
    if (can("configs.view")) add("configs", api<Config[]>("/api/configs"));
    if (can("users.view")) add("users", api<User[]>("/api/admin/users"));
    if (can("audit.view")) add("audit", api<AuditItem[]>("/api/admin/audit"));
    if (
      [
        "roles.view",
        "roles.create",
        "roles.update",
        "roles.delete",
        "users.manage",
      ].some(can)
    ) {
      add(
        "roles",
        api<{ roles: Role[]; permissions: PermissionDefinition[] }>(
          "/api/roles",
        ),
      );
    }
    const results = await Promise.allSettled(jobs);
    results.forEach((result, index) => {
      if (result.status !== "fulfilled") {
        if (keys[index] === "servers")
          setError(
            (result.reason as Error).message || "Could not load servers",
          );
        return;
      }
      switch (keys[index]) {
        case "servers":
          setServers((result.value as GameServer[]) || []);
          break;
        case "nodes":
          setNodes((result.value as NodeItem[]) || []);
          break;
        case "configs":
          setConfigs((result.value as Config[]) || []);
          break;
        case "users":
          setUsers((result.value as User[]) || []);
          break;
        case "audit":
          setAudit((result.value as AuditItem[]) || []);
          break;
        case "roles": {
          const payload = result.value as {
            roles: Role[];
            permissions: PermissionDefinition[];
          };
          setRoles(payload.roles || []);
          setPermissionCatalog(payload.permissions || []);
          break;
        }
      }
    });
  }, [user]);
  useEffect(() => {
    void refresh();
  }, [refresh]);
  useEffect(() => {
    if (!user) return;
    const timer = window.setInterval(() => {
      void refresh();
    }, 10000);
    return () => window.clearInterval(timer);
  }, [user, refresh]);
  useEffect(() => {
    if (selectedServer?.status !== "installing") return;
    const timer = window.setInterval(() => void refresh(), 1500);
    return () => window.clearInterval(timer);
  }, [selectedServer?.id, selectedServer?.status, refresh]);

  const closeModal = () => {
    setModal(null);
    setEditing(null);
    setEditingRole(null);
    setEditingUser(null);
  };
  const login = async (username: string, password: string) => {
    const result = await api<{
      user?: User;
      requiresOTP: boolean;
      challengeId?: string;
    }>("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
    if (result.user) {
      setUser(result.user);
      navigate("/dashboard", true);
    }
    return { requiresOTP: result.requiresOTP, challengeId: result.challengeId };
  };
  const loginOTP = async (challengeId: string, code: string) => {
    const result = await api<{ user: User }>("/api/auth/login/otp", {
      method: "POST",
      body: JSON.stringify({ challengeId, code }),
    });
    setUser(result.user);
    navigate("/dashboard", true);
  };
  const logout = async () => {
    try {
      await api("/api/auth/logout", { method: "POST" });
    } finally {
      setUser(null);
      setSelectedServer(null);
      setSelectedNode(null);
      setTab("Dashboard");
      navigate("/login", true);
    }
  };
  const createNode = async (result: NodeEnrollment) => {
    closeModal();
    setCreatedNode(result);
    setTab("Nodes");
    navigate("/nodes");
    await refresh();
  };
  const done = async (message: string) => {
    closeModal();
    setNotice(message);
    await refresh();
  };
  const onServerCreated = (server: GameServer) => {
    closeModal();
    setError("");
    setNotice("Server created. Installation is starting.");
    setServers((current) => [
      server,
      ...current.filter((item) => item.id !== server.id),
    ]);
    setSelectedServer(server);
    setSelectedNode(null);
    setEditServerConfig(false);
    setTab("Servers");
    navigate(`/servers/${server.id}`);
  };

  if (setup === null) {
    return (
      <div className="center">
        <span className="spinner" />
        Connecting to panel…
      </div>
    );
  }
  if (error.startsWith("Panel unavailable:")) {
    return (
      <div className="auth-page">
        <div className="auth-card">
          <Brand />
          <h1>Panel unavailable</h1>
          <p>{error}</p>
          <Button variant="primary" onClick={() => location.reload()}>
            <RefreshCw size={16} />
            Retry
          </Button>
        </div>
      </div>
    );
  }
  if (setup) {
    return (
      <Setup
        onDone={() => {
          setSetup(false);
          setNotice("Root account created. Sign in to continue.");
        }}
      />
    );
  }
  if (!user) {
    return (
      <>
        <Login onLogin={login} onLoginOTP={loginOTP} />
        <ToastRegion
          notice={notice}
          error={error}
          noticeFading={noticeFading}
          errorFading={errorFading}
          setNotice={setNotice}
          setError={setError}
        />
      </>
    );
  }

  const hasPermission = (permission: string) =>
    user.role === "root" || !!user.permissions?.includes(permission);
  const canSeeAdmin =
    user.role === "root" || administrationPermissions.some(hasPermission);
  const visibleNav = navItems.filter((item) => {
    if (item.name === "Administration") return canSeeAdmin;
    return !item.permission || hasPermission(item.permission);
  });
  const nodeByID = new Map(nodes.map((n) => [n.id, n]));
  const configByID = new Map(configs.map((t) => [t.id, t]));
  const title = selectedServer ? selectedServer.name : tab;
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <Brand full />
        <div className="nav-label">WORKSPACE</div>
        <nav>
          {visibleNav.map(({ name, icon: Icon }) => (
            <button
              key={name}
              className={
                "nav-item " +
                (tab === name && !selectedServer && !selectedNode
                  ? "active"
                  : "")
              }
              onClick={() => {
                setTab(name);
                setSelectedServer(null);
                setSelectedNode(null);
                setEditServerConfig(false);
                setError("");
                navigate(`/${name.toLowerCase()}`);
              }}
            >
              <Icon size={18} />
              <span>{name}</span>
              {tab === name && !selectedServer && !selectedNode && <i />}
            </button>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="connection">
            <span className="status-dot online" />
            Panel connected
          </div>
          <div className="account-mini">
            <div className="avatar">
              {user.username.slice(0, 1).toUpperCase()}
            </div>
            <div className="account-copy">
              <strong>{user.username}</strong>
              <small>{user.role}</small>
            </div>
            <button
              className="icon-button"
              title="Sign out"
              onClick={() => void logout()}
            >
              <LogOut size={16} />
            </button>
          </div>
        </div>
      </aside>
      <main className="main-area">
        <header className="topbar">
          <div className="breadcrumbs">
            <span>Control Plane</span>
            <ChevronRight size={14} />
            <strong>{title}</strong>
          </div>
          <div className="topbar-actions">
            <button
              className="icon-button"
              title={manualRefreshing ? "Refreshing…" : "Refresh"}
              aria-label={manualRefreshing ? "Refreshing" : "Refresh"}
              aria-busy={manualRefreshing}
              disabled={manualRefreshing}
              onClick={() => {
                setManualRefreshing(true);
                void refresh().finally(() => setManualRefreshing(false));
              }}
            >
              <RefreshCw
                className={manualRefreshing ? "topbar-refreshing" : ""}
                size={16}
              />
            </button>
            <button
              className="icon-button"
              title="Settings"
              onClick={() => {
                setTab("Settings");
                setSelectedServer(null);
                setSelectedNode(null);
                navigate("/settings");
              }}
            >
              <Settings size={17} />
            </button>
          </div>
        </header>
        <div className="content-wrap">
          <div key={pathname} className="page-transition">
            {selectedServer ? (
              <ServerDetail
                server={selectedServer}
                node={nodeByID.get(selectedServer.nodeId)}
                config={configByID.get(selectedServer.configId)}
                permissions={user.permissions || []}
                editConfig={editServerConfig}
                onEditConfig={setEditServerConfig}
                onBack={() => {
                  setSelectedServer(null);
                  setEditServerConfig(false);
                  void refresh();
                  navigate("/servers");
                }}
                onDeleted={() => {
                  setSelectedServer(null);
                  setEditServerConfig(false);
                  void refresh();
                  navigate("/servers");
                }}
                onChanged={() => void refresh()}
              />
            ) : selectedNode ? (
              routeSubsection === "resources" ? (
                <NodeResourcesPage
                  node={nodeByID.get(selectedNode.id) || selectedNode}
                  servers={servers.filter(
                    (server) => server.nodeId === selectedNode.id,
                  )}
                  canManage={hasPermission("nodes.manage")}
                  onChanged={() => void refresh()}
                  onBack={() => navigate(`/nodes/${selectedNode.id}`)}
                />
              ) : (
                <NodeDetail
                  node={nodeByID.get(selectedNode.id) || selectedNode}
                  canManage={hasPermission("nodes.manage")}
                  servers={servers}
                  configByID={configByID}
                  onOpenServer={(server) => {
                    setSelectedServer(server);
                    setEditServerConfig(false);
                    navigate(`/servers/${server.id}`);
                  }}
                  onOpenResources={() =>
                    navigate(`/nodes/${selectedNode.id}/resources`)
                  }
                  onBack={() => {
                    setSelectedNode(null);
                    void refresh();
                    navigate("/nodes");
                  }}
                  onDeleted={() => {
                    setSelectedNode(null);
                    void refresh();
                    navigate("/nodes");
                  }}
                  onChanged={() => void refresh()}
                />
              )
            ) : (
              <>
                {tab === "Dashboard" && (
                  <Dashboard
                    servers={servers}
                    nodes={nodes}
                    configs={configs}
                    nodeByID={nodeByID}
                    configByID={configByID}
                    onCreate={(kind: ModalKind) => setModal(kind)}
                    onOpen={(server: GameServer) => {
                      setSelectedServer(server);
                      navigate(`/servers/${server.id}`);
                    }}
                    onNavigate={goToSection}
                    canCreateServer={hasPermission("servers.create")}
                    canManageNodes={hasPermission("nodes.manage")}
                    canManageConfigs={hasPermission("configs.manage")}
                    canViewNodes={hasPermission("nodes.view")}
                    canViewServers={hasPermission("servers.view")}
                    onNode={(node: NodeItem) => {
                      setSelectedNode(node);
                      navigate(`/nodes/${node.id}`);
                    }}
                  />
                )}
                {tab === "Servers" && (
                  <ServersPage
                    servers={servers}
                    nodes={nodes}
                    configs={configs}
                    nodeByID={nodeByID}
                    configByID={configByID}
                    onCreate={() => setModal("server")}
                    canCreate={hasPermission("servers.create")}
                    canUpdate={hasPermission("servers.update")}
                    canConsole={hasPermission("servers.console")}
                    onOpen={(s: GameServer) => {
                      setSelectedServer(s);
                      setEditServerConfig(false);
                      navigate(`/servers/${s.id}`);
                    }}
                    onEdit={(s: GameServer) => {
                      setSelectedServer(s);
                      setEditServerConfig(true);
                      navigate(`/servers/${s.id}`);
                    }}
                  />
                )}
                {tab === "Volumes" && (
                  <VolumesPage
                    servers={servers}
                    nodes={nodes}
                    configs={configs}
                    canManage={hasPermission("servers.update")}
                    canExtend={hasPermission("volumes.extend")}
                    canBrowseFiles={hasPermission("volumes.files.view")}
                    canManageFiles={hasPermission("volumes.files.manage")}
                    isRoot={user.role === "root"}
                    userId={user.id}
                    onRefresh={refresh}
                    onOpenServer={(server) => {
                      setSelectedServer(server);
                      setEditServerConfig(false);
                      navigate(`/servers/${server.id}`);
                    }}
                  />
                )}
                {tab === "Nodes" && (
                  <NodesPage
                    nodes={nodes}
                    servers={servers}
                    canManage={hasPermission("nodes.manage")}
                    onCreate={() => setModal("node")}
                    onOpen={(node: NodeItem) => {
                      setSelectedNode(node);
                      navigate(`/nodes/${node.id}`);
                    }}
                    onHelp={(node) =>
                      setCreatedNode({ node, token: "", panelURL: "" })
                    }
                    onOpenServer={(server) => {
                      setSelectedServer(server);
                      setEditServerConfig(false);
                      navigate(`/servers/${server.id}`);
                    }}
                  />
                )}
                {tab === "Configs" && (
                  <ConfigsPage
                    configs={configs}
                    canManage={hasPermission("configs.manage")}
                    onCreate={() => {
                      setEditing(null);
                      setModal("config");
                    }}
                    onEdit={(config) => {
                      setEditing(config);
                      setModal("config");
                    }}
                  />
                )}
                {tab === "Administration" && (
                  <AdminPage
                    users={users}
                    audit={audit}
                    roles={roles}
                    permissions={user.permissions || []}
                    permissionCatalog={permissionCatalog}
                    user={user}
                    nodes={nodes}
                    configs={configs}
                    onCreate={() => {
                      setEditingUser(null);
                      setModal("user");
                    }}
                    onCreateRole={() => {
                      setEditingRole(null);
                      setModal("role");
                    }}
                    onEditRole={(role) => {
                      setEditingRole(role);
                      setModal("role");
                    }}
                    onDeleteRole={async (role) => {
                      if (
                        !(await confirmAction({
                          title: "Delete role",
                          message: `Delete the “${role.name}” role? Users assigned to it may need a different role.`,
                          confirmLabel: "Delete role",
                          destructive: true,
                        }))
                      )
                        return;
                      try {
                        await api(`/api/roles/${encodeURIComponent(role.id)}`, {
                          method: "DELETE",
                        });
                        setNotice("Role deleted.");
                        await refresh();
                      } catch (err: unknown) {
                        setError(errorMessage(err));
                      }
                    }}
                    onEditUser={(item) => {
                      setEditingUser(item);
                      setModal("user");
                    }}
                    onDeleteUser={async (item) => {
                      const serverCount = item.managedServers?.length || 0;
                      if (
                        !(await confirmAction({
                          title: `Delete user ${item.username}`,
                          message: `This permanently deletes the user and ${serverCount} owned server${serverCount === 1 ? "" : "s"}, including server files, volumes, backups, and backup images. Nodes hosting those servers must be online.`,
                          confirmLabel: "Delete user and data",
                          destructive: true,
                        }))
                      )
                        return false;
                      try {
                        await api(
                          `/api/admin/users/${encodeURIComponent(item.id)}`,
                          { method: "DELETE" },
                        );
                        setNotice("User and owned server data deleted.");
                        await refresh();
                        return true;
                      } catch (err: unknown) {
                        setError(errorMessage(err));
                        return false;
                      }
                    }}
                    onOpenServer={(serverID) => {
                      if (!hasPermission("servers.view")) {
                        setError("You do not have permission to view servers.");
                        return;
                      }
                      const server = servers.find(
                        (item) => item.id === serverID,
                      );
                      if (!server) return;
                      setSelectedServer(server);
                      setSelectedNode(null);
                      navigate(`/servers/${server.id}`);
                    }}
                  />
                )}
                {tab === "Settings" && (
                  <SettingsPage
                    user={user}
                    theme={theme}
                    onTheme={setTheme}
                    onLogout={() => void logout()}
                    onProfileUpdated={setUser}
                  />
                )}
              </>
            )}
          </div>
        </div>
      </main>
      <ToastRegion
        notice={notice}
        error={error}
        noticeFading={noticeFading}
        errorFading={errorFading}
        setNotice={setNotice}
        setError={setError}
      />
      {modal && (
        <Modal
          wide={modal === "server"}
          title={
            modal === "node"
              ? "Register node"
              : modal === "config"
                ? editing
                  ? "Edit config"
                  : "Create config"
                : modal === "role"
                  ? editingRole
                    ? "Edit role"
                    : "Create role"
                  : modal === "server"
                    ? "Create game server"
                    : modal === "user"
                      ? editingUser
                        ? "Edit user"
                        : "Create user"
                      : "Create user"
          }
          onClose={closeModal}
        >
          {modal === "node" && (
            <NodeForm onCreated={createNode} onCancel={closeModal} />
          )}
          {modal === "config" && (
            <ConfigForm
              config={editing}
              availableConfigs={configs}
              onSaved={() =>
                done(editing ? "Config updated." : "Config created.")
              }
              onCancel={closeModal}
            />
          )}
          {modal === "server" && (
            <ServerForm
              nodes={nodes}
              configs={configs}
              servers={servers}
              onCreated={onServerCreated}
              onCancel={closeModal}
            />
          )}
          {modal === "user" && (
            <UserForm
              roles={roles.filter(
                (role) => role.id !== "root" || user.role === "root",
              )}
              user={editingUser}
              currentUser={user}
              onSaved={async () => {
                closeModal();
                setNotice(editingUser ? "User updated." : "User created.");
                if (editingUser?.id === user.id) {
                  try {
                    setUser(await api<User>("/api/auth/me"));
                  } catch {
                    /* session remains active */
                  }
                }
                await refresh();
              }}
              onCancel={closeModal}
            />
          )}
          {modal === "role" && (
            <RoleForm
              role={editingRole}
              permissions={permissionCatalog.filter(
                (permission) =>
                  hasPermission(permission.key) ||
                  !!editingRole?.permissions.includes(permission.key),
              )}
              onSaved={async () => {
                closeModal();
                setNotice(editingRole ? "Role updated." : "Role created.");
                try {
                  const current = await api<User>("/api/auth/me");
                  setUser(current);
                } catch {
                  /* current session remains active */
                }
                await refresh();
              }}
              onCancel={closeModal}
            />
          )}
        </Modal>
      )}
      {createdNode && (
        <Modal
          title={createdNode.token ? "Save this node token" : "Node setup help"}
          onClose={() => setCreatedNode(null)}
        >
          <NodeTokenCard
            result={createdNode}
            onClose={() => setCreatedNode(null)}
          />
        </Modal>
      )}
    </div>
  );
}

createRoot(document.getElementById("root")!).render(
  <ConfirmationProvider>
    <App />
  </ConfirmationProvider>,
);
