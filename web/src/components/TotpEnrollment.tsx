import { QRCodeSVG } from "qrcode.react";

/** Show a TOTP provisioning QR code and a manual setup key. */
export function TotpEnrollment({
  otpauthUrl,
  secret,
}: {
  otpauthUrl: string;
  secret: string;
}) {
  return (
    <div className="totp-enrollment">
      <div className="totp-qr-code">
        <QRCodeSVG
          value={otpauthUrl}
          size={176}
          level="M"
          marginSize={2}
          title="Scan to add this account to your authenticator app"
        />
      </div>
      <div className="totp-secret-panel">
        <span>SETUP KEY</span>
        <code>{secret}</code>
        <a href={otpauthUrl}>Open authenticator setup</a>
        <small>
          Scan the QR code or enter this key manually in your authenticator
          app. Choose a time based code.
        </small>
      </div>
    </div>
  );
}
