package smtpmail

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSecretEncryptionAndValidation(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	first, err := Seal(key, "secret")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Seal(key, "secret")
	if err != nil || bytes.Equal(first, second) || bytes.Contains(first, []byte("secret")) {
		t.Fatal("SMTP ciphertext must use a random nonce")
	}
	value, err := Open(key, first)
	if err != nil || value != "secret" {
		t.Fatal("SMTP password did not round-trip", err)
	}
	if _, err := Open(bytes.Repeat([]byte{2}, 32), first); err == nil {
		t.Fatal("wrong key must fail")
	}
	if _, err := Seal(nil, "secret"); err == nil {
		t.Fatal("missing key must fail")
	}
	for _, config := range []Config{
		{Enabled: true, Host: "mail.example.test", Port: 25, Security: "none", FromAddress: "a@example.test"},
		{Enabled: true, Host: "bad\r\nhost", Port: 587, Security: "tls", FromAddress: "a@example.test"},
		{Enabled: true, Host: "localhost", Port: 587, Security: "tls", FromAddress: "bad\r\n@example.test"},
		{Enabled: true, Host: "localhost", Port: 587, Security: "tls", Username: "user", FromAddress: "a@example.test"},
	} {
		if err := Validate(config, false); err == nil {
			t.Fatalf("accepted invalid SMTP configuration: %+v", config)
		}
	}
}

func TestLoopbackPeer(t *testing.T) {
	for _, test := range []struct {
		address net.Addr
		want    bool
	}{
		{&net.TCPAddr{IP: net.ParseIP("127.0.0.2"), Port: 25}, true},
		{&net.TCPAddr{IP: net.ParseIP("::1"), Port: 25}, true},
		{&net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 25}, false},
		{&net.TCPAddr{Port: 25}, false},
		{&net.UnixAddr{Name: "test", Net: "unix"}, false},
	} {
		if got := loopbackPeer(test.address); got != test.want {
			t.Errorf("loopbackPeer(%v) = %v, want %v", test.address, got, test.want)
		}
	}
}

func TestSendSMTPAndAuthenticationFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	messages := make(chan string, 2)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go fakeSMTP(conn, messages)
		}
	}()
	defer func() {
		_ = listener.Close()
		<-done
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	config := Config{Enabled: true, Host: "127.0.0.1", Port: port, Security: "none",
		Username: "test", Password: "correct", FromAddress: "sender@example.test"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := Send(ctx, config, "recipient@example.test", "Test subject", "Test body"); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-messages:
		if !strings.Contains(message, "Test subject") || !strings.Contains(message, "Test body") ||
			!strings.Contains(message, "recipient@example.test") {
			t.Fatal("SMTP message missing expected content")
		}
	case <-ctx.Done():
		t.Fatal("SMTP message not received")
	}
	config.Host = "LOCALHOST"
	if err := Send(ctx, config, "recipient@example.test", "Test", "Loopback"); err != nil {
		t.Fatal("case-insensitive localhost must support local SMTP authentication", err)
	}
	select {
	case <-messages:
	case <-ctx.Done():
		t.Fatal("localhost SMTP message not received")
	}
	config.Host = "127.0.0.1"
	if err := Send(ctx, config, "recipient@example.test", "测试主题", "你好，PandaTable"); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-messages:
		for _, value := range []byte(message) {
			if value >= 0x80 {
				t.Fatal("SMTP message must be 7-bit safe without 8BITMIME")
			}
		}
		parsed, err := mail.ReadMessage(strings.NewReader(message))
		if err != nil || parsed.Header.Get("Content-Transfer-Encoding") != "quoted-printable" {
			t.Fatal("missing quoted-printable SMTP body", err)
		}
		decoded, err := io.ReadAll(quotedprintable.NewReader(parsed.Body))
		if err != nil || strings.TrimSpace(string(decoded)) != "你好，PandaTable" {
			t.Fatal("localized SMTP body did not round-trip", err)
		}
	case <-ctx.Done():
		t.Fatal("localized SMTP message not received")
	}
	config.Password = "wrong"
	if err := Send(ctx, config, "recipient@example.test", "Test", "Body"); err == nil {
		t.Fatal("SMTP authentication failure must be returned")
	}
	config.Security = "starttls"
	if err := Send(ctx, config, "recipient@example.test", "Test", "Body"); err == nil {
		t.Fatal("STARTTLS must not silently fall back to plaintext")
	}
}

func fakeSMTP(conn net.Conn, messages chan<- string) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(conn)
	_, _ = fmt.Fprint(conn, "220 localhost ESMTP\r\n")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		command := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(command, "EHLO "):
			_, _ = fmt.Fprint(conn, "250-localhost\r\n250-AUTH PLAIN\r\n250 HELP\r\n")
		case strings.HasPrefix(command, "AUTH PLAIN "):
			value, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(command, "AUTH PLAIN "))
			if string(value) == "\x00test\x00correct" {
				_, _ = fmt.Fprint(conn, "235 authenticated\r\n")
			} else {
				_, _ = fmt.Fprint(conn, "535 authentication failed\r\n")
			}
		case strings.HasPrefix(command, "MAIL "), strings.HasPrefix(command, "RCPT "):
			_, _ = fmt.Fprint(conn, "250 ok\r\n")
		case command == "DATA":
			_, _ = fmt.Fprint(conn, "354 send data\r\n")
			var data strings.Builder
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if line == ".\r\n" {
					break
				}
				data.WriteString(line)
			}
			messages <- data.String()
			_, _ = fmt.Fprint(conn, "250 queued\r\n")
		case command == "QUIT":
			_, _ = fmt.Fprint(conn, "221 bye\r\n")
			return
		default:
			_, _ = fmt.Fprint(conn, "500 unexpected "+strconv.Quote(command)+"\r\n")
		}
	}
}
