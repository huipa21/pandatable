package smtpmail

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/netip"
	"net/smtp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrKey      = errors.New("SMTP encryption key is unavailable or invalid")
	ErrDisabled = errors.New("SMTP is disabled")
)

type Config struct {
	Enabled     bool
	Host        string
	Port        int
	Security    string
	Username    string
	Password    string
	FromAddress string
}

func Seal(key []byte, value string) ([]byte, error) {
	if len(key) != 32 {
		return nil, ErrKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrKey
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrKey
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate SMTP nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, []byte(value), nil), nil
}

func Open(key, value []byte) (string, error) {
	if len(key) != 32 {
		return "", ErrKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", ErrKey
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(value) < gcm.NonceSize() {
		return "", ErrKey
	}
	plain, err := gcm.Open(nil, value[:gcm.NonceSize()], value[gcm.NonceSize():], nil)
	if err != nil {
		return "", ErrKey
	}
	defer clear(plain)
	return string(plain), nil
}

func validHost(host string) bool {
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.Zone() == ""
	}
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

func loopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.IsLoopback() && ip.Zone() == ""
}

func loopbackPeer(address net.Addr) bool {
	peer, ok := address.(*net.TCPAddr)
	return ok && peer.AddrPort().Addr().Unmap().IsLoopback()
}

func validAddress(value string) bool {
	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value && !strings.ContainsAny(value, "\r\n") && len(value) <= 254
}

func Validate(config Config, hasPassword bool) error {
	if config.Port < 1 || config.Port > 65535 ||
		config.Security != "starttls" && config.Security != "tls" && config.Security != "none" ||
		!utf8.ValidString(config.Username) || len(config.Username) > 254 ||
		strings.ContainsFunc(config.Username, unicode.IsControl) ||
		config.Host != "" && !validHost(config.Host) ||
		config.FromAddress != "" && !validAddress(config.FromAddress) {
		return errors.New("invalid SMTP settings")
	}
	if config.Security == "none" && config.Host != "" && !loopback(config.Host) {
		return errors.New("unencrypted SMTP requires loopback")
	}
	if config.Enabled && (config.Host == "" || config.FromAddress == "" || config.Username != "" && !hasPassword) {
		return errors.New("incomplete SMTP settings")
	}
	return nil
}

func Send(ctx context.Context, config Config, recipient, subject, body string) error {
	if !config.Enabled {
		return ErrDisabled
	}
	if err := Validate(config, config.Password != ""); err != nil || !validAddress(recipient) ||
		strings.ContainsAny(subject, "\r\n") || strings.Contains(body, "\r") {
		return errors.New("invalid SMTP message or settings")
	}
	address := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("connect SMTP: %w", err)
	}
	defer func() { _ = conn.Close() }()
	smtpHost := config.Host
	if config.Security == "none" {
		if !loopbackPeer(conn.RemoteAddr()) {
			return errors.New("unencrypted SMTP requires a loopback connection")
		}
		// PlainAuth accepts only canonical loopback names, regardless of the peer.
		smtpHost = "localhost"
	}
	deadline := time.Now().Add(8 * time.Second)
	if end, ok := ctx.Deadline(); ok && end.Before(deadline) {
		deadline = end
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("set SMTP deadline: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()
	tlsConfig := &tls.Config{ServerName: config.Host, MinVersion: tls.VersionTLS12}
	var transport net.Conn = conn
	if config.Security == "tls" {
		secure := tls.Client(conn, tlsConfig)
		if err := secure.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("establish SMTP TLS: %w", err)
		}
		transport = secure
	}
	client, err := smtp.NewClient(transport, smtpHost)
	if err != nil {
		return fmt.Errorf("open SMTP session: %w", err)
	}
	defer func() { _ = client.Close() }()
	if config.Security == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("SMTP server does not offer STARTTLS")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("start SMTP TLS: %w", err)
		}
	}
	if config.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", config.Username, config.Password, smtpHost)); err != nil {
			return fmt.Errorf("authenticate SMTP: %w", err)
		}
	}
	if err := client.Mail(config.FromAddress); err != nil {
		return fmt.Errorf("set SMTP sender: %w", err)
	}
	if err := client.Rcpt(recipient); err != nil {
		return fmt.Errorf("set SMTP recipient: %w", err)
	}
	var encodedBody bytes.Buffer
	encoder := quotedprintable.NewWriter(&encodedBody)
	if _, err := io.WriteString(encoder, strings.ReplaceAll(body, "\n", "\r\n")); err != nil {
		return fmt.Errorf("encode SMTP message: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("finish encoding SMTP message: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("open SMTP message: %w", err)
	}
	message := "From: " + (&mail.Address{Address: config.FromAddress}).String() +
		"\r\nTo: " + (&mail.Address{Address: recipient}).String() +
		"\r\nSubject: " + mime.QEncoding.Encode("utf-8", subject) +
		"\r\nDate: " + time.Now().Format(time.RFC1123Z) +
		"\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n\r\n" + encodedBody.String() + "\r\n"
	if _, err := io.WriteString(writer, message); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write SMTP message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish SMTP message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("finish SMTP session: %w", err)
	}
	return nil
}
