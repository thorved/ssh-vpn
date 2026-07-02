package tunnel

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

func (s *Server) serveAdminHTTP(newCh ssh.NewChannel) error {
	ch, reqs, err := newCh.Accept()
	if err != nil {
		return err
	}
	go ssh.DiscardRequests(reqs)

	conn := newSSHChannelConn(ch)
	listener := newSingleConnListener(conn)
	server := &http.Server{
		Handler:           s.adminHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	err = server.Serve(listener)
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("serve admin dashboard: %w", err)
	}
	return nil
}

type singleConnListener struct {
	conn net.Conn
	done <-chan struct{}

	mu   sync.Mutex
	used bool
}

func newSingleConnListener(conn *sshChannelConn) *singleConnListener {
	return &singleConnListener{
		conn: conn,
		done: conn.done,
	}
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	if !l.used {
		l.used = true
		conn := l.conn
		l.mu.Unlock()
		return conn, nil
	}
	done := l.done
	l.mu.Unlock()

	<-done
	return nil, net.ErrClosed
}

func (l *singleConnListener) Close() error {
	return l.conn.Close()
}

func (l *singleConnListener) Addr() net.Addr {
	return staticAddr("ssh-admin-listener")
}

type sshChannelConn struct {
	ssh.Channel
	done chan struct{}
	once sync.Once
}

func newSSHChannelConn(ch ssh.Channel) *sshChannelConn {
	return &sshChannelConn{
		Channel: ch,
		done:    make(chan struct{}),
	}
}

func (c *sshChannelConn) Close() error {
	c.once.Do(func() {
		close(c.done)
	})
	return c.Channel.Close()
}

func (c *sshChannelConn) LocalAddr() net.Addr {
	return staticAddr("ssh-admin-local")
}

func (c *sshChannelConn) RemoteAddr() net.Addr {
	return staticAddr("ssh-admin-remote")
}

func (c *sshChannelConn) SetDeadline(time.Time) error {
	return nil
}

func (c *sshChannelConn) SetReadDeadline(time.Time) error {
	return nil
}

func (c *sshChannelConn) SetWriteDeadline(time.Time) error {
	return nil
}

func (c *sshChannelConn) Read(p []byte) (int, error) {
	n, err := c.Channel.Read(p)
	if errors.Is(err, io.EOF) {
		c.once.Do(func() {
			close(c.done)
		})
	}
	return n, err
}

type staticAddr string

func (a staticAddr) Network() string {
	return "ssh"
}

func (a staticAddr) String() string {
	return string(a)
}
