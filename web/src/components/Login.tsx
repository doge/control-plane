import { errorMessage } from "../shared/domain";
import { useState, type FormEvent } from "react";
import { Brand } from "./Brand";
import { Field } from "./Field";

/** Keep six-digit codes numeric and group recovery codes for readability. */
function formatLoginCode(value: string): string {
  const normalized = value.toUpperCase();
  if (/^\d{0,6}$/.test(normalized)) return normalized;
  const recoveryCode = normalized.replace(/[^A-Z2-7]/g, "").slice(0, 16);
  return recoveryCode.match(/.{1,4}/g)?.join("-") || "";
}

/** Authenticate a user and complete an optional second-factor challenge. */
export function Login({
  onLogin,
  onLoginOTP,
}: {
  onLogin: (
    username: string,
    password: string,
  ) => Promise<{ requiresOTP: boolean; challengeId?: string }>;
  onLoginOTP: (challengeId: string, code: string) => Promise<void>;
}) {
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [challengeId, setChallengeId] = useState("");
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      if (challengeId) await onLoginOTP(challengeId, code);
      else {
        const result = await onLogin(username, password);
        if (result.requiresOTP && result.challengeId) {
          setChallengeId(result.challengeId);
        }
      }
    } catch (err: unknown) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="auth-page">
      <div className="auth-card">
        <Brand />
        <div className="auth-kicker">CONTROL PLANE</div>
        <h1>{challengeId ? "Verify it’s you" : "Welcome back"}</h1>
        <p>
          {challengeId
            ? "Enter a current authenticator code or one of your unused recovery codes."
            : "Sign in to manage your game servers."}
        </p>
        <form className="form-stack" onSubmit={submit}>
          {!challengeId ? (
            <>
              <Field label="Username or email">
                <input
                  autoComplete="username"
                  required
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                />
              </Field>
              <Field label="Password">
                <input
                  autoComplete="current-password"
                  required
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
              </Field>
            </>
          ) : (
            <Field label="Authenticator code or recovery code">
              <input
                autoFocus
                className={`otp-input${/[A-Z]/.test(code) || code.replace(/\D/g, "").length > 6 ? " otp-recovery-input" : ""}`}
                autoCapitalize="characters"
                autoComplete="one-time-code"
                inputMode="text"
                maxLength={19}
                placeholder="123456 or XXXX-XXXX-XXXX-XXXX"
                required
                value={code}
                onChange={(e) => setCode(formatLoginCode(e.target.value))}
              />
              <small className="field-hint">
                Each recovery code works once and can replace your 6 digit code.
              </small>
            </Field>
          )}
          {error && <div className="form-error">{error}</div>}
          <button className="button primary full-width" disabled={busy}>
            {busy
              ? "Signing in…"
              : challengeId
                ? "Verify and sign in"
                : "Sign in"}
          </button>
          {challengeId && (
            <button
              className="button subtle full-width"
              type="button"

              onClick={() => {
                setChallengeId("");
                setCode("");
                setError("");
              }}
            >
              Back to password
            </button>
          )}
        </form>
      </div>
    </div>
  );
}
