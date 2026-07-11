package tunnel

import (
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorved/ssh-vpn/backend/internal/userui"
	"golang.org/x/crypto/ssh"
)

func (s *Server) handleUserSession(room string, conn *ssh.ServerConn, newCh ssh.NewChannel) {
	ch, reqs, err := newCh.Accept()
	if err != nil {
		return
	}
	defer ch.Close()

	width, height := 80, 24
	hasPTY, started := false, false
	var program *tea.Program
	cleanup := func() {}
	done := make(chan error, 1)
	store := &userStore{registry: s.registry, conn: conn}
	defer func() {
		if program != nil {
			program.Quit()
		}
		cleanup()
	}()

	for {
		select {
		case err := <-done:
			status := uint32(0)
			if err != nil {
				status = 1
			}
			_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
			if store.shouldDisconnect() {
				_ = conn.Close()
			}
			return
		case req, ok := <-reqs:
			if !ok {
				return
			}
			switch req.Type {
			case "pty-req":
				if started {
					replyRequest(req, false, nil)
					continue
				}
				var payload ptyRequest
				if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
					replyRequest(req, false, nil)
					continue
				}
				width, height = terminalSize(payload.Columns, payload.Rows)
				hasPTY = true
				replyRequest(req, true, nil)
			case "window-change":
				var payload windowChangeRequest
				if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
					replyRequest(req, false, nil)
					continue
				}
				width, height = terminalSize(payload.Columns, payload.Rows)
				if program != nil {
					program.Send(tea.WindowSizeMsg{Width: width, Height: height})
				}
				replyRequest(req, true, nil)
			case "shell":
				if started || !hasPTY {
					replyRequest(req, false, nil)
					if !hasPTY {
						_, _ = io.WriteString(ch, "room dashboard requires a PTY; connect with ssh -t or use -N for forwarding only\r\n")
						return
					}
					continue
				}
				started = true
				replyRequest(req, true, nil)
				program, cleanup = userui.NewProgram(store, ch, ch, width, height)
				go func() { _, err := program.Run(); done <- err }()
			case "exec":
				replyRequest(req, false, nil)
				_, _ = fmt.Fprintf(ch, "commands are disabled for room %q; use ssh -t for the dashboard or -N for forwarding only\r\n", room)
				return
			default:
				replyRequest(req, false, nil)
			}
		}
	}
}
