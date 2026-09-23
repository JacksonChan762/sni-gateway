package gateway

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const maxClientHello = 64 * 1024

// ReadServerName reads one TLS ClientHello without consuming it for the upstream.
// The returned bytes must be forwarded before copying the rest of the connection.
func ReadServerName(conn net.Conn) (string, []byte, error) {
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return "", nil, err
	}
	defer conn.SetReadDeadline(time.Time{})
	var wire, handshake []byte
	for len(handshake) < 4 || len(handshake) < 4+(int(handshake[1])<<16)+(int(handshake[2])<<8)+int(handshake[3]) {
		var header [5]byte
		if _, err := io.ReadFull(conn, header[:]); err != nil {
			return "", nil, err
		}
		length := int(binary.BigEndian.Uint16(header[3:]))
		if header[0] != 22 || length == 0 || length > 18*1024 || len(wire)+5+length > maxClientHello {
			return "", nil, errors.New("invalid or oversized TLS ClientHello")
		}
		wire = append(wire, header[:]...)
		start := len(wire)
		wire = append(wire, make([]byte, length)...)
		if _, err := io.ReadFull(conn, wire[start:]); err != nil {
			return "", nil, err
		}
		handshake = append(handshake, wire[start:]...)
		if len(handshake) >= 4 {
			if handshake[0] != 1 || 4+(int(handshake[1])<<16)+(int(handshake[2])<<8)+int(handshake[3]) > maxClientHello {
				return "", nil, errors.New("not a TLS ClientHello")
			}
		}
	}
	name, err := clientHelloServerName(handshake[4:])
	if err != nil {
		return "", nil, err
	}
	return name, wire, nil
}

func clientHelloServerName(hello []byte) (string, error) {
	if len(hello) < 34 {
		return "", errors.New("short TLS ClientHello")
	}
	pos := 34 // legacy version and random
	read := func(n int) ([]byte, error) {
		if n < 0 || pos+n > len(hello) {
			return nil, io.ErrUnexpectedEOF
		}
		part := hello[pos : pos+n]
		pos += n
		return part, nil
	}
	length, err := read(1)
	if err != nil {
		return "", err
	}
	if _, err = read(int(length[0])); err != nil {
		return "", err
	}
	length, err = read(2)
	if err != nil {
		return "", err
	}
	if _, err = read(int(binary.BigEndian.Uint16(length))); err != nil {
		return "", err
	}
	length, err = read(1)
	if err != nil {
		return "", err
	}
	if _, err = read(int(length[0])); err != nil {
		return "", err
	}
	length, err = read(2)
	if err != nil {
		return "", err
	}
	extensions, err := read(int(binary.BigEndian.Uint16(length)))
	if err != nil {
		return "", err
	}
	for len(extensions) >= 4 {
		typ := binary.BigEndian.Uint16(extensions[:2])
		n := int(binary.BigEndian.Uint16(extensions[2:4]))
		extensions = extensions[4:]
		if n > len(extensions) {
			return "", io.ErrUnexpectedEOF
		}
		data := extensions[:n]
		extensions = extensions[n:]
		if typ != 0 {
			continue
		}
		if len(data) < 5 || int(binary.BigEndian.Uint16(data[:2])) != len(data)-2 || data[2] != 0 {
			return "", errors.New("invalid TLS server name extension")
		}
		nameLen := int(binary.BigEndian.Uint16(data[3:5]))
		if nameLen == 0 || nameLen > 253 || nameLen != len(data)-5 {
			return "", errors.New("invalid TLS server name")
		}
		name := strings.ToLower(string(data[5:]))
		if !validDNSName(name) {
			return "", fmt.Errorf("invalid TLS server name %q", name)
		}
		return name, nil
	}
	return "", errors.New("TLS ClientHello has no server name")
}

func validDNSName(host string) bool {
	if len(host) == 0 || len(host) > 253 || strings.Contains(host, "..") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
				return false
			}
		}
	}
	return true
}
