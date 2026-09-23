package gateway

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"io"
	"net"
	"testing"
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
