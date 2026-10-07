import { Button } from "../../components/Button";
import { FormEvent, useEffect, useState } from "react";
import {
  Check,
  ChevronRight,
  Clipboard,
  KeyRound,
  LogOut,
  Mail,
  Shield,
  UserRound,
} from "lucide-react";

import { errorMessage, api, type User } from "../../shared/domain";
import { showToast } from "../../shared/toast";

import {
  Detail,
  Field,
  Modal,
  PageHeading,
  PanelHeading,
  RecoveryCodesPanel,
  TotpEnrollment,
} from "../../components";

/** Manage the signed-in user's profile, security, and display preferences. */
export function SettingsPage({
  user,
  theme,
  onTheme,
  onLogout,
  onProfileUpdated,
}: {
  user: User;
  theme: string;
  onTheme: (t: string) => void;
  onLogout: () => void;
  onProfileUpdated: (user: User) => void;
}) {
  const [activeRow, setActiveRow] = useState<
    "username" | "email" | "password" | "authenticator" | null
  >(null);
  const [username, setUsername] = useState(user.username);
  const [email, setEmail] = useState(user.email);
  const [profilePassword, setProfilePassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [profileError, setProfileError] = useState("");
  const [profileBusy, setProfileBusy] = useState(false);
  const [totpBusy, setTotpBusy] = useState(false);
  const [totpSetup, setTotpSetup] = useState<{
    secret: string;
    otpauthUrl: string;
  } | null>(null);
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);
  const [removeTotp, setRemoveTotp] = useState(false);
  const [removeTotpCode, setRemoveTotpCode] = useState("");
  const [regenerateRecoveryCodes, setRegenerateRecoveryCodes] = useState(false);
  const [recoveryCodeOTP, setRecoveryCodeOTP] = useState("");
  const [totpPassword, setTotpPassword] = useState("");
  const [totpCode, setTotpCode] = useState("");
  const [totpEnabled, setTotpEnabled] = useState(Boolean(user.totpEnabled));
  const [totpError, setTotpError] = useState("");
  const [httpsRequired, setHttpsRequired] = useState(false);
  const [httpsDomain, setHttpsDomain] = useState("");
  const [certificateEmail, setCertificateEmail] = useState("");
  const [httpsPlanSaved, setHttpsPlanSaved] = useState(false);
  const securePage = location.protocol === "https:";

  useEffect(() => {
    api<{
      httpsRequired: boolean;
      httpsDomain: string;
      certificateEmail: string;
    }>("/api/settings/transport")
      .then((value) => {
        setHttpsRequired(value.httpsRequired);
        setHttpsDomain(value.httpsDomain || "");
        setCertificateEmail(value.certificateEmail || "");
        setHttpsPlanSaved(Boolean(value.httpsDomain));
      })
      .catch(() => {});
  }, []);

  const openRow = (row: typeof activeRow) => {
    setActiveRow(activeRow === row ? null : row);
    setProfileError("");
    setTotpError("");
    setProfilePassword("");
    setNewPassword("");
    setTotpPassword("");
    setTotpCode("");
    setTotpSetup(null);
    setRecoveryCodes(null);
    setRemoveTotp(false);
    setRemoveTotpCode("");
    setRegenerateRecoveryCodes(false);
    setRecoveryCodeOTP("");
  };

  const saveProfile = async (event: FormEvent) => {
    event.preventDefault();
    setProfileBusy(true);
    setProfileError("");
    try {
      const updated = await api<User>("/api/auth/profile", {
        method: "PUT",
        body: JSON.stringify({
          username,
          email,
          currentPassword: profilePassword,
        }),
      });
      onProfileUpdated(updated);
      setProfilePassword("");
      setActiveRow(null);
    } catch (error: unknown) {
      setProfileError(errorMessage(error));
    } finally {
      setProfileBusy(false);
    }
  };

  const updatePassword = async (event: FormEvent) => {
    event.preventDefault();
    setProfileBusy(true);
    setProfileError("");
    try {
      await api("/api/auth/password", {
        method: "POST",
        body: JSON.stringify({ currentPassword: profilePassword, newPassword }),
      });
      setProfilePassword("");
      setNewPassword("");
      setActiveRow(null);
    } catch (error: unknown) {
      setProfileError(errorMessage(error));
    } finally {
      setProfileBusy(false);
    }
  };

  const beginTotp = async (event: FormEvent) => {
    event.preventDefault();
    setTotpError("");
    setTotpBusy(true);
    try {
      const setup = await api<{ secret: string; otpauthUrl: string }>(
        "/api/auth/totp/enroll",
        { method: "POST", body: JSON.stringify({ password: totpPassword }) },
      );
      setTotpSetup(setup);
      setTotpPassword("");
    } catch (error: unknown) {
      setTotpError(errorMessage(error));
    } finally {
      setTotpBusy(false);
    }
  };

  const finishTotp = async (event: FormEvent) => {
    event.preventDefault();
    setTotpError("");
    setTotpBusy(true);
    try {
      const result = await api<{ recoveryCodes: string[] }>(
        "/api/auth/totp/verify",
        {
          method: "POST",
          body: JSON.stringify({ code: totpCode }),
        },
      );
      setTotpEnabled(true);
      onProfileUpdated({ ...user, totpEnabled: true });
      setTotpSetup(null);
      setTotpCode("");
      setRecoveryCodes(result.recoveryCodes);
    } catch (error: unknown) {
      setTotpError(errorMessage(error));
    } finally {
      setTotpBusy(false);
    }
  };

  const removeAuthenticator = async (event: FormEvent) => {
    event.preventDefault();
    setTotpError("");
    setTotpBusy(true);
    try {
      await api("/api/auth/totp/remove", {
        method: "POST",
        body: JSON.stringify({ code: removeTotpCode }),
      });
      setTotpEnabled(false);
      onProfileUpdated({ ...user, totpEnabled: false });
      setRemoveTotp(false);
      setRemoveTotpCode("");
      setActiveRow(null);
    } catch (error: unknown) {
      setTotpError(errorMessage(error));
    } finally {
      setTotpBusy(false);
    }
  };

  const createRecoveryCodes = async (event: FormEvent) => {
    event.preventDefault();
    setTotpError("");
    setTotpBusy(true);
    try {
      const result = await api<{ recoveryCodes: string[] }>(
        "/api/auth/totp/recovery-codes",
        {
          method: "POST",
          body: JSON.stringify({ code: recoveryCodeOTP }),
        },
      );
      setRecoveryCodes(result.recoveryCodes);
      setRegenerateRecoveryCodes(false);
      setRecoveryCodeOTP("");
    } catch (error: unknown) {
      setTotpError(errorMessage(error));
    } finally {
      setTotpBusy(false);
    }
  };

  const saveHTTPSSetup = async (event: FormEvent) => {
    event.preventDefault();
    try {
      const result = await api<{
        httpsRequired: boolean;
        httpsDomain: string;
        certificateEmail: string;
      }>("/api/settings/transport", {
        method: "PUT",
        body: JSON.stringify({
          httpsRequired,
          httpsDomain,
          certificateEmail,
        }),
      });
      setHttpsRequired(result.httpsRequired);
      setHttpsDomain(result.httpsDomain);
      setCertificateEmail(result.certificateEmail);
      setHttpsPlanSaved(true);
      showToast("HTTPS setup details saved.", "success");
    } catch (error: unknown) {
      showToast(errorMessage(error));
    }
  };

  const setHTTPS = async (enabled: boolean) => {
    try {
      const result = await api<{
        httpsRequired: boolean;
        httpsDomain: string;
        certificateEmail: string;
      }>("/api/settings/transport", {
        method: "PUT",
        body: JSON.stringify({
          httpsRequired: enabled,
          httpsDomain,
          certificateEmail,
        }),
      });
      setHttpsRequired(result.httpsRequired);
      setHttpsDomain(result.httpsDomain);
      setCertificateEmail(result.certificateEmail);
      setHttpsPlanSaved(Boolean(result.httpsDomain));
      showToast(
        enabled ? "HTTPS enforcement enabled." : "HTTPS enforcement disabled.",
        "success",
      );
    } catch (error: unknown) {
      showToast(errorMessage(error));
    }
  };

  const httpsInstallCommand = `sudo /opt/control-plane/scripts/install-https.sh ${shellArgument(
    httpsDomain,
  )} ${shellArgument(certificateEmail)}`;
  const copyHTTPSCommand = async () => {
    try {
      await navigator.clipboard.writeText(httpsInstallCommand);
      showToast("Installer command copied.", "success");
    } catch {
      showToast("Could not copy the command. Select and copy it instead.");
    }
  };

  return (
    <div className="page-stack">
      <PageHeading
        eyebrow="PREFERENCES"
        title="Settings"
        description="Set your dashboard appearance and account security."
      />
      <section className="panel-card settings-card">
        <PanelHeading
          title="Appearance"
          sub="Theme applies immediately and is saved in this browser."
        />
        <div className="theme-options">
          <ThemeOption
            name="dark"
            title="Midnight"
            detail="Charcoal dark"
            selected={theme === "dark"}
            onClick={() => onTheme("dark")}
          />
          <ThemeOption
            name="nord"
            title="Nord"
            detail="Arctic blue-gray"
            selected={theme === "nord"}
            onClick={() => onTheme("nord")}
          />
        </div>
      </section>
      <section className="panel-card settings-card">
        <PanelHeading
          title="Account"
          sub="Select an item to update it. Your password is required to save account changes."
        />
        <div className="account-settings-list">
          <AccountSettingRow
            icon={UserRound}
            label="Username"
            value={user.username}
            onClick={() => openRow("username")}
          />
          <AccountSettingRow
            icon={Mail}
            label="Email"
            value={user.email}
            onClick={() => openRow("email")}
          />
          <AccountSettingRow
            icon={KeyRound}
            label="Password"
            value="Change your sign-in password"
            onClick={() => openRow("password")}
          />
          <AccountSettingRow
            icon={Shield}
            label="Authenticator"
            value="Two-step sign-in with an authenticator app"
            onClick={() => openRow("authenticator")}
            status={totpEnabled ? "Enabled" : "Disabled"}
            statusTone={totpEnabled ? "success" : "danger"}
          />
        </div>
        <Detail label="Role">
          <span className="role-pill">{user.role}</span>
        </Detail>
        <div className="settings-footer">
          <Button variant="danger-outline" onClick={onLogout}>
            <LogOut size={15} />
            Sign out
          </Button>
        </div>
      </section>
      {activeRow && (
        <Modal
          title={
            activeRow === "username"
              ? "Change username"
              : activeRow === "email"
                ? "Change email"
                : activeRow === "password"
                  ? "Change password"
                  : recoveryCodes
                    ? "Save recovery codes"
                    : regenerateRecoveryCodes
                      ? "Generate recovery codes"
                      : removeTotp
                        ? "Remove authenticator"
                        : totpSetup
                          ? "Verify authenticator"
                          : totpEnabled
                            ? "Replace authenticator"
                            : "Set up authenticator"
          }
          onClose={() => openRow(null)}
        >
          {activeRow === "username" && (
            <form className="form-stack modal-body" onSubmit={saveProfile}>
              <Field label="Username">
                <input
                  required
                  minLength={3}
                  autoFocus
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                />
              </Field>
              <Field label="Current password">
                <input
                  required
                  type="password"
                  autoComplete="current-password"
                  value={profilePassword}
                  onChange={(event) => setProfilePassword(event.target.value)}
                />
              </Field>
              {profileError && <div className="form-error">{profileError}</div>}
              <AccountEditorActions
                busy={profileBusy}
                onCancel={() => openRow(null)}
                label="Save username"
              />
            </form>
          )}
          {activeRow === "email" && (
            <form className="form-stack modal-body" onSubmit={saveProfile}>
              <Field label="Email">
                <input
                  required
                  type="email"
                  autoFocus
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                />
              </Field>
              <Field label="Current password">
                <input
                  required
                  type="password"
                  autoComplete="current-password"
                  value={profilePassword}
                  onChange={(event) => setProfilePassword(event.target.value)}
                />
              </Field>
              {profileError && <div className="form-error">{profileError}</div>}
              <AccountEditorActions
                busy={profileBusy}
                onCancel={() => openRow(null)}
                label="Save email"
              />
            </form>
          )}
          {activeRow === "password" && (
            <form className="form-stack modal-body" onSubmit={updatePassword}>
              <Field label="Current password">
                <input
                  required
                  autoFocus
                  type="password"
                  autoComplete="current-password"
                  value={profilePassword}
                  onChange={(event) => setProfilePassword(event.target.value)}
                />
              </Field>
              <Field label="New password">
                <input
                  required
                  minLength={8}
                  type="password"
                  autoComplete="new-password"
                  value={newPassword}
                  onChange={(event) => setNewPassword(event.target.value)}
                />
                <small className="field-hint">At least 8 characters</small>
              </Field>
              {profileError && <div className="form-error">{profileError}</div>}
              <AccountEditorActions
                busy={profileBusy}
                onCancel={() => openRow(null)}
                label="Update password"
              />
            </form>
          )}
          {activeRow === "authenticator" && recoveryCodes && (
            <div className="modal-body">
              <RecoveryCodesPanel
                codes={recoveryCodes}
                onDone={() => {
                  setRecoveryCodes(null);
                  setActiveRow(null);
                }}
              />
            </div>
          )}
          {activeRow === "authenticator" && removeTotp && (
            <form
              className="form-stack modal-body"
              onSubmit={removeAuthenticator}
            >
              <p className="modal-intro">
                Enter a current authenticator code or an unused recovery code.
                Using a recovery code consumes it and turns off two-step
                sign-in. You can set up a new authenticator afterward.
              </p>
              <Field label="Authenticator code or recovery code">
                <input
                  autoFocus
                  autoCapitalize="characters"
                  autoComplete="one-time-code"
                  inputMode="text"
                  maxLength={24}
                  placeholder="123456 or XXXX-XXXX-XXXX-XXXX"
                  required
                  value={removeTotpCode}
                  onChange={(event) =>
                    setRemoveTotpCode(event.target.value.toUpperCase())
                  }
                />
              </Field>
              {totpError && <div className="form-error">{totpError}</div>}
              <div className="account-editor-actions">
                <Button
                  variant="subtle"
                  type="button"

                  onClick={() => {
                    setRemoveTotp(false);
                    setRemoveTotpCode("");
                    setTotpError("");
                  }}
                >
                  Cancel
                </Button>
                <Button variant="danger-outline" disabled={totpBusy}>
                  {totpBusy ? "Removing…" : "Remove authenticator"}
                </Button>
              </div>
            </form>
          )}
          {activeRow === "authenticator" && regenerateRecoveryCodes && (
            <form
              className="form-stack modal-body"
              onSubmit={createRecoveryCodes}
            >
              <p className="modal-intro">
                Enter a current authenticator code to replace your recovery
                codes. Any previous recovery codes will stop working.
              </p>
              <Field label="Current 6 digit authenticator code">
                <input
                  autoFocus
                  className="otp-input"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  pattern="[0-9]{6}"
                  maxLength={6}
                  required
                  value={recoveryCodeOTP}
                  onChange={(event) =>
                    setRecoveryCodeOTP(
                      event.target.value.replace(/\D/g, "").slice(0, 6),
                    )
                  }
                />
              </Field>
              {totpError && <div className="form-error">{totpError}</div>}
              <div className="account-editor-actions">
                <Button
                  variant="subtle"
                  type="button"

                  onClick={() => {
                    setRegenerateRecoveryCodes(false);
                    setRecoveryCodeOTP("");
                    setTotpError("");
                  }}
                >
                  Cancel
                </Button>
                <Button variant="primary" disabled={totpBusy}>
                  {totpBusy ? "Generating…" : "Generate recovery codes"}
                </Button>
              </div>
            </form>
          )}
          {activeRow === "authenticator" &&
            !totpSetup &&
            !recoveryCodes &&
            !removeTotp &&
            !regenerateRecoveryCodes && (
              <form className="form-stack modal-body" onSubmit={beginTotp}>
                <p className="modal-intro">
                  {totpEnabled
                    ? "Your current authenticator stays active until you verify a replacement. Replacing it creates new recovery codes and invalidates the previous set."
                    : "Set up Google Authenticator or another compatible TOTP app."}
                </p>
                <Field label="Current password">
                  <input
                    required
                    autoFocus
                    type="password"
                    autoComplete="current-password"
                    value={totpPassword}
                    onChange={(event) => setTotpPassword(event.target.value)}
                  />
                </Field>
                {totpError && <div className="form-error">{totpError}</div>}
                <AccountEditorActions
                  busy={totpBusy}
                  onCancel={() => openRow(null)}
                  label={
                    totpEnabled
                      ? "Replace authenticator"
                      : "Set up authenticator"
                  }
                />
                {totpEnabled && (
                  <div className="totp-management-actions">
                    <Button
                      variant="subtle"
                      type="button"

                      onClick={() => {
                        setTotpError("");
                        setRegenerateRecoveryCodes(true);
                      }}
                    >
                      Generate new recovery codes
                    </Button>
                    <Button
                      variant="danger-outline"
                      type="button"

                      onClick={() => {
                        setTotpError("");
                        setRemoveTotp(true);
                      }}
                    >
                      Remove authenticator
                    </Button>
                  </div>
                )}
              </form>
            )}
          {activeRow === "authenticator" && totpSetup && (
            <form className="form-stack modal-body" onSubmit={finishTotp}>
              <p className="modal-intro">
                Add this setup key to your authenticator app, then enter its
                current six digit code.
              </p>
              <TotpEnrollment
                otpauthUrl={totpSetup.otpauthUrl}
                secret={totpSetup.secret}
              />
              <Field label="6 digit authenticator code">
                <input
                  className="otp-input"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  pattern="[0-9]{6}"
                  maxLength={6}
                  required
                  value={totpCode}
                  onChange={(event) =>
                    setTotpCode(
                      event.target.value.replace(/\D/g, "").slice(0, 6),
                    )
                  }
                />
              </Field>
              {totpError && <div className="form-error">{totpError}</div>}
              <AccountEditorActions
                busy={totpBusy}
                onCancel={() => openRow(null)}
                label="Verify authenticator"
              />
            </form>
          )}
        </Modal>
      )}
      {user.role === "root" && (
        <section className="panel-card settings-card">
          <PanelHeading
            title="HTTPS and secure sockets"
            sub="Configure the public hostname and certificate contact, then enable HTTPS after TLS is installed."
          />
          <form className="https-setup-form" onSubmit={saveHTTPSSetup}>
            <div className="form-two">
              <Field label="Panel hostname">
                <input
                  required
                  placeholder="panel.example.com"
                  disabled={httpsRequired}
                  value={httpsDomain}
                  onChange={(event) => {
                    setHttpsDomain(event.target.value);
                    setHttpsPlanSaved(false);
                  }}
                />
                <small className="field-hint">
                  DNS must point to this server before requesting a certificate.
                </small>
              </Field>
              <Field label="Certificate contact email">
                <input
                  type="email"
                  placeholder="admin@example.com"
                  disabled={httpsRequired}
                  value={certificateEmail}
                  onChange={(event) => {
                    setCertificateEmail(event.target.value);
                    setHttpsPlanSaved(false);
                  }}
                />
              </Field>
            </div>
            <Button variant="subtle" disabled={httpsRequired}>
              Save HTTPS setup
            </Button>
          </form>
          {httpsPlanSaved && httpsDomain && !httpsRequired && (
            <div className="https-install-plan">
              <div>
                <strong>Install TLS on the panel host</strong>
                <p>
                  Run this command on the panel machine. Then visit the HTTPS
                  URL below and enable HTTPS here.
                </p>
              </div>
              <code>{httpsInstallCommand}</code>
              <div className="https-plan-actions">
                <Button
                  variant="subtle"
                  type="button"

                  onClick={() => void copyHTTPSCommand()}
                >
                  <Clipboard size={14} />
                  Copy command
                </Button>
                <a className="button subtle" href={`https://${httpsDomain}`}>
                  Open secure panel
                </a>
              </div>
            </div>
          )}
          <div className="transport-options">
            <button
              type="button"
              className={!httpsRequired ? "selected" : ""}
              disabled={httpsRequired}
              onClick={() => void setHTTPS(false)}
            >
              HTTP allowed
            </button>
            <button
              type="button"
              className={httpsRequired ? "selected" : ""}
              disabled={!httpsRequired && (!securePage || !httpsPlanSaved)}
              onClick={() => void setHTTPS(!httpsRequired)}
            >
              {httpsRequired
                ? "HTTPS required · click to allow HTTP"
                : "Require HTTPS"}
            </button>
          </div>
          <p className="field-hint https-setup-hint">
            {httpsRequired
              ? "HTTPS is enforced. HTTP is disabled and redirected; node agents connect over WSS."
              : !httpsPlanSaved
                ? "Save the hostname and email to prepare the TLS install flow. HTTP remains available until you enable enforcement."
                : securePage
                  ? "TLS is reachable. Require HTTPS to redirect HTTP requests and secure panel sessions."
                  : "Install the Caddy proxy, open the panel over HTTPS, then enable enforcement. Caddy manages certificate renewal and supports the node WebSocket connection."}
          </p>
        </section>
      )}
    </div>
  );
}

function shellArgument(value: string) {
  return "'" + value.replaceAll("'", "'\\''") + "'";
}

function AccountSettingRow({
  icon: Icon,
  label,
  value,
  onClick,
  status,
  statusTone,
}: {
  icon: typeof Shield;
  label: string;
  value: string;
  onClick: () => void;
  status?: string;
  statusTone?: "success" | "danger";
}) {
  return (
    <button type="button" className="account-setting-row" onClick={onClick}>
      <span className="account-row-icon">
        <Icon size={16} />
      </span>
      <span className="account-row-copy">
        <strong>{label}</strong>
        <small>{value}</small>
      </span>
      <span className="account-row-status-slot">
        {status && (
          <span className={`account-row-status ${statusTone || ""}`}>
            {status}
          </span>
        )}
      </span>
      <ChevronRight className="account-row-chevron" size={16} />
    </button>
  );
}

function AccountEditorActions({
  busy,
  onCancel,
  label,
}: {
  busy: boolean;
  onCancel: () => void;
  label: string;
}) {
  return (
    <div className="account-editor-actions">
      <Button variant="subtle" type="button" onClick={onCancel}>
        Cancel
      </Button>
      <Button variant="primary" disabled={busy}>
        {busy ? "Saving…" : label}
      </Button>
    </div>
  );
}

/** Render one selectable theme option for the appearance settings. */
export function ThemeOption({
  name,
  title,
  detail,
  selected,
  onClick,
}: {
  name: string;
  title: string;
  detail: string;
  selected: boolean;
  onClick: () => void;
}) {
  return (
    <button
      className={"theme-option " + (selected ? "selected" : "")}
      onClick={onClick}
    >
      <div className={"theme-preview preview-" + name}>
        <span />
        <span />
        <span />
      </div>
      <div>
        <strong>{title}</strong>
        <small>{detail}</small>
      </div>
      {selected && <Check size={17} />}
    </button>
  );
}
