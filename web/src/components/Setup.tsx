import { useState, type FormEvent } from "react";
import { api, errorMessage } from "../shared/domain";
import { Brand } from "./Brand";
import { Field } from "./Field";
import { RecoveryCodesPanel } from "./RecoveryCodesPanel";
import { TotpEnrollment } from "./TotpEnrollment";

/** Create the first root account and verify authenticator enrollment. */
export function Setup({ onDone }: { onDone: () => void }) {
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [enrollment, setEnrollment] = useState<{
    setupId: string;
    secret: string;
    otpauthUrl: string;
  } | null>(null);
  const [code, setCode] = useState("");
  const [recoveryCodes, setRecoveryCodes] = useState<string[] | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const result = await api<{
        setupId: string;
        secret: string;
        otpauthUrl: string;
      }>("/api/setup", {
        method: "POST",
        body: JSON.stringify({ username, email, password }),
      });
      setEnrollment(result);
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };
  const verify = async (e: FormEvent) => {
    e.preventDefault();
    if (!enrollment) return;
    setBusy(true);
    setError("");
    try {
      const result = await api<{ recoveryCodes: string[] }>(
        "/api/setup/verify",
        {
          method: "POST",
          body: JSON.stringify({ setupId: enrollment.setupId, code }),
        },
      );
      setRecoveryCodes(result.recoveryCodes);
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="auth-page">
      <div className={`auth-card${enrollment ? " auth-card-totp" : ""}`}>
        <Brand />
        <div className="auth-kicker">Setup</div>
        {recoveryCodes ? (
          <>
            <h1>Save recovery codes</h1>
            <p>
              Authenticator setup is complete. Save these codes before
              continuing.
            </p>
            <RecoveryCodesPanel codes={recoveryCodes} onDone={onDone} />
          </>
        ) : !enrollment ? (
          <>
            <h1>Create root account</h1>
            <p>
              This account manages nodes, configs, and server access.
              Authenticator setup is required.
            </p>
            <form className="form-stack" onSubmit={submit}>
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
                  required
                  minLength={8}
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
                <small className="field-hint">At least 8 characters</small>
              </Field>
              {error && <div className="form-error">{error}</div>}
              <button className="button primary full-width" disabled={busy}>
                {busy ? "Preparing setup…" : "Continue to authenticator setup"}
              </button>
            </form>
          </>
        ) : (
          <>
            <h1>Set up authenticator</h1>
            <p>
              Add this account to Google Authenticator or another TOTP app.
              Enter the six digit code to finish creating your account.
            </p>
            <form className="form-stack" onSubmit={verify}>
              <TotpEnrollment
                otpauthUrl={enrollment.otpauthUrl}
                secret={enrollment.secret}
              />
              <Field label="6 digit authenticator code">
                <input
                  className="otp-input"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  pattern="[0-9]{6}"
                  maxLength={6}
                  required
                  value={code}
                  onChange={(e) =>
                    setCode(e.target.value.replace(/\D/g, "").slice(0, 6))
                  }
                />
              </Field>
              {error && <div className="form-error">{error}</div>}
              <button className="button primary full-width" disabled={busy}>
                {busy ? "Verifying…" : "Verify and create account"}
              </button>
            </form>
          </>
        )}
      </div>
    </div>
  );
}
