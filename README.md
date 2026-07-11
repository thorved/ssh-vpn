# SSH VPN

A small SSH forwarding broker for sharing TCP ports through named rooms. Operators manage live rooms, connections, and forwards through a native terminal dashboard served directly over SSH—there is no frontend, HTTP server, database, or additional public port.

## Tunnel usage

Machine A publishes local port `8080` into `roomname`:

```bash
ssh -N -R 8080:localhost:8080 roomname@serverip -p 2222
```

Machine B connects its local port `8080` to the same room:

```bash
ssh -N -L 8080:localhost:8080 roomname@serverip -p 2222
```

Traffic sent to `localhost:8080` on Machine B is forwarded to Machine A. Room users remain authentication-free; use network controls when exposing the service.

## Admin dashboard

The configured admin username requires an authorized public key and opens the dashboard immediately:

```bash
ssh -t -i ~/.ssh/id_ed25519 root@serverip -p 2222
```

The dashboard includes:

- Live totals for rooms, connections, published forwards, and active channels.
- Searchable room, connection, and forward inventories with detailed ownership and activity.
- Confirmed actions to remove a room, disconnect a connection, or remove a forward.
- Event-driven updates with a periodic refresh fallback.
- Mouse-enabled tabs, row selection, scrolling, toolbar actions, and confirmation buttons in compatible terminals.

Use `Tab` or `1`–`4` to change views, arrows or `j`/`k` to move, `/` to search, `d` to remove the selected item, `r` to refresh, `?` for help, and `q` to exit. Admin sessions are control-plane only: they never appear as rooms and cannot publish or consume forwards.

Removing a forward disconnects its owning SSH connection, ending its active traffic and every other forward owned by that connection. Each destructive action shows its impact before asking for confirmation.

## Admin keys

Create `data/admin_authorized_keys` and add one or more OpenSSH public keys. A safe template is tracked at `data/admin_authorized_keys.example`:

```text
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA... operator-a
ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQ... operator-b
```

Comments and blank lines are supported. If `ADMIN_AUTHORIZED_KEYS_FILE` is empty, the tunnel server still starts but admin login is disabled. If a non-empty path is unreadable or contains an invalid entry, startup fails so a broken security configuration is visible immediately.

## Configuration

Copy `.env.example` to `.env` and adjust it:

| Variable | Default | Description |
| --- | --- | --- |
| `SSH_PORT` | `2222` | Listener port when `SSH_LISTEN_ADDR` is unset. |
| `SSH_LISTEN_ADDR` | `:2222` | Full SSH listen address. |
| `SSH_HOST_KEY_PATH` | empty | Persisted server host key. An empty value generates an in-memory key on each start. |
| `SSH_SERVER_IDENT` | `SSH-2.0-ssh-vpn` | SSH server identification string. |
| `ADMIN_USER` | `root` | Username reserved for the terminal dashboard. |
| `ADMIN_AUTHORIZED_KEYS_FILE` | empty | OpenSSH authorized-keys file required by the admin user. |

Normal usernames continue to identify rooms and do not require keys. The configured admin username is reserved and cannot be used as a tunnel room.

## Local development

From the repository root:

```powershell
Copy-Item .env.example .env
Copy-Item data/admin_authorized_keys.example data/admin_authorized_keys
go -C backend build -o server ./cmd/server
./backend/server
```

Replace the placeholder key before starting. The process loads `.env` from its working directory, or `backend/.env` when run from `backend`.

## Docker

Place real admin public keys in `data/admin_authorized_keys`, then run:

```powershell
docker compose up -d --build
```

Compose exposes only SSH on port `2222`. The repository-root `data` directory is mounted at `/app/data` for the generated host key and admin authorized-keys file. Real files under `data/` are ignored by Git; only the safe `.example` template is tracked.

## Behavior and security

- Room names come from SSH usernames.
- Published ports are isolated by room, so different rooms can use the same port.
- A second publisher for the same room and port is rejected while the first is connected.
- A connection attempting to publish an already-owned room/port is rejected and disconnected; the existing publisher stays online.
- Publishers disappear when their SSH connection closes or cancels the remote forward.
- Admin authentication protects only the reserved admin username; ordinary rooms intentionally use `NoClientAuth`.
- Do not expose the broker publicly without a firewall, private network, or equivalent access policy.

## Verification

```powershell
go -C backend test ./...
go -C backend test -race ./...
go -C backend vet ./...
go -C backend build ./cmd/server
docker compose build
```
