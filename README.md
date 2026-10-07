# Control Plane

Control Plane is a self-hosted panel for deploying and managing game servers on Docker nodes. It includes a Go API and node agent, a React web app, and MongoDB persistence.

## Features

- Create nodes, servers, and persistent volumes from the web panel.
- Manage container lifecycle, console input/output, files, backups, and resource usage.
- Define reusable server Configs with Docker images, variables, ports, resource limits, install scripts, and startup commands.
- Manage users and roles with permissions, plus an audit log.
- Require TOTP for the initial root account, with recovery codes for sign-in.
- Configure HTTPS through Caddy or terminate TLS directly in the panel.

## Run locally

Requirements: Go 1.26+, Node.js 20+, npm, and MongoDB (`mongod` and `mongosh`). From the project root:

```bash
./scripts/dev.sh
```

The script starts the panel and Vite. If MongoDB is not already reachable at `127.0.0.1:27018`, it starts a local instance using `data/dev-mongo` and the `control-plane` database. Open the Vite URL printed in the terminal, normally <http://127.0.0.1:5173/>. Press Ctrl+C to stop the dev services.

The first visit prompts you to create the root account and set up an authenticator app. After setup, add a node from **Nodes** and run the node install command shown by the panel on a Linux Docker host. When the node is online, create a server from **Servers**.

You can override the local MongoDB port, URI, database name, or panel address when starting the script:

```bash
MONGO_PORT=27018 MONGO_DB=control-plane PANEL_ADDR=127.0.0.1:8080 ./scripts/dev.sh
```

`MONGO_URI` can point the panel and dev script at an existing MongoDB instance. The dev database directory is disposable local runtime data; it is not part of the release bundles.

## Configs

Configs describe how a server is installed and run. The built-in Configs are Counter-Strike 2, DayZ, Minecraft Java, Paper Minecraft, Rust, and a SteamCMD starter.

Each Config can define runtime Docker images, an optional installer image and script, a startup command, environment variables, ports, data directory, resource limits, and supported CPU architectures. The node runs the install script once in a temporary container against the server's persistent data volume. It then starts the runtime container with the Config's startup command and environment. Configs can also leave an image's own entrypoint and command unchanged.

Built-in definitions are in `configs/<slug>/config.json`. Longer scripts can live beside the definition and are embedded into the Go binary at build time. At startup, built-ins are added to MongoDB only when their slug is missing; edits made in the panel are retained.

The SteamCMD, Rust, DayZ, and CS2 Configs require amd64 nodes. CS2 also needs substantial free storage; its Config recommends about 60 GB.

## Build a release

Install frontend dependencies and build the Linux release bundle:

```bash
./scripts/build-release.sh
```

The default target is Linux amd64. To build for Linux arm64:

```bash
GOARCH=arm64 ./scripts/build-release.sh
```

The bundle contains the panel and node binaries, the built web app, and the installer scripts. Matching install scripts are also copied into `dist/`.

For development checks:

```bash
go test ./...
(cd web && npm ci && npm run build)
```

## Install the panel

The installer supports Debian 12 amd64 and Ubuntu 20.04, 22.04, or 24.04. It installs MongoDB when needed, creates the panel service, and binds it to loopback on `127.0.0.1:8080`.

For local or reverse-proxy access:

```bash
sudo ./dist/install-panel.sh ./dist/control-plane-linux-amd64.tar.gz
```

To configure a public HTTPS hostname during installation, point DNS to the panel host and allow inbound TCP ports 80 and 443, then run:

```bash
sudo ./dist/install-panel.sh ./dist/control-plane-linux-amd64.tar.gz panel.example.com admin@example.com
```

The panel is then available at `https://panel.example.com` without a port in the URL. The database name defaults to `control-plane`. The service reads `/etc/control-plane/panel.env` and logs to the system journal:

```bash
sudo systemctl status control-plane-panel
sudo journalctl -u control-plane-panel -f
```

The panel environment supports `PANEL_ADDR`, `MONGO_URI`, `MONGO_DB`, `SESSION_TTL_HOURS`, and `STATIC_DIR`. For direct TLS termination, set both `PANEL_TLS_CERT_FILE` and `PANEL_TLS_KEY_FILE`.

## Install a node

Install one agent on each Linux Docker host. Create the node in **Nodes** first, then use its one-time token:

```bash
sudo ./dist/install-node.sh ./dist/control-plane-linux-amd64.tar.gz https://panel.example.com NODE_TOKEN node-name
```

Use the arm64 bundle for an arm64 host. The node installer installs Docker Engine and creates the `control-plane-node` systemd service. The agent runs as root to access Docker and manage containers. Keep its token private and do not expose Docker's TCP API.

```bash
sudo systemctl status control-plane-node
sudo journalctl -u control-plane-node -f
```

The node service reads `/etc/control-plane/node.env`, including `PANEL_URL`, `NODE_TOKEN`, and `NODE_NAME`. Node-to-panel WebSocket connections should use the TLS-enabled panel URL in production.

## HTTPS

In **Settings → HTTPS and secure sockets**, save the hostname and certificate contact email to prepare the TLS install command. HTTP remains available until HTTPS enforcement is enabled. Run the provided command on the panel host, open the HTTPS URL to confirm TLS is working, then choose **Require HTTPS**. Caddy manages certificates and proxies node WebSocket traffic.

You can also run the installed helper directly:

```bash
sudo /opt/control-plane/scripts/install-https.sh panel.example.com admin@example.com
```

The helper refreshes Caddy's signing key and apt source before updating packages. If the upstream repository fails signature verification, it can fall back to the host's signed Caddy package. To change the panel's loopback port, update `PANEL_ADDR` in `/etc/control-plane/panel.env` and pass the same address as the helper's third argument.

## Security

- The first account is the protected root account and must enroll TOTP. Save its recovery codes securely.
- Login and sensitive authentication endpoints are rate-limited per client. Login and OTP allow 10 requests per 15 minutes; first-time setup allows 5 requests per 15 minutes. These in-memory limits reset when the panel restarts and are local to each panel instance.
- Role permissions are checked by the API. Only root or users with the relevant permission can manage roles, users, HTTPS enforcement, and other protected actions.
- Use HTTPS for public panel access. Keep MongoDB and Docker bound to trusted interfaces and restrict inbound ports with a firewall.
- The node agent needs root access to Docker. Protect node tokens and do not expose the Docker socket over TCP.

## Containerized panel

The included `docker-compose.yml` starts MongoDB and the panel. Before using it outside local development, replace the example MongoDB password in both service configurations, restrict the panel's public port or put it behind a TLS reverse proxy, and set appropriate firewall rules.

## Project layout

```text
cmd/                 Panel and node entry points
configs/             Built-in Config definitions and scripts
internal/config/     Config validation and defaults
internal/models/     Domain models
internal/node/       Docker node agent
internal/panel/      HTTP controllers, services, and repositories
scripts/             Development, release, and installation scripts
test/                Go tests
web/src/             React application
```
