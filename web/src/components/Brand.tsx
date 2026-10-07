import { Server } from "lucide-react";

/** Display the Control Plane mark with an optional full wordmark. */
export function Brand({ full = false }: { full?: boolean }) {
  return (
    <div className={"brand " + (full ? "brand-full" : "")}>
      <div className="brand-mark">
        <Server size={19} />
      </div>
      {full && (
        <div>
          <strong>CONTROL</strong>
          <small>PLANE</small>
        </div>
      )}
    </div>
  );
}
