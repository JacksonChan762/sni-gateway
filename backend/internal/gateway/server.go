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
	"time"
)

type Router interface{ ExitName(host string) string }

// Server accepts clear HTTP redirects and TLS passthrough on separate ports.
type Server struct {
	Router       Router
	HTTPAddr     string
	TLSAddr      string
	http         *http.Server
	listener     net.Listener
	httpListener net.Listener
	mu           sync.Mutex
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
	s.listener = listener
	s.httpListener = httpListener
	s.mu.Unlock()
	s.http = &http.Server{Addr: s.HTTPAddr, ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(s.redirect)}
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
				go func() { defer func() { <-s.slots }(); s.handleTLS(conn) }()
			default:
				conn.Close()
			}
		}
	}()
	return nil
}

func (s *Server) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}
	if s.httpListener != nil {
		s.httpListener.Close()
		s.httpListener = nil
	}
	if s.http != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.http.Shutdown(ctx)
	}
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
	if _, err := io.Copy(out, bytes.NewReader(preface)); err != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		io.Copy(out, in)
		if tcp, ok := out.(*net.TCPConn); ok {
			tcp.CloseWrite()
		}
		close(done)
	}()
	io.Copy(in, out)
	if tcp, ok := in.(*net.TCPConn); ok {
		tcp.CloseWrite()
	}
	in.Close()
	<-done
}
