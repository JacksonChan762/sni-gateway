package gateway

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestReadServerNamePreservesClientHello(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() { _ = tls.Client(client, &tls.Config{ServerName: "API.Example.COM"}).Handshake() }()
	name, wire, err := ReadServerName(server)
	if err != nil {
		t.Fatal(err)
	}
	if name != "api.example.com" {
		t.Fatalf("name = %q", name)
	}
	if len(wire) < 6 || wire[0] != 22 {
		t.Fatalf("invalid captured ClientHello: %x", wire)
	}
	if int(binary.BigEndian.Uint16(wire[3:5]))+5 != len(wire) {
		t.Fatal("captured bytes lost")
	}
}

func TestReadServerNameRejectsPlaintext(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() { _, _ = client.Write([]byte("GET / HTTP/1.1\r\n")) }()
	if _, _, err := ReadServerName(server); err == nil {
		t.Fatal("plaintext accepted")
	}
}

func TestDialSOCKSUsesDomainDestination(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	result := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			result <- err.Error()
			return
		}
		defer conn.Close()
		greeting := make([]byte, 3)
		if _, err := io.ReadFull(conn, greeting); err != nil {
			result <- err.Error()
			return
		}
		if !bytes.Equal(greeting, []byte{5, 1, 0}) {
			result <- "bad greeting"
			return
		}
		_, _ = conn.Write([]byte{5, 0})
		header := make([]byte, 5)
		if _, err := io.ReadFull(conn, header); err != nil {
			result <- err.Error()
			return
		}
		name := make([]byte, int(header[4])+2)
		if _, err := io.ReadFull(conn, name); err != nil {
			result <- err.Error()
			return
		}
		if !bytes.Equal(header[:4], []byte{5, 1, 0, 3}) {
			result <- "bad request"
			return
		}
		result <- string(name[:len(name)-2])
		_, _ = conn.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
	}()
	conn, err := DialSOCKS(listener.Addr().String(), "example.com", 443)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if got := <-result; got != "example.com" {
		t.Fatalf("SOCKS destination = %q", got)
	}
}

func TestDialSOCKSContextCancelsHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if conn, err := DialSOCKSContext(ctx, listener.Addr().String(), "example.com", 443); err == nil {
		conn.Close()
		t.Fatal("SOCKS handshake ignored cancellation")
	}
	select {
	case conn := <-accepted:
		defer conn.Close()
		conn.SetReadDeadline(time.Now().Add(time.Second))
		_, err := io.ReadAll(conn)
		if err != nil {
			t.Fatalf("cancelled socket stayed open: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SOCKS connection was never accepted")
	}
}

type noRoutes struct{}

func (noRoutes) ExitName(string) string { return "" }

func TestCloseReleasesPendingTLSConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := &Server{Router: noRoutes{}, HTTPAddr: "127.0.0.1:0", TLSAddr: "127.0.0.1:0"}
	if err := server.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	conn, err := net.Dial("tcp", server.listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	deadline := time.Now().Add(time.Second)
	for {
		server.mu.Lock()
		count := len(server.active)
		server.mu.Unlock()
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("connection was not tracked")
		}
		time.Sleep(time.Millisecond)
	}
	server.Close()
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("connection stayed open after server shutdown")
	}
	if got := server.TLSStats().Active; got != 0 {
		t.Fatalf("%d TLS slots still occupied", got)
	}
}

func TestTLSConnectionLimitAndStats(t *testing.T) {
	server := &Server{Router: noRoutes{}, HTTPAddr: "127.0.0.1:0", TLSAddr: "127.0.0.1:0", MaxConnections: 2}
	if err := server.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Close)
	serverAddr := server.listener.Addr().String()
	dial := func() net.Conn {
		t.Helper()
		conn, err := net.DialTimeout("tcp", serverAddr, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		return conn
	}
	first := dial()
	dial()
	waitForTLSStats(t, server, TLSStats{Active: 2, Limit: 2})

	rejected := dial()
	if err := rejected.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	_, err := rejected.Read(make([]byte, 1))
	var networkError net.Error
	if err == nil || (errors.As(err, &networkError) && networkError.Timeout()) {
		t.Fatalf("expected the extra TLS connection to close immediately, got %v", err)
	}
	waitForTLSStats(t, server, TLSStats{Active: 2, Limit: 2, Rejected: 1})

	first.Close()
	waitForTLSStats(t, server, TLSStats{Active: 1, Limit: 2, Rejected: 1})
	dial()
	waitForTLSStats(t, server, TLSStats{Active: 2, Limit: 2, Rejected: 1})
	server.Close()
	waitForTLSStats(t, server, TLSStats{Limit: 2, Rejected: 1})
}

func waitForTLSStats(t *testing.T, server *Server, want TLSStats) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		got := server.TLSStats()
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("TLS stats = %+v, want %+v", got, want)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestTLSConnectionLimitDefaultAndInvalid(t *testing.T) {
	server := &Server{}
	if got := server.TLSStats(); got != (TLSStats{Limit: 2048}) {
		t.Fatalf("default stats = %+v", got)
	}
	server = &Server{MaxConnections: -1, TLSAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"}
	if err := server.Start(context.Background()); err == nil {
		server.Close()
		t.Fatal("negative TLS connection limit accepted")
	}
}

type scriptedTLSListener struct {
	net.Listener
	accept func() (net.Conn, error)
}

func (l scriptedTLSListener) Accept() (net.Conn, error) { return l.accept() }

func TestTLSAcceptRecoversFromTemporaryResourceErrors(t *testing.T) {
	server := &Server{Router: noRoutes{}, active: make(map[net.Conn]net.Conn), limit: 1}
	t.Cleanup(server.Close)
	client, conn := net.Pipe()
	t.Cleanup(func() { client.Close() })
	var calls int
	transient := []error{syscall.EMFILE, syscall.ENFILE, syscall.ENOBUFS, syscall.ENOMEM}
	listener := scriptedTLSListener{accept: func() (net.Conn, error) {
		calls++
		if calls <= len(transient) {
			return nil, &net.OpError{Op: "accept", Net: "tcp", Err: os.NewSyscallError("accept", transient[calls-1])}
		}
		if calls == len(transient)+1 {
			return conn, nil
		}
		return nil, net.ErrClosed
	}}
	server.serveTLS(context.Background(), listener)
	if calls != len(transient)+2 {
		t.Fatalf("accept called %d times, want %d", calls, len(transient)+2)
	}
	if got := server.TLSStats(); got != (TLSStats{Active: 1, Limit: 1}) {
		t.Fatalf("expected the next connection to be admitted after recovery, got %+v", got)
	}
}

func TestTLSAcceptStopsOnCancellationOrPermanentError(t *testing.T) {
	t.Run("cancellation", func(t *testing.T) {
		server := &Server{}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		firstAttempt := make(chan struct{})
		finished := make(chan struct{})
		listener := scriptedTLSListener{accept: func() (net.Conn, error) {
			select {
			case <-firstAttempt:
			default:
				close(firstAttempt)
			}
			return nil, syscall.EMFILE
		}}
		go func() {
			defer close(finished)
			server.serveTLS(ctx, listener)
		}()
		select {
		case <-firstAttempt:
		case <-time.After(time.Second):
			t.Fatal("listener never attempted to accept")
		}
		cancel()
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("accept loop did not stop after cancellation")
		}
	})
	t.Run("permanent error", func(t *testing.T) {
		server := &Server{}
		calls := 0
		listener := scriptedTLSListener{accept: func() (net.Conn, error) {
			calls++
			if calls > 1 {
				t.Fatal("permanent accept error was retried")
			}
			return nil, errors.New("permanent listener failure")
		}}
		server.serveTLS(context.Background(), listener)
		if calls != 1 {
			t.Fatalf("accept called %d times, want 1", calls)
		}
	})
}

func TestIdleWatcherClosesBothConnections(t *testing.T) {
	in, client := net.Pipe()
	out, upstream := net.Pipe()
	defer client.Close()
	defer upstream.Close()
	_, _, stop := watchIdle(in, out, 40*time.Millisecond)
	defer stop()
	client.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("idle client connection stayed open")
	}
	upstream.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := upstream.Read(make([]byte, 1)); err == nil {
		t.Fatal("idle upstream connection stayed open")
	}
}

func TestIdleWatcherTrafficExtendsConnection(t *testing.T) {
	in, client := net.Pipe()
	out, upstream := net.Pipe()
	defer client.Close()
	defer upstream.Close()
	wrappedIn, _, stop := watchIdle(in, out, 300*time.Millisecond)
	defer stop()
	time.Sleep(180 * time.Millisecond)
	written := make(chan error, 1)
	go func() { _, err := wrappedIn.Write([]byte("x")); written <- err }()
	client.SetReadDeadline(time.Now().Add(time.Second))
	var received [1]byte
	if _, err := client.Read(received[:]); err != nil || received[0] != 'x' {
		t.Fatalf("traffic failed: %q, %v", received, err)
	}
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	time.Sleep(180 * time.Millisecond)
	upstream.SetReadDeadline(time.Now().Add(30 * time.Millisecond))
	_, err := upstream.Read(make([]byte, 1))
	ne, ok := err.(net.Error)
	if !ok || !ne.Timeout() {
		t.Fatalf("connection closed despite recent traffic: %v", err)
	}
	upstream.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := upstream.Read(make([]byte, 1)); err == nil {
		t.Fatal("connection did not close after traffic stopped")
	}
}

func TestActivityConnPreservesTCPHalfClose(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	wrapped := &activityConn{Conn: client, touch: func() {}}
	if err := wrapped.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	server.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := server.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("peer did not receive EOF: %v", err)
	}
	if _, err := server.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	client.SetReadDeadline(time.Now().Add(time.Second))
	var response [1]byte
	if _, err := client.Read(response[:]); err != nil || response[0] != 'x' {
		t.Fatalf("reverse traffic stopped after half-close: %q, %v", response, err)
	}
}

func BenchmarkRelayCopy(b *testing.B) {
	payload := make([]byte, 64*1024)
	for _, tt := range []struct {
		name string
		copy func(io.Writer, io.Reader) (int64, error)
	}{
		{"unpooled", io.Copy},
		{"pooled", copyTraffic},
	} {
		b.Run(tt.name, func(b *testing.B) {
			var reader bytes.Reader
			// The relay's activityConn exposes Read and Write without the io.Copy fast paths.
			src := &struct{ io.Reader }{&reader}
			dst := &struct{ io.Writer }{io.Discard}
			b.SetBytes(int64(len(payload)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				reader.Reset(payload)
				if n, err := tt.copy(dst, src); n != int64(len(payload)) || err != nil {
					b.Fatalf("copied %d of %d bytes: %v", n, len(payload), err)
				}
			}
		})
	}
}
