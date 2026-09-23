package singbox

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"testing"

	"github.com/youmark/pkcs8"
)

func TestEncryptedProviderOpenVPNKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := pkcs8.MarshalPrivateKey(key, []byte("test-passphrase"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := privateKeyPEM(base64.StdEncoding.EncodeToString(der), "wrong-passphrase"); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	decrypted, err := privateKeyPEM(base64.StdEncoding.EncodeToString(der), "test-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode([]byte(decrypted))
	if block == nil || block.Type != "PRIVATE KEY" {
		t.Fatal("missing decrypted PKCS#8 PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.(*rsa.PrivateKey).N.Cmp(key.N) != 0 {
		t.Fatal("decrypted key changed")
	}
}
