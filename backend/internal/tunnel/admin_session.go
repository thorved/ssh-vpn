package tunnel

import (
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thorved/ssh-vpn/backend/internal/adminui"
	"golang.org/x/crypto/ssh"
)

type ptyRequest struct {
	Term            string
	Columns         uint32
	Rows            uint32
	WidthPixels     uint32
	HeightPixels    uint32
	EncodedTerminal string
}

type windowChangeRequest struct {
	Columns      uint32
	Rows         uint32
	WidthPixels  uint32
	HeightPixels uint32
}

func (s *Server) handleAdminChannel(newCh ssh.NewChannel) {
	if newCh.ChannelType() != "session" {
		_ = newCh.Reject(ssh.Prohibited, "admin only supports interactive sessions")
		return
	}

	ch, reqs, err := newCh.Accept()
	if err != nil {
		return
	}
	defer ch.Close()

	width, height := 80, 24
	hasPTY := false
	started := false
	var program *tea.Program
	cleanup := func() {}
	done := make(chan error, 1)
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
						_, _ = io.WriteString(ch, "admin dashboard requires a PTY; connect with ssh -t\r\n")
						return
					}
					continue
				}
				started = true
				replyRequest(req, true, nil)
				program, cleanup = adminui.NewProgram(s.registry, ch, ch, width, height)
				go func() {
					_, err := program.Run()
					done <- err
				}()
			case "exec":
				replyRequest(req, false, nil)
				_, _ = fmt.Fprintln(ch, "admin exec commands are disabled; connect with ssh -t to open the dashboard")
				return
			default:
				replyRequest(req, false, nil)
			}
		}
	}
}

func terminalSize(columns, rows uint32) (int, int) {
	width, height := int(columns), int(rows)
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	return width, height
}
