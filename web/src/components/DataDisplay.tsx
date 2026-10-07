import React from "react";

/** Show a concise, color-coded state label. */
export function Status({ status }: { status: string }) {
  const s = (status || "unknown").toLowerCase();
  return (
    <span className={"status-pill " + s}>
      <span className="status-dot" />
      {s}
    </span>
  );
}

/** Present a labeled value in a consistent key/value row. */
export function Detail({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="detail-line">
      <span>{label}</span>
      <div>{children}</div>
    </div>
  );
}

type EntityProps = {
  icon: import("lucide-react").LucideIcon;
  title: React.ReactNode;
  sub: React.ReactNode;
};

/** Present an icon, primary label, and supporting detail as one entity. */
export function Entity({ icon: Icon, title, sub }: EntityProps) {
  return (
    <div className="entity-cell">
      <div className="entity-icon">
        <Icon size={16} />
      </div>
      <div>
        <strong>{title}</strong>
        <small>{sub}</small>
      </div>
    </div>
  );
}

/** Keep wide tables usable on narrow screens. */
export function TableWrap({ children }: { children: React.ReactNode }) {
  return <div className="table-scroll">{children}</div>;
}
