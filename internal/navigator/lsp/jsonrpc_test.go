package lsp

import (
	"bufio"
	"fmt"
	"io"
	"net/textproto"
	"strings"
	"testing"
)

func TestReadMessageRejectsBadContentLength(t *testing.T) {
	for _, n := range []string{"-1", fmt.Sprint(maxMessage + 1), "x"} {
		br := bufio.NewReader(strings.NewReader("Content-Length: " + n + "\r\n\r\n{}"))
		if _, err := readMessage(textproto.NewReader(br), br); err == nil {
			t.Errorf("Content-Length %s: no error", n)
		}
	}
}

func TestReadMessage(t *testing.T) {
	body := `{"jsonrpc":"2.0","method":"initialized"}`
	br := bufio.NewReader(strings.NewReader(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)))
	m, err := readMessage(textproto.NewReader(br), br)
	if err != nil || m.Method != "initialized" {
		t.Fatalf("got %+v, %v", m, err)
	}
}

// A connection that reads a bad Content-Length closes, failing pending
// calls, rather than panicking.
func TestConnClosesOnBadContentLength(t *testing.T) {
	c := newConn(io.Discard)
	c.read(strings.NewReader("Content-Length: -5\r\n\r\n"))
	if c.closed == nil {
		t.Fatal("connection not closed")
	}
}
