package web

import (
	"testing"
	"time"
)

func TestTicketsExpire(t *testing.T) {
	ts := newTickets()
	now := time.Now()
	tk := ts.issue(now)
	if ts.redeem(tk, now.Add(ticketLife)) {
		t.Error("an expired ticket was accepted")
	}
}

func TestTicketsAreCapped(t *testing.T) {
	ts := newTickets()
	now := time.Now()
	first := ts.issue(now)
	for i := 0; i < maxTickets; i++ {
		ts.issue(now.Add(time.Duration(i+1) * time.Millisecond))
	}
	if len(ts.issued) != maxTickets {
		t.Errorf("%d tickets held, want %d", len(ts.issued), maxTickets)
	}
	if ts.redeem(first, now) {
		t.Error("the oldest ticket survived the cap")
	}
}
