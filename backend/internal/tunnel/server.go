package tunnel

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/thorved/ssh-vpn/backend/internal/config"
	"golang.org/x/crypto/ssh"
)

type Server struct {
	cfg      config.Config
	signer   ssh.Signer
	registry *Registry

	mu       sync.Mutex
	listener net.Listener
}

type tcpipForwardPayload struct {
	BindHost string
	BindPort uint32
}

type directTCPIPPayload struct {
	Host           string
	Port           uint32
	OriginatorIP   string
	OriginatorPort uint32
}

type forwardedTCPIPPayload struct {
	ConnectedAddress string
	ConnectedPort    uint32
	OriginatorIP     string
	OriginatorPort   uint32
}

type connState struct {
	mu          sync.Mutex
	session     ssh.Channel
	pending     []string
	statusShown bool
}

func NewServer(cfg config.Config) (*Server, error) {
	signer, err := loadOrGenerateHostKey(cfg.SSHHostKeyPath)
	if err != nil {
		return nil, err
	}

	return &Server{
		cfg:      cfg,
		signer:   signer,
		registry: NewRegistry(),
	}, nil
}

func (s *Server) Run() error {
	serverConfig := &ssh.ServerConfig{
		NoClientAuth:  true,
		ServerVersion: s.cfg.SSHServerIdent,
	}
	serverConfig.AddHostKey(s.signer)

	ln, err := net.Listen("tcp", s.cfg.SSHListenAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.cfg.SSHListenAddr, err)
	}

	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	log.Printf("ssh vpn listening on %s", s.cfg.SSHListenAddr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go s.handleConn(conn, serverConfig)
	}
}

func (s *Server) Shutdown() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}

func (s *Server) handleConn(nc net.Conn, serverConfig *ssh.ServerConfig) {
	defer nc.Close()

	conn, chans, reqs, err := ssh.NewServerConn(nc, serverConfig)
	if err != nil {
		log.Printf("ssh handshake error from %s: %v", nc.RemoteAddr(), err)
		return
	}
	defer conn.Close()
	defer func() {
		if removed := s.registry.UnregisterConn(conn); removed > 0 {
			log.Printf("cleaned up %d publisher(s) for room %q", removed, conn.User())
		}
	}()

	room := strings.TrimSpace(conn.User())
	if room == "" {
		log.Printf("ssh connection from %s missing room name", nc.RemoteAddr())
		return
	}

	state := &connState{}
	log.Printf("room %q connected from %s", room, nc.RemoteAddr())
	go s.handleGlobalRequests(room, conn, state, reqs)

	for newCh := range chans {
		go s.handleChannel(room, state, newCh)
	}
}

func (s *Server) handleGlobalRequests(room string, conn *ssh.ServerConn, state *connState, reqs <-chan *ssh.Request) {
	for req := range reqs {
		switch req.Type {
		case "tcpip-forward":
			s.handleTCPIPForward(room, conn, state, req)
		case "cancel-tcpip-forward":
			s.handleCancelTCPIPForward(room, conn, state, req)
		default:
			replyRequest(req, false, nil)
		}
	}
}

func (s *Server) handleTCPIPForward(room string, conn *ssh.ServerConn, state *connState, req *ssh.Request) {
	var payload tcpipForwardPayload
	if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
		log.Printf("invalid tcpip-forward request for room %q: %v", room, err)
		state.notify("remote forward rejected: invalid request for room %q", room)
		replyRequest(req, false, nil)
		return
	}

	err := s.registry.Register(&Publisher{
		Room:     room,
		BindHost: payload.BindHost,
		Port:     payload.BindPort,
		Conn:     conn,
	})
	if err != nil {
		log.Printf("tcpip-forward rejected for room %q port %d: %v", room, payload.BindPort, err)
		state.notify("remote forward rejected: room %q port %d is already published", room, payload.BindPort)
		replyRequest(req, false, nil)
		return
	}

	log.Printf("registered publisher room=%q bind=%s:%d", room, payload.BindHost, payload.BindPort)
	state.notify("remote forward ready: room %q port %d", room, payload.BindPort)
	replyRequest(req, true, nil)
}

func (s *Server) handleCancelTCPIPForward(room string, conn *ssh.ServerConn, state *connState, req *ssh.Request) {
	var payload tcpipForwardPayload
	if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
		log.Printf("invalid cancel-tcpip-forward request for room %q: %v", room, err)
		state.notify("remote forward cancel failed: invalid request for room %q", room)
		replyRequest(req, false, nil)
		return
	}

	removed := s.registry.Unregister(room, payload.BindPort, conn)
	if removed {
		log.Printf("unregistered publisher room=%q bind=%s:%d", room, payload.BindHost, payload.BindPort)
		state.notify("remote forward closed: room %q port %d", room, payload.BindPort)
	}
	replyRequest(req, true, nil)
}

func (s *Server) handleChannel(room string, state *connState, newCh ssh.NewChannel) {
	switch newCh.ChannelType() {
	case "direct-tcpip":
		if err := s.handleDirectTCPIP(room, newCh); err != nil {
			log.Printf("direct-tcpip error for room %q: %v", room, err)
		}
	case "session":
		s.handleSession(state, newCh)
	default:
		_ = newCh.Reject(ssh.UnknownChannelType, "unsupported channel type")
	}
}

func (s *Server) handleDirectTCPIP(room string, newCh ssh.NewChannel) error {
	var payload directTCPIPPayload
	if err := ssh.Unmarshal(newCh.ExtraData(), &payload); err != nil {
		_ = newCh.Reject(ssh.ConnectionFailed, "invalid direct-tcpip payload")
		return err
	}

	publisher, err := s.registry.Lookup(room, payload.Port)
	if err != nil {
		_ = newCh.Reject(ssh.ConnectionFailed, err.Error())
		return err
	}

	forwardPayload := forwardedTCPIPPayload{
		ConnectedAddress: publisher.BindHost,
		ConnectedPort:    publisher.Port,
		OriginatorIP:     payload.OriginatorIP,
		OriginatorPort:   payload.OriginatorPort,
	}
	if forwardPayload.ConnectedAddress == "" {
		forwardPayload.ConnectedAddress = payload.Host
	}

	remoteCh, remoteReqs, err := publisher.Conn.OpenChannel("forwarded-tcpip", ssh.Marshal(forwardPayload))
	if err != nil {
		_ = newCh.Reject(ssh.ConnectionFailed, err.Error())
		return fmt.Errorf("open publisher channel: %w", err)
	}

	localCh, localReqs, err := newCh.Accept()
	if err != nil {
		_ = remoteCh.Close()
		return err
	}

	bridgeChannels(localCh, localReqs, remoteCh, remoteReqs)
	return nil
}

func (s *Server) handleSession(state *connState, newCh ssh.NewChannel) {
	ch, reqs, err := newCh.Accept()
	if err != nil {
		return
	}
	state.attachSession(ch)

	go func() {
		defer state.detachSession(ch)
		defer ch.Close()
		for req := range reqs {
			switch req.Type {
			case "pty-req", "shell", "exec":
				replyRequest(req, true, nil)
				state.showStatus()
			default:
				replyRequest(req, false, nil)
			}
		}
	}()
}

func (s *connState) notify(format string, args ...any) {
	message := fmt.Sprintf(format, args...) + "\n"

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session == nil {
		s.pending = append(s.pending, message)
		return
	}
	_, _ = io.WriteString(s.session, message)
}

func (s *connState) attachSession(ch ssh.Channel) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.session = ch
	for _, message := range s.pending {
		_, _ = io.WriteString(ch, message)
	}
	s.pending = nil
}

func (s *connState) detachSession(ch ssh.Channel) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session == ch {
		s.session = nil
	}
}

func (s *connState) showStatus() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.statusShown || s.session == nil {
		return
	}
	s.statusShown = true
	_, _ = io.WriteString(s.session, "ssh-vpn tunnel connected. Use -N for forwarding-only sessions.\n")
}

func bridgeChannels(a ssh.Channel, aReqs <-chan *ssh.Request, b ssh.Channel, bReqs <-chan *ssh.Request) {
	defer a.Close()
	defer b.Close()

	go forwardChannelRequests(aReqs, b)
	go forwardChannelRequests(bReqs, a)

	done := make(chan struct{}, 2)
	go copyChannel(a, b, done)
	go copyChannel(b, a, done)
	<-done
	<-done
}

func copyChannel(dst ssh.Channel, src ssh.Channel, done chan<- struct{}) {
	_, _ = io.Copy(dst, src)
	_ = dst.CloseWrite()
	done <- struct{}{}
}

func forwardChannelRequests(in <-chan *ssh.Request, out ssh.Channel) {
	for req := range in {
		ok, err := out.SendRequest(req.Type, req.WantReply, req.Payload)
		if err != nil {
			log.Printf("channel request forward error: %v", err)
		}
		replyRequest(req, ok && err == nil, nil)
	}
}

func replyRequest(req *ssh.Request, ok bool, payload []byte) {
	if req.WantReply {
		_ = req.Reply(ok, payload)
	}
}

func loadOrGenerateHostKey(path string) (ssh.Signer, error) {
	if strings.TrimSpace(path) == "" {
		_, privateKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		return ssh.NewSignerFromKey(privateKey)
	}

	keyBytes, err := os.ReadFile(path)
	if err == nil {
		return ssh.ParsePrivateKey(keyBytes)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		return nil, err
	}

	return ssh.NewSignerFromKey(privateKey)
}
