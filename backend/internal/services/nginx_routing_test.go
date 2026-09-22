package services

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func freeRoutingPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func TestNginxRoutingIntegration(t *testing.T) {
	binaryPath, err := exec.LookPath("nginx")
	if err != nil {
		binaryPath = "/usr/sbin/nginx"
		if _, err = os.Stat(binaryPath); err != nil {
			t.Skip("nginx not installed")
		}
	}
	module := "/usr/lib/nginx/modules/ngx_stream_module.so"
	if _, err := os.Stat(module); err != nil {
		t.Skip("nginx stream module not installed")
	}
	rules, exits := routingFixtures()
	table, err := NewRoutingTable(rules, exits)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	httpPort, streamPort := freeRoutingPort(t), freeRoutingPort(t)
	config := fmt.Sprintf(`load_module %s;
 daemon off;
 master_process off;
 pid %s/nginx.pid;
 error_log %s/error.log;
 events { worker_connections 128; }
 http {
 access_log off;
 %s
 server { listen 127.0.0.1:%d; return 200 "$http_backend"; }
 }
 stream {
 %s
 server { listen 127.0.0.1:%d; ssl_preread on; return "$tls_backend"; }
 }
 `, module, dir, dir, buildHostMap("$host", "$http_backend", table.MapLines()), httpPort, buildHostMap("$ssl_preread_server_name", "$tls_backend", table.MapLines()), streamPort)
	path := filepath.Join(dir, "nginx.conf")
	if err := os.WriteFile(path, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	check := exec.Command(binaryPath, "-t", "-p", dir, "-c", path)
	if out, err := check.CombinedOutput(); err != nil {
		t.Fatalf("nginx -t: %v: %s", err, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	cmd := exec.CommandContext(ctx, binaryPath, "-p", dir, "-c", path)
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	defer func() { cancel(); _ = cmd.Wait() }()
	address := fmt.Sprintf("127.0.0.1:%d", httpPort)
	for {
		c, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			c.Close()
			break
		}
		if ctx.Err() != nil {
			t.Fatal("nginx failed to start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	for host, want := range map[string]string{
		"example.com": "gluetun-a:10001", "API.Example.COM": "gluetun-a:10001",
		"x.deep.example.com": "gluetun-a:10001", "special.example.com": "gluetun-b:10002",
		"x.special.example.com": "gluetun-b:10002", "only.example.net": "gluetun-b:10002",
		"x.only.example.net": "127.0.0.1:10099", "notexample.com": "127.0.0.1:10099",
	} {
		t.Run(host, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "http://"+address+"/", nil)
			req.Host = host + ":8080"
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != want {
				t.Fatalf("HTTP: got %q want %q", body, want)
			}
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", streamPort), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			if _, err := conn.Write(clientHello(t, host)); err != nil {
				t.Fatal(err)
			}
			body = make([]byte, len(want))
			_, err = io.ReadFull(conn, body)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != want {
				t.Fatalf("SNI: got %q want %q", body, want)
			}
		})
	}
}

func clientHello(t *testing.T, host string) []byte {
	t.Helper()
	writer, reader := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer writer.Close()
		_ = tls.Client(writer, &tls.Config{ServerName: host}).Handshake()
	}()
	defer func() { reader.Close(); <-done }()
	reader.SetDeadline(time.Now().Add(time.Second))
	header := make([]byte, 5)
	if _, err := io.ReadFull(reader, header); err != nil {
		t.Fatal(err)
	}
	body := make([]byte, int(binary.BigEndian.Uint16(header[3:])))
	if _, err := io.ReadFull(reader, body); err != nil {
		t.Fatal(err)
	}
	return bytes.Join([][]byte{header, body}, nil)
}

func TestNginxMapHasNoRegex(t *testing.T) {
	rules, exits := routingFixtures()
	table, err := NewRoutingTable(rules, exits)
	if err != nil {
		t.Fatal(err)
	}
	config := buildHostMap("$host", "$http_backend", table.MapLines())
	if !strings.Contains(config, "hostnames;") || strings.Contains(config, "~") {
		t.Fatal(config)
	}
}
