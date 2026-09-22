package services

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseListFile(t *testing.T) {
	input := "\ufeff# list\r\n\nExample.COM\nDOMAIN,example.com\nDOMAIN-SUFFIX,Example.COM,Proxy\n+.example.com\nDOMAIN, api.example.com ,Proxy\ntrailing.example.\nIP-CIDR,192.0.2.0/24\nhttps://bad.example/path\ninvalid.example;\n-bad.example\nempty..example\n"
	want := []ParsedRule{{"example.com", "exact"}, {"example.com", "suffix"}, {"api.example.com", "exact"}, {"trailing.example", "exact"}}
	if got := ParseListFile(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestParseListFileContinuesAfterLongLine(t *testing.T) {
	input := strings.Repeat("x", 70000) + ".com\nvalid.example\n"
	want := []ParsedRule{{"valid.example", "exact"}}
	if got := ParseListFile(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
