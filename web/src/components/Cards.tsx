import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { ChevronRight, Server } from "lucide-react";
import type { Config, GameServer, NodeItem } from "../shared/domain";
import { Status } from "./DataDisplay";

type IconProps = { icon: LucideIcon };
type StatProps = IconProps & {
  label: string;
  value: ReactNode;
  detail: ReactNode;
  tone: string;
};

/** Present a labeled metric with its matching icon and detail text. */
export function Stat({ icon: Icon, label, value, detail, tone }: StatProps) {
  return (
    <div className="stat-card">
      <div className={`stat-icon ${tone}`}>
        <Icon size={19} />
      </div>
      <div className="stat-label">{label}</div>
      <div className="stat-value">{value}</div>
      <div className="stat-detail">{detail}</div>
    </div>
  );
}

type QuickProps = IconProps & {
  title: string;
  text: string;
  onClick: () => void;
};

/** Present a compact navigational action. */
export function Quick({ icon: Icon, title, text, onClick }: QuickProps) {
  return (
    <button className="quick-action" onClick={onClick}>
      <div className="quick-icon">
        <Icon size={17} />
      </div>
      <div>
        <strong>{title}</strong>
        <small>{text}</small>
      </div>
      <ChevronRight size={16} />
    </button>
  );
}

type ServerRowProps = {
  server: Pick<GameServer, "id" | "name" | "status" | "nodeId" | "configId">;
  node?: NodeItem;
  config?: Config;
  onClick: () => void;
};

/** Show a server, its node and config, and its effective running status. */
export function ServerRow({ server, node, config, onClick }: ServerRowProps) {
  return (
    <button className="server-row" onClick={onClick}>
      <div className="entity-icon">
        <Server size={16} />
      </div>
      <div className="server-row-main">
        <strong>{server.name}</strong>
        <small>
          {config?.name || "Config"} · {node?.name || "Node"}
        </small>
      </div>
      <Status status={node?.status === "online" ? server.status : "offline"} />
      <ChevronRight size={15} />
    </button>
  );
}

type EmptyProps = IconProps & {
  title: string;
  text: string;
  action?: ReactNode;
};

/** Explain why a view has no content and optionally offer its next action. */
export function Empty({ icon: Icon, title, text, action }: EmptyProps) {
  return (
    <div className="empty-state">
      <div className="empty-icon">
        <Icon size={22} />
      </div>
      <h2>{title}</h2>
      <p>{text}</p>
      {action}
    </div>
  );
}
