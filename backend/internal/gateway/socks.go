package gateway

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// DialSOCKS connects by hostname so DNS resolution happens inside the VPN exit.
func DialSOCKS(proxyAddress, host string, port uint16) (net.Conn, error) {
	return DialSOCKSContext(context.Background(), proxyAddress, host, port)
}

func DialSOCKSContext(ctx context.Context, proxyAddress, host string, port uint16) (net.Conn, error) {
	if !validDNSName(host) {
		return nil, errors.New("invalid destination hostname")
	}
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", proxyAddress)
	if err != nil {
		return nil, err
	}
	stopCancel := context.AfterFunc(ctx, func() { conn.Close() })
	defer stopCancel()
	connected := false
	defer func() {
		if !connected {
			conn.Close()
		}
	}()
	if err = conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return nil, err
	}
	if _, err = io.Copy(conn, bytes.NewReader([]byte{5, 1, 0})); err != nil {
		return nil, err
	}
	var response [4]byte
	if _, err = io.ReadFull(conn, response[:2]); err != nil {
		return nil, err
	}
	if response[0] != 5 || response[1] != 0 {
		return nil, errors.New("SOCKS authentication refused")
	}
	request := []byte{5, 1, 0, 3, byte(len(host))}
	request = append(request, host...)
	var p [2]byte
	binary.BigEndian.PutUint16(p[:], port)
	request = append(request, p[:]...)
	if _, err = io.Copy(conn, bytes.NewReader(request)); err != nil {
		return nil, err
	}
	if _, err = io.ReadFull(conn, response[:]); err != nil {
		return nil, err
	}
	if response[0] != 5 || response[1] != 0 || response[2] != 0 {
		return nil, fmt.Errorf("SOCKS connect failed: %d", response[1])
	}
	var addressLength int
	switch response[3] {
	case 1:
		addressLength = 4
	case 3:
		var n [1]byte
		if _, err = io.ReadFull(conn, n[:]); err != nil {
			return nil, err
		}
		addressLength = int(n[0])
	case 4:
		addressLength = 16
	default:
		return nil, errors.New("invalid SOCKS response address type")
	}
	if _, err = io.CopyN(io.Discard, conn, int64(addressLength+2)); err != nil {
		return nil, err
	}
	if err = conn.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	connected = true
	return conn, nil
}
