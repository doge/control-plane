import { useState } from "react";
import { Clipboard } from "lucide-react";

/** Display one-time recovery codes and let the user copy them safely. */
export function RecoveryCodesPanel({
  codes,
  onDone,
}: {
  codes: string[];
  onDone: () => void;
}) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(codes.join("\n"));
      setCopied(true);
    } catch {
      setCopied(false);
    }
  };
  return (
    <section className="recovery-codes-panel">
      <p>
        Save these codes somewhere private. Each code can be used once if you
        cannot access your authenticator. They will only be shown here once.
      </p>
      <div className="recovery-code-grid">
        {codes.map((code) => (
          <code key={code}>{code}</code>
        ))}
      </div>
      <div className="recovery-code-actions">
        <button
          type="button"
          className="button subtle"
          onClick={() => void copy()}
        >
          <Clipboard size={15} />
          {copied ? "Copied" : "Copy codes"}
        </button>
        <button type="button" className="button primary" onClick={onDone}>
          I've saved these codes
        </button>
      </div>
    </section>
  );
}
