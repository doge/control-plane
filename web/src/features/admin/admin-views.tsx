import { Button } from "../../components/Button";
import { FormEvent, useState } from "react";
import { Check, Clipboard, Plus, Settings, Trash } from "lucide-react";

import {
  errorMessage,
  api,
  type AuditItem,
  type NodeEnrollment,
  type NodeItem,
  type Config,
  type PermissionDefinition,
  type Role,
  type User,
} from "../../shared/domain";

import {
  Field,
  FormActions,
  Modal,
  PageHeading,
  PanelHeading,
  ServerRow,
  TableWrap,
} from "../../components";

/** Present user, role, and audit administration views with permission checks. */
export function AdminPage({
  users,
  audit,
  roles,
  permissions,
  permissionCatalog,
  user,
  nodes,
  configs,
  onCreate,
  onCreateRole,
  onEditRole,
  onDeleteRole,
  onEditUser,
  onDeleteUser,
  onOpenServer,
}: {
  users: User[];
  audit: AuditItem[];
  roles: Role[];
  permissions: string[];
  permissionCatalog: PermissionDefinition[];
  user: User;
  nodes: NodeItem[];
  configs: Config[];
  onCreate: () => void;
  onCreateRole: () => void;
  onEditRole: (role: Role) => void;
  onDeleteRole: (role: Role) => void;
  onEditUser: (user: User) => void;
  onDeleteUser: (user: User) => Promise<boolean>;
  onOpenServer: (serverID: string) => void;
}) {
  const [profileUser, setProfileUser] = useState<User | null>(null);
  const can = (permission: string) =>
    user.role === "root" || permissions.includes(permission);
  const canSeeRoles = [
    "roles.view",
    "roles.create",
    "roles.update",
    "roles.delete",
  ].some(can);
  return (
    <div className="page-stack">
      <PageHeading
        eyebrow="ACCESS CONTROL"
        title="Administration"
        description="Manage panel accounts, roles, and recorded activity."
        action={
          can("users.manage") ? (
            <Button variant="primary" onClick={onCreate}>
              <Plus size={16} />
              Add user
            </Button>
          ) : undefined
        }
      />
      {canSeeRoles && (
        <section className="panel-card table-card">
          <PanelHeading
            title="Roles and permissions"
            sub={`${roles.length} roles · permissions control panel access`}
            action={
              can("roles.create") ? (
                <Button variant="subtle" size="tiny" onClick={onCreateRole}>
                  <Plus size={14} />
                  Create role
                </Button>
              ) : undefined
            }
          />
          {roles.length ? (
            <TableWrap>
              <table>
                <thead>
                  <tr>
                    <th>ROLE</th>
                    <th>PERMISSIONS</th>
                    <th>ACTIONS</th>
                  </tr>
                </thead>
                <tbody>
                  {roles.map((role) => (
                    <tr key={role.id}>
                      <td>
                        <strong>{role.name}</strong>
                        {role.description && (
                          <small className="role-description">
                            {role.description}
                          </small>
                        )}
                      </td>
                      <td>
                        {role.id === "root" ? (
                          `All ${permissionCatalog.length} permissions`
                        ) : (
                          <div
                            className="permission-summary"
                            title={role.permissions
                              .map(
                                (key) =>
                                  permissionCatalog.find(
                                    (item) => item.key === key,
                                  )?.label || key,
                              )
                              .join(", ")}
                          >
                            {role.permissions.slice(0, 3).map((key) => (
                              <span className="permission-tag" key={key}>
                                {permissionCatalog.find(
                                  (item) => item.key === key,
                                )?.label || key}
                              </span>
                            ))}
                            {role.permissions.length > 3 && (
                              <span className="permission-tag permission-count">
                                +{role.permissions.length - 3}
                              </span>
                            )}
                          </div>
                        )}
                      </td>
                      <td>
                        {role.id === "root" ? (
                          <span className="role-pill">System · locked</span>
                        ) : (
                          <div className="inline-actions">
                            {can("roles.update") && (
                              <button
                                className="icon-button"
                                title="Edit role"
                                onClick={() => onEditRole(role)}
                              >
                                <Settings size={15} />
                              </button>
                            )}
                            {can("roles.delete") && (
                              <button
                                className="icon-button danger-icon"
                                title="Delete role"
                                onClick={() => onDeleteRole(role)}
                              >
                                <Trash size={15} />
                              </button>
                            )}
                          </div>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </TableWrap>
          ) : (
            <div className="inline-empty">No roles found.</div>
          )}
        </section>
      )}
      {can("users.view") && (
        <section className="panel-card table-card">
          <PanelHeading title="Users" sub={users.length + " accounts"} />
          {users.length ? (
            <TableWrap>
              <table>
                <thead>
                  <tr>
                    <th>USER</th>
                    <th>EMAIL</th>
                    <th>ROLE</th>
                  </tr>
                </thead>
                <tbody>
                  {users.map((u) => (
                    <tr
                      key={u.id}
                      className="user-profile-row"
                      tabIndex={0}
                      onClick={() => setProfileUser(u)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter" || event.key === " ") {
                          event.preventDefault();
                          setProfileUser(u);
                        }
                      }}
                    >
                      <td>
                        <strong>{u.username}</strong>
                      </td>
                      <td>{u.email}</td>
                      <td>
                        <span className="role-pill">
                          {roles.find((role) => role.id === u.role)?.name ||
                            u.role}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </TableWrap>
          ) : (
            <div className="inline-empty">No users found.</div>
          )}
        </section>
      )}
      {can("audit.view") && (
        <section className="panel-card table-card">
          <PanelHeading title="Audit activity" sub="Recent recorded changes" />
          {audit.length ? (
            <TableWrap>
              <table>
                <thead>
                  <tr>
                    <th>ACTION</th>
                    <th>DETAIL</th>
                    <th>WHEN</th>
                  </tr>
                </thead>
                <tbody>
                  {audit.map((a) => (
                    <tr key={a.id}>
                      <td>
                        <span className="role-pill">{a.action}</span>
                      </td>
                      <td>{a.detail}</td>
                      <td>{new Date(a.createdAt).toLocaleString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </TableWrap>
          ) : (
            <div className="inline-empty">
              No audit events have been recorded.
            </div>
          )}
        </section>
      )}
      {profileUser && (
        <Modal title="User profile" onClose={() => setProfileUser(null)}>
          <div className="modal-body user-profile">
            <div className="user-profile-facts">
              <div>
                <small>USERNAME</small>
                <strong>{profileUser.username}</strong>
              </div>
              <div>
                <small>EMAIL</small>
                <strong>{profileUser.email}</strong>
              </div>
              <div>
                <small>ROLE</small>
                <strong>
                  {roles.find((role) => role.id === profileUser.role)?.name ||
                    profileUser.role}
                </strong>
              </div>
            </div>
            {profileUser.disabled && (
              <div className="inline-empty">
                This account is disabled while server cleanup is pending. Retry
                deletion after the affected nodes are available.
              </div>
            )}
            <section className="user-profile-servers">
              <PanelHeading
                title="Managed servers"
                sub={`${profileUser.managedServers?.length || 0} server${profileUser.managedServers?.length === 1 ? "" : "s"} owned by this user`}
              />
              {profileUser.managedServers?.length ? (
                profileUser.managedServers.map((server) => (
                  <ServerRow
                    key={server.id}
                    server={server}
                    node={nodes.find((item) => item.id === server.nodeId)}
                    config={configs.find((item) => item.id === server.configId)}
                    onClick={() => onOpenServer(server.id)}
                  />
                ))
              ) : (
                <div className="inline-empty">
                  This user does not own any servers.
                </div>
              )}
            </section>
            <div className="form-actions">
              {can("users.manage") && profileUser.role !== "root" && (
                <Button
                  variant="subtle"
                  onClick={() => {
                    onEditUser(profileUser);
                    setProfileUser(null);
                  }}
                >
                  <Settings size={14} />
                  Edit account
                </Button>
              )}
              {can("users.delete") &&
                profileUser.role !== "root" &&
                profileUser.id !== user.id && (
                  <Button
                    variant="danger"
                    onClick={() =>
                      void (async () => {
                        if (await onDeleteUser(profileUser))
                          setProfileUser(null);
                      })()
                    }
                  >
                    <Trash size={14} />
                    Delete user
                  </Button>
                )}
              <Button variant="primary" onClick={() => setProfileUser(null)}>
                Done
              </Button>
            </div>
          </div>
        </Modal>
      )}
    </div>
  );
}

/** Create or edit a node and its connection and allocation settings. */
export function NodeForm({
  onCreated,
  onCancel,
}: {
  onCreated: (r: NodeEnrollment) => void;
  onCancel: () => void;
}) {
  const [name, setName] = useState("");
  const [region, setRegion] = useState("");
  const [address, setAddress] = useState("");
  const [ports, setPorts] = useState(["25565"]);
  const [panelURL, setPanelURL] = useState(location.origin);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const result = await api<{ node: NodeItem; token: string }>(
        "/api/nodes",
        {
          method: "POST",
          body: JSON.stringify({
            name,
            region,
            address,
            ports: ports.map(Number),
          }),
        },
      );
      onCreated({ ...result, panelURL: panelURL.trim().replace(/\/$/, "") });
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };
  const addPort = () =>
    setPorts((old) => [
      ...old,
      String(Math.min(65535, Math.max(25564, ...old.map(Number)) + 1)),
    ]);
  return (
    <form className="form-stack modal-body" onSubmit={submit}>
      <p className="modal-intro">
        Register a Docker host and list the exact ports available for servers on
        each resolved IP.
      </p>
      <div className="form-two">
        <Field label="Node name">
          <input
            autoFocus
            required
            placeholder="e.g. calgary-01"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </Field>
        <Field label="Region">
          <input
            placeholder="e.g. Calgary, AB"
            value={region}
            onChange={(e) => setRegion(e.target.value)}
          />
        </Field>
      </div>
      <Field label="Node hostname or IP">
        <input
          required
          placeholder="games.example.net"
          value={address}
          onChange={(e) => setAddress(e.target.value)}
        />
        <small className="field-hint">
          The hostname must resolve to at least one valid IPv4 or IPv6 address.
        </small>
      </Field>
      <div className="node-port-editor">
        <div className="allocation-heading">
          <div>
            <div className="subsection-label">AVAILABLE PORTS</div>
            <small>
              Each port will be allocated on every IP returned by DNS.
            </small>
          </div>
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
        {ports.map((port, index) => (
          <div className="node-port-row" key={index}>
            <label>
              Port {index + 1}
              <input
                required
                type="number"
                min="1"
                max="65535"
                value={port}
                onChange={(e) =>
                  setPorts((old) =>
                    old.map((p, i) => (i === index ? e.target.value : p)),
                  )
                }
              />
            </label>
            <button
              type="button"
              className="icon-button"
              title="Remove port"
              disabled={ports.length === 1}
              onClick={() =>
                setPorts((old) => old.filter((_, i) => i !== index))
              }
            >
              <Trash size={14} />
            </button>
          </div>
        ))}
      </div>
      <Field label="Panel URL reachable from this node">
        <input
          type="url"
          required
          value={panelURL}
          onChange={(e) => setPanelURL(e.target.value)}
        />
        <small className="field-hint">
          This URL is for the node agent to connect back to the panel.
        </small>
      </Field>
      {error && <div className="form-error">{error}</div>}
      <FormActions cancel={onCancel} busy={busy} label="Register node" />
    </form>
  );
}

/** Create or edit a user account and assign its roles. */
export function UserForm({
  roles,
  user,
  currentUser,
  onSaved,
  onCancel,
}: {
  roles: Role[];
  user: User | null;
  currentUser: User;
  onSaved: () => void;
  onCancel: () => void;
}) {
  const editing = !!user;
  const assignableRoles = roles.filter(
    (item) =>
      item.id === user?.role ||
      currentUser.role === "root" ||
      item.permissions.every((permission) =>
        currentUser.permissions?.includes(permission),
      ),
  );
  const [username, setUsername] = useState(user?.username || "");
  const [email, setEmail] = useState(user?.email || "");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState(
    user?.role ||
      assignableRoles.find((item) => item.id === "user")?.id ||
      assignableRoles[0]?.id ||
      "",
  );
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api(
        user
          ? `/api/admin/users/${encodeURIComponent(user.id)}`
          : "/api/admin/users",
        {
          method: user ? "PUT" : "POST",
          body: JSON.stringify({
            username,
            email,
            ...(password ? { password } : {}),
            ...(user?.role === "root" ? {} : { role }),
          }),
        },
      );
      onSaved();
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="form-stack modal-body" onSubmit={submit}>
      <p className="modal-intro">
        {editing
          ? `Edit ${user.username}'s panel account.`
          : "Create a panel account for another operator."}
      </p>
      <Field label="Username">
        <input
          required
          minLength={3}
          value={username}
          onChange={(e) => setUsername(e.target.value)}
        />
      </Field>
      <Field label="Email">
        <input
          required
          type="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
      </Field>
      <Field label="Password">
        <input
          required={!editing}
          minLength={password ? 8 : undefined}
          type="password"
          autoComplete="new-password"
          placeholder={
            editing ? "Leave blank to keep the current password" : undefined
          }
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
      </Field>
      {user?.role === "root" ? (
        <Field label="Role">
          <input value="Root · all permissions · protected" disabled />
        </Field>
      ) : (
        <Field label="Role">
          <select
            required
            value={role}
            onChange={(e) => setRole(e.target.value)}
          >
            {assignableRoles.map((item) => (
              <option key={item.id} value={item.id}>
                {item.name}
              </option>
            ))}
          </select>
        </Field>
      )}
      {error && <div className="form-error">{error}</div>}
      <FormActions
        cancel={onCancel}
        busy={busy}
        label={editing ? "Save changes" : "Create user"}
      />
    </form>
  );
}

/** Create or edit a role and select its allowed permissions. */
export function RoleForm({
  role,
  permissions,
  onSaved,
  onCancel,
}: {
  role: Role | null;
  permissions: PermissionDefinition[];
  onSaved: () => void;
  onCancel: () => void;
}) {
  const [name, setName] = useState(role?.name || "");
  const [description, setDescription] = useState(role?.description || "");
  const [selected, setSelected] = useState<string[]>(role?.permissions || []);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const groups = permissions.reduce<Record<string, PermissionDefinition[]>>(
    (result, item) => {
      (result[item.group] ||= []).push(item);
      return result;
    },
    {},
  );
  const toggle = (key: string) =>
    setSelected((current) =>
      current.includes(key)
        ? current.filter((item) => item !== key)
        : [...current, key],
    );
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api(
        role ? `/api/roles/${encodeURIComponent(role.id)}` : "/api/roles",
        {
          method: role ? "PUT" : "POST",
          body: JSON.stringify({ name, description, permissions: selected }),
        },
      );
      onSaved();
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <form className="form-stack modal-body role-form" onSubmit={submit}>
      <p className="modal-intro">
        Choose the actions this role can access in the panel.
      </p>
      <Field label="Role name">
        <input
          required
          maxLength={48}
          autoFocus
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
      </Field>
      <Field label="Description">
        <input
          maxLength={180}
          value={description}
          onChange={(event) => setDescription(event.target.value)}
        />
      </Field>
      <div className="role-permission-groups">
        {Object.entries(groups).map(([group, items]) => (
          <section className="role-permission-group" key={group}>
            <h3>{group}</h3>
            {items.map((permission) => (
              <label className="role-permission-option" key={permission.key}>
                <input
                  type="checkbox"
                  checked={selected.includes(permission.key)}
                  onChange={() => toggle(permission.key)}
                />
                <span>{permission.label}</span>
              </label>
            ))}
          </section>
        ))}
      </div>
      {error && <div className="form-error">{error}</div>}
      <FormActions
        cancel={onCancel}
        busy={busy}
        label={role ? "Save role" : "Create role"}
      />
    </form>
  );
}

/** Show node installation commands and allow its enrollment token to be rotated. */
export function NodeTokenCard({
  result,
  onClose,
}: {
  result: NodeEnrollment;
  onClose: () => void;
}) {
  const [copied, setCopied] = useState(false);
  const command =
    "PANEL_URL=" +
    shellQuote(result.panelURL || location.origin) +
    " NODE_TOKEN=" +
    shellQuote(result.token) +
    " NODE_NAME=" +
    shellQuote(result.node.name) +
    " go run ./cmd/node";
  if (!result.token) {
    return (
      <div className="modal-body">
        <p className="modal-intro">
          The node token is displayed only once. Register a replacement node if
          a new token is needed.
        </p>
        <div className="form-actions">
          <Button variant="primary" onClick={onClose}>
            Done
          </Button>
        </div>
      </div>
    );
  }
  return (
    <div className="modal-body">
      <p className="modal-intro">
        Run this from the project checkout on{" "}
        <strong>{result.node.name}</strong>. Keep the token private.
      </p>
      <div className="token-box">
        <span>NODE TOKEN</span>
        <code>{result.token}</code>
      </div>
      <div className="command-box">
        <div>
          <span>AGENT START COMMAND</span>
          <Button
            variant="subtle"
            size="tiny"

            onClick={() =>
              navigator.clipboard.writeText(command).then(() => setCopied(true))
            }
          >
            {copied ? <Check size={14} /> : <Clipboard size={14} />}{" "}
            {copied ? "Copied" : "Copy"}
          </Button>
        </div>
        <pre>{command}</pre>
      </div>
      <div className="form-actions">
        <Button variant="primary" onClick={onClose}>
          I saved the token
        </Button>
      </div>
    </div>
  );
}

/** Escape a value for safe inclusion in a POSIX shell command. */
export function shellQuote(value: string) {
  return "'" + value.replace(/'/g, "'\\''") + "'";
}
