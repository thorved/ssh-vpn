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
	"github.com/thorved/ssh-vpn/backend/internal/sshauth"
	"golang.org/x/crypto/ssh"
)

type Server struct {
	cfg       config.Config
	signer    ssh.Signer
	registry  *Registry
	adminKeys *sshauth.AuthorizedKeys

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

func NewServer(cfg config.Config) (*Server, error) {
	signer, err := loadOrGenerateHostKey(cfg.SSHHostKeyPath)
	if err != nil {
		return nil, err
	}

	adminKeys, err := sshauth.LoadAuthorizedKeys(cfg.AdminAuthorizedKeysFile)
	if err != nil {
		return nil, err
	}

	return &Server{
		cfg:       cfg,
		signer:    signer,
		registry:  NewRegistry(),
		adminKeys: adminKeys,
	}, nil
}

func (s *Server) Run() error {
	serverConfig := &ssh.ServerConfig{
		NoClientAuth:         true,
		NoClientAuthCallback: s.noClientAuth,
		ServerVersion:        s.cfg.SSHServerIdent,
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
	room := strings.TrimSpace(conn.User())
	if room == "" {
		log.Printf("ssh connection from %s missing room name", nc.RemoteAddr())
		return
	}

	if room == s.cfg.AdminUser {
		log.Printf("admin connected from %s", nc.RemoteAddr())
		go rejectGlobalRequests(reqs)
		for newCh := range chans {
			go s.handleAdminChannel(newCh)
		}
		return
	}

	defer func() {
		if removed := s.registry.UnregisterConn(conn); removed > 0 {
			log.Printf("cleaned up %d publisher(s) for room %q", removed, conn.User())
		}
	}()
	s.registry.RegisterConn(room, conn, nc.RemoteAddr())
	log.Printf("room %q connected from %s", room, nc.RemoteAddr())
	go s.handleGlobalRequests(room, conn, reqs)

	for newCh := range chans {
		go s.handleChannel(room, conn, newCh)
	}
}

func (s *Server) handleGlobalRequests(room string, conn *ssh.ServerConn, reqs <-chan *ssh.Request) {
	for req := range reqs {
		switch req.Type {
		case "tcpip-forward":
			s.handleTCPIPForward(room, conn, req)
		case "cancel-tcpip-forward":
			s.handleCancelTCPIPForward(room, conn, req)
		default:
			replyRequest(req, false, nil)
		}
	}
}

func (s *Server) handleTCPIPForward(room string, conn *ssh.ServerConn, req *ssh.Request) {
	var payload tcpipForwardPayload
	if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
		log.Printf("invalid tcpip-forward request for room %q: %v", room, err)
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
		replyRequest(req, false, nil)
		if errors.Is(err, ErrPublisherExists) {
			_ = conn.Close()
			return
		}
		return
	}

	log.Printf("registered publisher room=%q bind=%s:%d", room, payload.BindHost, payload.BindPort)
	replyRequest(req, true, nil)
}

func (s *Server) handleCancelTCPIPForward(room string, conn *ssh.ServerConn, req *ssh.Request) {
	var payload tcpipForwardPayload
	if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
		log.Printf("invalid cancel-tcpip-forward request for room %q: %v", room, err)
		replyRequest(req, false, nil)
		return
	}

	removed := s.registry.Unregister(room, payload.BindPort, conn)
	if removed {
		log.Printf("unregistered publisher room=%q bind=%s:%d", room, payload.BindHost, payload.BindPort)
	}
	replyRequest(req, true, nil)
}

func (s *Server) handleChannel(room string, conn *ssh.ServerConn, newCh ssh.NewChannel) {
	switch newCh.ChannelType() {
	case "direct-tcpip":
		if err := s.handleDirectTCPIP(room, conn, newCh); err != nil {
			log.Printf("direct-tcpip error for room %q: %v", room, err)
		}
	case "session":
		s.handleUserSession(room, conn, newCh)
	default:
		_ = newCh.Reject(ssh.UnknownChannelType, "unsupported channel type")
	}
}

func (s *Server) handleDirectTCPIP(room string, receiver *ssh.ServerConn, newCh ssh.NewChannel) error {
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

	doneForward := s.registry.BeginForward(receiver, publisher, localCh, remoteCh)
	defer doneForward()
	bridgeChannels(localCh, localReqs, remoteCh, remoteReqs)
	return nil
}

func rejectGlobalRequests(reqs <-chan *ssh.Request) {
	for req := range reqs {
		replyRequest(req, false, nil)
	}
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
