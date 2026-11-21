package gateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type Router interface{ ExitName(host string) string }

// DefaultMaxConnections is the admission limit when no custom limit is set.
const DefaultMaxConnections = 2048

// TLSStats counts admitted connections and rejections by the TLS admission limit.
type TLSStats struct {
	Active   int    `json:"active"`
	Limit    int    `json:"limit"`
	Rejected uint64 `json:"rejected"`
}

// Server accepts clear HTTP redirects and TLS passthrough on separate ports.
type Server struct {
	Router         Router
	HTTPAddr       string
	TLSAddr        string
	IdleTimeout    time.Duration
	MaxConnections int // Zero selects DefaultMaxConnections.

	http         *http.Server
	listener     net.Listener
	httpListener net.Listener
	cancelAccept context.CancelFunc
	mu           sync.Mutex
	active       map[net.Conn]net.Conn
	closed       bool
	limit        int
	rejected     uint64
	wg           sync.WaitGroup
}

func (s *Server) connectionLimit() int {
	if s.MaxConnections == 0 {
		return DefaultMaxConnections
	}
	return s.MaxConnections
}

// TLSStats returns a snapshot for this server process. Active includes handshakes.
func (s *Server) TLSStats() TLSStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	limit := s.limit
	if limit == 0 {
		limit = s.connectionLimit()
	}
	return TLSStats{Active: len(s.active), Limit: limit, Rejected: s.rejected}
}

func (s *Server) Start(ctx context.Context) error {
	limit := s.connectionLimit()
	if limit < 0 {
		return fmt.Errorf("maximum TLS connections must be non-negative, got %d", limit)
	}
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
	if s.closed || s.listener != nil {
		closed := s.closed
		s.mu.Unlock()
		listener.Close()
		httpListener.Close()
		if closed {
			return net.ErrClosed
		}
		return errors.New("gateway already started")
	}
	acceptCtx, cancelAccept := context.WithCancel(ctx)
	httpServer := &http.Server{Addr: s.HTTPAddr, ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(s.redirect)}
	s.listener = listener
	s.httpListener = httpListener
	s.cancelAccept = cancelAccept
	s.active = make(map[net.Conn]net.Conn)
	s.limit = limit
	s.http = httpServer
	s.wg.Add(1)
	s.mu.Unlock()
	go func() {
		if err := httpServer.Serve(httpListener); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP ingress failed: %v", err)
		}
	}()
	go func() {
		<-acceptCtx.Done()
		if ctx.Err() != nil {
			s.Close()
		}
	}()
	go func() {
		defer s.wg.Done()
		s.serveTLS(acceptCtx, listener)
	}()
	return nil
}

func (s *Server) serveTLS(ctx context.Context, listener net.Listener) {
	var retryDelay time.Duration
	for ctx.Err() == nil {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			if !retryableAcceptError(err) {
				log.Printf("TLS ingress failed: %v", err)
				return
			}
			if retryDelay == 0 {
				retryDelay = 5 * time.Millisecond
			} else {
				retryDelay = min(retryDelay*2, time.Second)
			}
			log.Printf("TLS ingress accept failed: %v, retrying in %s", err, retryDelay)
			timer := time.NewTimer(retryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				continue
			}
		}
		retryDelay = 0
		s.mu.Lock()
		if s.closed || ctx.Err() != nil {
			s.mu.Unlock()
			conn.Close()
			return
		}
		if len(s.active) >= s.limit {
			s.rejected++
			s.mu.Unlock()
			conn.Close()
			continue
		}
		s.active[conn] = nil
		s.wg.Add(1)
		s.mu.Unlock()
		go func() {
			defer func() {
				s.mu.Lock()
				delete(s.active, conn)
				s.mu.Unlock()
				s.wg.Done()
			}()
			s.handleTLS(ctx, conn)
		}()
	}
}

func retryableAcceptError(err error) bool {
	if errors.Is(err, syscall.EMFILE) || errors.Is(err, syscall.ENFILE) ||
		errors.Is(err, syscall.ENOBUFS) || errors.Is(err, syscall.ENOMEM) {
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError) && networkError.Temporary()
}

func (s *Server) Close() {
	s.mu.Lock()
	s.closed = true
	if s.cancelAccept != nil {
		s.cancelAccept()
	}
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

func (s *Server) handleTLS(ctx context.Context, in net.Conn) {
	defer in.Close()
	host, preface, err := ReadServerName(in)
	if err != nil {
		return
	}
	exit := s.Router.ExitName(host)
	if exit == "" {
		return
	}
	out, err := DialSOCKSContext(ctx, "singbox-"+exit+":1080", host, 443)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("exit %s dial failed: %v", exit, err)
		}
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
		copyTraffic(out, in)
		if half, ok := out.(interface{ CloseWrite() error }); ok {
			half.CloseWrite()
		}
		close(done)
	}()
	copyTraffic(in, out)
	if half, ok := in.(interface{ CloseWrite() error }); ok {
		half.CloseWrite()
	}
	in.Close()
	<-done
}

const trafficBufferSize = 32 * 1024

var trafficBuffers = sync.Pool{New: func() any { return new([trafficBufferSize]byte) }}

func copyTraffic(dst io.Writer, src io.Reader) (int64, error) {
	buffer := trafficBuffers.Get().(*[trafficBufferSize]byte)
	defer trafficBuffers.Put(buffer)
	return io.CopyBuffer(dst, src, buffer[:])
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
