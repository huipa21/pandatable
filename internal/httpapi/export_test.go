package httpapi

import (
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"pandatable/internal/apperr"
)

func TestExportTicketsAreBoundSingleUseAndBounded(t *testing.T) {
	tickets := newExportTickets()
	now := time.Unix(1_000, 0)
	session := sha256.Sum256([]byte("first session"))
	otherSession := sha256.Sum256([]byte("other session"))
	tokens := make([]string, 0, maxUserTickets)
	for range maxUserTickets {
		token, err := tickets.issue(exportTicket{userID: "alice", session: session, viewID: "view"}, now)
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
	}
	_, err := tickets.issue(exportTicket{userID: "alice"}, now)
	var rateLimit *apperr.Error
	if !errors.As(err, &rateLimit) || rateLimit.Status != 429 {
		t.Fatalf("per-user limit returned %v", err)
	}
	if _, err := tickets.consume(tokens[0], "alice", otherSession, now); err == nil {
		t.Fatal("ticket accepted another session")
	}
	if _, err := tickets.consume(tokens[0], "bob", session, now); err == nil {
		t.Fatal("ticket accepted another user")
	}
	if ticket, err := tickets.consume(tokens[0], "alice", session, now); err != nil || ticket.viewID != "view" {
		t.Fatalf("valid ticket = %+v, %v", ticket, err)
	}
	if _, err := tickets.consume(tokens[0], "alice", session, now); err == nil {
		t.Fatal("ticket was reusable")
	}
	if _, err := tickets.consume(tokens[1], "alice", session, now.Add(exportTicketTTL)); err == nil {
		t.Fatal("expired ticket was accepted")
	}
	if _, err := tickets.issue(exportTicket{userID: "alice"}, now.Add(exportTicketTTL)); err != nil {
		t.Fatalf("expired tickets did not free capacity: %v", err)
	}

	for index := 0; index < maxExportTickets-1; index++ {
		if _, err := tickets.issue(exportTicket{userID: string(rune(index + 1))}, now.Add(exportTicketTTL)); err != nil {
			t.Fatal(err)
		}
	}
	_, err = tickets.issue(exportTicket{userID: "overflow"}, now.Add(exportTicketTTL))
	if !errors.As(err, &rateLimit) || rateLimit.Status != 429 {
		t.Fatalf("global limit returned %v", err)
	}
}
