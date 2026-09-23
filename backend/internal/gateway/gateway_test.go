package gateway

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"io"
	"net"
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
	if got := len(server.slots); got != 0 {
		t.Fatalf("%d TLS slots still occupied", got)
	}
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
