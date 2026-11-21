package gateway

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Router interface{ ExitName(host string) string }

// Server accepts clear HTTP redirects and TLS passthrough on separate ports.
type Server struct {
	Router       Router
	HTTPAddr     string
	TLSAddr      string
	IdleTimeout  time.Duration
	http         *http.Server
	listener     net.Listener
	httpListener net.Listener
	mu           sync.Mutex
	active       map[net.Conn]net.Conn
	closed       bool
	wg           sync.WaitGroup
	slots        chan struct{}
}

func (s *Server) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.TLSAddr)
	if err != nil {
		return err
	}
	httpListener, err := net.Listen("tcp", s.HTTPAddr)
	if err != nil {
		listener.Close()
		return err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		listener.Close()
		httpListener.Close()
		return net.ErrClosed
	}
	s.listener = listener
	s.httpListener = httpListener
	s.active = make(map[net.Conn]net.Conn)
	s.http = &http.Server{Addr: s.HTTPAddr, ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(s.redirect)}
	s.mu.Unlock()
	s.slots = make(chan struct{}, 2048)
	go func() {
		if err := s.http.Serve(httpListener); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP ingress failed: %v", err)
		}
	}()
	go func() { <-ctx.Done(); s.Close() }()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			select {
			case s.slots <- struct{}{}:
				s.mu.Lock()
				if s.closed {
					s.mu.Unlock()
					<-s.slots
					conn.Close()
					return
				}
				s.active[conn] = nil
				s.wg.Add(1)
				s.mu.Unlock()
				go func() {
					defer func() {
						s.mu.Lock()
						delete(s.active, conn)
						s.mu.Unlock()
						<-s.slots
						s.wg.Done()
					}()
					s.handleTLS(conn)
				}()
			default:
				conn.Close()
			}
		}
	}()
	return nil
}

func (s *Server) Close() {
	s.mu.Lock()
	s.closed = true
	listener, httpListener, httpServer := s.listener, s.httpListener, s.http
	s.listener, s.httpListener = nil, nil
	connections := make([]net.Conn, 0, len(s.active)*2)
	for in, out := range s.active {
		connections = append(connections, in)
		if out != nil {
			connections = append(connections, out)
		}
	}
	s.mu.Unlock()
	if listener != nil {
		listener.Close()
	}
	if httpListener != nil {
		httpListener.Close()
	}
	for _, conn := range connections {
		conn.Close()
	}
	if httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpServer.Shutdown(ctx)
	}
	s.wg.Wait()
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(r.Host)
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	if s.Router.ExitName(host) == "" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "https://"+host+r.URL.RequestURI(), http.StatusPermanentRedirect)
}

func (s *Server) handleTLS(in net.Conn) {
	defer in.Close()
	host, preface, err := ReadServerName(in)
	if err != nil {
		return
	}
	exit := s.Router.ExitName(host)
	if exit == "" {
		return
	}
	out, err := DialSOCKS("singbox-"+exit+":1080", host, 443)
	if err != nil {
		log.Printf("exit %s dial failed: %v", exit, err)
		return
	}
	defer out.Close()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.active[in] = out
	s.mu.Unlock()
	timeout := s.IdleTimeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	in, out, stopIdle := watchIdle(in, out, timeout)
	defer stopIdle()
	if _, err := io.Copy(out, bytes.NewReader(preface)); err != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		io.Copy(out, in)
		if half, ok := out.(interface{ CloseWrite() error }); ok {
			half.CloseWrite()
		}
		close(done)
	}()
	io.Copy(in, out)
	if half, ok := in.(interface{ CloseWrite() error }); ok {
		half.CloseWrite()
	}
	in.Close()
	<-done
}

// watchIdle closes both sides when neither direction transfers data.
func watchIdle(in, out net.Conn, timeout time.Duration) (net.Conn, net.Conn, func()) {
	started := time.Now()
	var last atomic.Int64
	touch := func() {
		elapsed := time.Since(started).Nanoseconds()
		for {
			previous := last.Load()
			if elapsed <= previous || last.CompareAndSwap(previous, elapsed) {
				return
			}
		}
	}
	done := make(chan struct{})
	go func() {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		for {
			select {
			case <-done:
				return
			case <-timer.C:
				remaining := timeout - (time.Since(started) - time.Duration(last.Load()))
				if remaining <= 0 {
					in.Close()
					out.Close()
					return
				}
				timer.Reset(remaining)
			}
		}
	}()
	return &activityConn{Conn: in, touch: touch}, &activityConn{Conn: out, touch: touch}, func() { close(done) }
}

type activityConn struct {
	net.Conn
	touch func()
}

func (c *activityConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.touch()
	}
	return n, err
}

func (c *activityConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.touch()
	}
	return n, err
}

func (c *activityConn) CloseWrite() error {
	if conn, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return conn.CloseWrite()
	}
	return nil
}
