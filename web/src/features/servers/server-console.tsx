import { Button } from "../../components/Button";
import { FormEvent, useEffect, useRef, useState } from "react";
import { Terminal } from "lucide-react";
import type { GameServer, NodeItem } from "../../shared/domain";
import { showToast } from "../../shared/toast";

/** Connect to a server's live output and provide its interactive command input. */
export function Console({
  server,
  node,
  deploying = false,
}: {
  server: GameServer;
  node?: NodeItem;
  deploying?: boolean;
}) {
  const [status, setStatus] = useState("connecting");
  const [lines, setLines] = useState<string[]>([]);
  const [currentLine, setCurrentLine] = useState("");
  const [command, setCommand] = useState("");
  const [socket, setSocket] = useState<WebSocket | null>(null);
  const out = useRef<HTMLDivElement>(null);
  const nodeOfflineNotified = useRef(false);
  const terminal = useRef({ mode: "normal", line: "", cursor: 0, csi: "" });
  useEffect(() => {
    if (deploying) {
      setStatus("deploying");
      setLines([]);
      setCurrentLine("");
      return;
    }
    if (server.status === "installing") {
      setStatus("installing");
      setLines([]);
      setCurrentLine("");
      return;
    }
    if (server.status !== "running") {
      setStatus(server.status || "stopped");
      setLines([]);
      setCurrentLine("");
      return;
    }
    if (!server.containerId) {
      setStatus("not deployed");
      setLines([
        "Server has no container ID. Deploy the server to attach its console.",
      ]);
      return;
    }
    if (node?.status !== "online") {
      setStatus("node offline");
      if (!nodeOfflineNotified.current) {
        showToast(
          "Node is offline. Start the node agent to connect to the console.",
        );
        nodeOfflineNotified.current = true;
      }
      return;
    }
    nodeOfflineNotified.current = false;
    const scheme = location.protocol === "https:" ? "wss:" : "ws:";
    let live = true;
    let retry: number | undefined;
    let ws: WebSocket;
    const connect = () => {
      if (!live) return;
      ws = new WebSocket(
        scheme +
          "//" +
          location.host +
          "/api/servers/" +
          server.id +
          "/console",
      );
      setSocket(ws);
      ws.onopen = () => setStatus("attaching");
      ws.onmessage = (e) => {
        if (!live) return;
        try {
          const m = JSON.parse(e.data);
          if (m.type === "console_attached") setStatus("connected");
          if (m.type === "console_error") {
            setStatus("attach failed");
            ws.close();
          }
          const text = m.payload?.text || m.text || "";
          if (m.type === "console_status" && text) {
            const statusLines = String(text).split(/\r?\n/).filter(Boolean);
            if (statusLines.length) {
              setLines((old) => [...old, ...statusLines].slice(-1000));
            }
            terminal.current.line = "";
            terminal.current.cursor = 0;
            setCurrentLine("");
          } else if (text) {
            const parsed = decodeConsoleText(String(text), terminal.current);
            if (parsed.lines.length) {
              setLines((old) => [...old, ...parsed.lines].slice(-1000));
            }
            setCurrentLine(parsed.current);
          }
        } catch {
          /* ignore malformed console frames */
        }
      };
      ws.onerror = () => {
        if (live) setStatus("connection error");
      };
      ws.onclose = () => {
        if (!live) return;
        setStatus("disconnected");
        retry = window.setTimeout(connect, 1500);
      };
    };
    connect();
    return () => {
      live = false;
      if (retry) window.clearTimeout(retry);
      ws?.close();
      setSocket(null);
    };
  }, [server.id, server.containerId, server.status, node?.status, deploying]);
  useEffect(() => {
    if (out.current) out.current.scrollTop = out.current.scrollHeight;
  }, [lines, currentLine]);
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (
      deploying ||
      server.status !== "running" ||
      !command.trim() ||
      socket?.readyState !== WebSocket.OPEN
    )
      return;
    socket.send(JSON.stringify({ command }));
    setCommand("");
  };
  const visibleCurrent = /^>[.· ]*$/.test(currentLine.trim())
    ? ""
    : currentLine;
  return (
    <section className="console-card">
      <div className="console-top">
        <div className="console-title">
          <Terminal size={17} />
          <div>
            <strong>Live console</strong>
            <small>Game output and command input</small>
          </div>
        </div>
        <div className="console-state">
          <span
            className={"status-dot " + (status === "connected" ? "online" : "")}
          />
          {status}
        </div>
      </div>
      <div className="console-output" ref={out}>
        {deploying || server.status === "installing" ? (
          <div className="console-placeholder console-installing">
            <span className="spinner" aria-hidden="true" />
            {deploying
              ? "Deploying server. Console output will appear when it starts."
              : "Installing server. Console output will appear when it starts."}
          </div>
        ) : server.status === "error" && server.error ? (
          <>
            <div className="console-line console-error-line">
              Server installation failed:
            </div>
            {server.error.split(/\r?\n/).map((line, i) => (
              <div className="console-line console-error-line" key={i}>
                {line || " "}
              </div>
            ))}
          </>
        ) : lines.length || visibleCurrent ? (
          <>
            {lines.map((line, i) => (
              <div className="console-line" key={i}>
                {line}
              </div>
            ))}
            {visibleCurrent && (
              <div className="console-line">{visibleCurrent}</div>
            )}
          </>
        ) : (
          <div className="console-placeholder">
            {status === "node offline"
              ? "Console unavailable while the node is offline."
              : "Waiting for console output…"}
          </div>
        )}
      </div>
      <form className="console-input" onSubmit={submit}>
        <span>$</span>
        <input
          value={command}
          onChange={(e) => setCommand(e.target.value)}
          disabled={
            deploying ||
            status !== "connected" ||
            server.status !== "running"
          }
          placeholder={
            status === "connected" ? "Type a command…" : "Console disconnected"
          }
        />
        <Button
          variant="primary"

          disabled={
            deploying ||
            status !== "connected" ||
            server.status !== "running" ||
            !command.trim()
          }
        >
          Send
        </Button>
      </form>
    </section>
  );
}
/** Decode console output while preserving line boundaries and control text. */
export function decodeConsoleText(
  input: string,
  state: { mode: string; line: string; cursor: number; csi: string },
) {
  const lines: string[] = [];
  for (let i = 0; i < input.length; i++) {
    const ch = input[i],
      code = ch.charCodeAt(0);
    if (state.mode === "osc") {
      if (ch === "\u0007") state.mode = "normal";
      else if (ch === "\u001b") state.mode = "osc-esc";
      continue;
    }
    if (state.mode === "osc-esc") {
      state.mode = ch === "\\" ? "normal" : "osc";
      continue;
    }
    if (state.mode === "csi") {
      if (code >= 0x40 && code <= 0x7e) {
        if (ch === "K") {
          const mode = state.csi.split(";").at(-1) || "0";
          if (mode === "2") state.line = "";
          else if (mode === "1") state.line = state.line.slice(state.cursor);
        }
        state.csi = "";
        state.mode = "normal";
      } else state.csi += ch;
      continue;
    }
    if (state.mode === "esc") {
      if (ch === "]") state.mode = "osc";
      else if (ch === "[") {
        state.mode = "csi";
        state.csi = "";
      } else if (ch === "(" || ch === ")") state.mode = "esc-char";
      else state.mode = "normal";
      continue;
    }
    if (state.mode === "esc-char") {
      state.mode = "normal";
      continue;
    }
    if (ch === "\u001b") {
      state.mode = "esc";
      continue;
    }
    if (ch === "\r") {
      state.cursor = 0;
      continue;
    }
    if (ch === "\n") {
      if (!/^>[.· ]*$/.test(state.line.trim())) lines.push(state.line);
      state.line = "";
      state.cursor = 0;
      continue;
    }
    if (ch === "\b") {
      state.cursor = Math.max(0, state.cursor - 1);
      continue;
    }
    if (code < 32 || code === 127) continue;
    if (state.cursor < state.line.length) {
      state.line =
        state.line.slice(0, state.cursor) +
        ch +
        state.line.slice(state.cursor + 1);
    } else state.line += ch;
    state.cursor++;
  }
  return { lines, current: state.line };
}
