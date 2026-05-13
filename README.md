# SSH VPN

Minimal SSH forwarding broker for sharing a local TCP port through a named room.

This project intentionally has no frontend, database, password auth, public key auth, or admin API in the first version. The SSH username is the room name.

## Example

Machine A publishes its local port `8080` into room `roomname`:

```bash
ssh -R 8080:localhost:8080 roomname@serverip -p 2222
```

Machine B opens a local port `8080` through the same room:

```bash
ssh -L 8080:localhost:8080 roomname@serverip -p 2222
```

After both sessions are connected, traffic to `localhost:8080` on machine B is forwarded to `localhost:8080` on machine A.

For forwarding-only sessions, use `-N`:

```bash
ssh -N -R 8080:localhost:8080 roomname@serverip -p 2222
ssh -N -L 8080:localhost:8080 roomname@serverip -p 2222
```

## Behavior

- Room names come from the SSH username, such as `roomname@serverip`.
- Published ports are isolated by room.
- `room-a:8080` and `room-b:8080` can exist at the same time.
- A second publisher for the same room and port is rejected while the first publisher is connected.
- Publishers are removed when their SSH connection closes or sends `cancel-tcpip-forward`.

## Local Development

```powershell
cd backend
go mod tidy
go run ./cmd/server
```

The server listens on SSH port `2222` by default.

## Docker

```powershell
docker compose up -d --build
```

Published ports:

- SSH tunnel broker: `:2222`

Persistent data is mounted from `backend/data` to `/app/data`. The Docker image stores the SSH host key at `/app/data/host_key`.

## Configuration

| Variable | Default | Description |
| --- | --- | --- |
| `SSH_PORT` | `2222` | Port used when `SSH_LISTEN_ADDR` is not set. |
| `SSH_LISTEN_ADDR` | `:2222` | Full TCP listen address for the SSH server. |
| `SSH_HOST_KEY_PATH` | empty | Path to a persisted SSH host key. If empty, an in-memory key is generated on each start. |
| `SSH_SERVER_IDENT` | `SSH-2.0-ssh-vpn` | SSH server identification string. |

## Security Warning

This first version uses `NoClientAuth: true`, so anyone who can reach the SSH port can create or join any room. Do not expose it publicly without network controls such as a firewall, private network, or reverse proxy access policy.

## Verification

Backend:

```powershell
cd backend
go test ./...
```

Docker:

```powershell
docker compose build
```
