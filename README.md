# SSH VPN

SSH forwarding broker for sharing local TCP ports through named rooms, with an admin dashboard served through the SSH tunnel itself.

## Tunnel Usage

Machine A publishes its local port `8080` into room `roomname`:

```bash
ssh -N -R 8080:localhost:8080 roomname@serverip -p 2222
```

Machine B opens a local port `8080` through the same room:

```bash
ssh -N -L 8080:localhost:8080 roomname@serverip -p 2222
```

After both sessions are connected, traffic to `localhost:8080` on machine B is forwarded to `localhost:8080` on machine A.

## Admin Dashboard

The dashboard is available only through the configured admin SSH user. It does not expose a separate HTTP port.

```bash
ssh -N -L 8080:localhost:8080 root@serverip -p 2222
```

Then open:

```text
http://localhost:8080
```

The dashboard shows room totals, connected SSH sessions, published forwards, active forwarded channels, and copyable SSH commands. Removing a room closes all active SSH connections in that room and removes its publishers. The admin room cannot be deleted.

## Admin Keys

Create an authorized keys file for admin users:

```text
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA... admin-a
ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ... admin-b
```

Multiple public keys are allowed. Normal room users can still connect without keys; only `ADMIN_USER` requires one of these keys.

## Configuration

Example `.env`:

```env
SSH_PORT=2222
SSH_HOST_KEY_PATH=backend/data/host_key
PUBLIC_DOMAIN=serverip
PUBLIC_SSH_PORT=2222
ADMIN_USER=root
ADMIN_DASHBOARD_PORT=8080
ADMIN_AUTHORIZED_KEYS_FILE=backend/admin_authorized_keys
WEB_STATIC_DIR=frontend/out
```

| Variable | Default | Description |
| --- | --- | --- |
| `SSH_PORT` | `2222` | Port used when `SSH_LISTEN_ADDR` is not set. |
| `SSH_LISTEN_ADDR` | `:2222` | Full TCP listen address for the SSH server. |
| `SSH_HOST_KEY_PATH` | empty | Path to a persisted SSH host key. If empty, an in-memory key is generated on each start. |
| `SSH_SERVER_IDENT` | `SSH-2.0-ssh-vpn` | SSH server identification string. |
| `PUBLIC_DOMAIN` | `localhost` | Hostname shown in generated SSH commands. |
| `PUBLIC_SSH_PORT` | `SSH_PORT` or `2222` | SSH port shown in generated SSH commands. |
| `ADMIN_USER` | `root` | SSH username allowed to access the dashboard. |
| `ADMIN_DASHBOARD_PORT` | `8080` | Reserved target port for the admin dashboard inside the admin SSH room. |
| `ADMIN_AUTHORIZED_KEYS_FILE` | empty | Authorized public keys file for dashboard access. If empty, admin login is rejected. |
| `WEB_STATIC_DIR` | `frontend/out` | Static Next export directory served by Go. Docker sets this to `/app/web`. |

## Local Development

Backend:

```powershell
cd backend
go mod tidy
go run ./cmd/server
```

Frontend static export:

```powershell
cd frontend
bun install
bun run build
```

Air:

```powershell
air
```

## Docker

```powershell
docker compose up -d --build
```

Published ports:

- SSH tunnel broker and dashboard-over-SSH: `:2222`

Persistent data is mounted from `backend/data` to `/app/data`. The Docker image stores the SSH host key at `/app/data/host_key` and serves the built dashboard from `/app/web`.

## Behavior

- Room names come from the SSH username, such as `roomname@serverip`.
- Published ports are isolated by room.
- `room-a:8080` and `room-b:8080` can exist at the same time.
- `ADMIN_USER:ADMIN_DASHBOARD_PORT` is reserved for the dashboard.
- A second publisher for the same room and port is rejected while the first publisher is connected.
- Publishers are removed when their SSH connection closes or sends `cancel-tcpip-forward`.

## Verification

Backend:

```powershell
cd backend
go test ./...
```

Frontend:

```powershell
cd frontend
bun run lint
bun run build
```

Docker:

```powershell
docker compose build
```
