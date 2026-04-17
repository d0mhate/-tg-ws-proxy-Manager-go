package faketls_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"net"
	"testing"

	"tg-ws-proxy/internal/faketls"
)

// make sure AcceptClientHello + SendFakeServerHello complete a faketls
// handshake and data still flows through faketls.Conn.
func TestFakeTLSRoundtrip(t *testing.T) {
	secret := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	hostname := "example.com"

	// in-process tcp pair
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	errCh := make(chan error, 1)
	go func() {
		// server side
		srv, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer srv.Close()

		clientRandom := faketls.AcceptClientHello(srv, secret)
		if clientRandom == nil {
			errCh <- nil // signal failure without hanging the test
			return
		}
		if err := faketls.SendFakeServerHello(srv, secret, clientRandom); err != nil {
			errCh <- err
			return
		}
		// read one tls AppData record through server-side Conn
		sc := faketls.NewConn(srv)
		buf := make([]byte, 128)
		n, err := sc.Read(buf)
		if err != nil {
			errCh <- err
			return
		}
		if string(buf[:n]) != "hello" {
			t.Errorf("server received %q, want %q", buf[:n], "hello")
		}
		errCh <- nil
	}()

	// client side
	cli, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()

	hello := faketls.BuildClientHello(hostname)
	faketls.SignClientHello(hello, secret)
	if _, err := cli.Write(hello); err != nil {
		t.Fatal("client write ClientHello:", err)
	}
	if !faketls.DrainServerHello(cli) {
		t.Fatal("client DrainServerHello failed")
	}

	// send one tls AppData payload
	cc := faketls.NewConn(cli)
	if _, err := cc.Write([]byte("hello")); err != nil {
		t.Fatal("client write AppData:", err)
	}

	if err := <-errCh; err != nil {
		t.Fatalf("server error: %v", err)
	}
}

// wrong secret should be rejected.
func TestAcceptClientHelloWrongSecret(t *testing.T) {
	secret := make([]byte, 16)
	wrongSecret := make([]byte, 16)
	rand.Read(secret)
	rand.Read(wrongSecret)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	resultCh := make(chan bool, 1)
	go func() {
		srv, err := ln.Accept()
		if err != nil {
			resultCh <- false
			return
		}
		defer srv.Close()
		resultCh <- faketls.AcceptClientHello(srv, wrongSecret) != nil
	}()

	cli, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()

	hello := faketls.BuildClientHello("example.com")
	faketls.SignClientHello(hello, secret)
	cli.Write(hello)

	if accepted := <-resultCh; accepted {
		t.Error("expected AcceptClientHello to fail for wrong secret, but it accepted")
	}
}

func TestSendFakeServerHelloMatchesTelegramClientValidation(t *testing.T) {
	secret := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}

	clientRandom := make([]byte, 32)
	if _, err := rand.Read(clientRandom); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := faketls.SendFakeServerHello(&buf, secret, clientRandom); err != nil {
		t.Fatalf("SendFakeServerHello: %v", err)
	}

	packet := buf.Bytes()
	reader := bytes.NewReader(packet)

	handshake := readTLSRecord(t, reader)
	if handshake[0] != 0x16 {
		t.Fatalf("unexpected first record type: 0x%02x", handshake[0])
	}
	ccs := readTLSRecord(t, reader)
	if ccs[0] != 0x14 {
		t.Fatalf("unexpected second record type: 0x%02x", ccs[0])
	}
	appData := readTLSRecord(t, reader)
	if appData[0] != 0x17 {
		t.Fatalf("unexpected third record type: 0x%02x", appData[0])
	}

	if reader.Len() != 0 {
		t.Fatalf("unexpected trailing bytes after fake TLS handshake: %d", reader.Len())
	}

	const serverRandomOffset = 11
	gotDigest := append([]byte(nil), handshake[serverRandomOffset:serverRandomOffset+32]...)

	zeroedPacket := append([]byte(nil), packet...)
	copy(zeroedPacket[serverRandomOffset:serverRandomOffset+32], make([]byte, 32))

	mac := hmac.New(sha256.New, secret)
	mac.Write(clientRandom)
	mac.Write(zeroedPacket)
	wantDigest := mac.Sum(nil)

	if !hmac.Equal(gotDigest, wantDigest) {
		t.Fatalf("server hello digest mismatch")
	}
}

func readTLSRecord(t *testing.T, r io.Reader) []byte {
	t.Helper()

	hdr := make([]byte, 5)
	if _, err := io.ReadFull(r, hdr); err != nil {
		t.Fatalf("read TLS header: %v", err)
	}
	length := int(hdr[3])<<8 | int(hdr[4])
	record := make([]byte, 5+length)
	copy(record, hdr)
	if _, err := io.ReadFull(r, record[5:]); err != nil {
		t.Fatalf("read TLS payload: %v", err)
	}
	return record
}
