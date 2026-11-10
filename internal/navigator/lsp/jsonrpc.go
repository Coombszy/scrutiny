package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"sync"
)

// conn is a JSON-RPC 2.0 connection framed with LSP's Content-Length headers.
type conn struct {
	w  io.Writer
	wm sync.Mutex

	mu      sync.Mutex
	nextID  int
	pending map[int]chan response
	closed  error

	// onNotify handles notifications; onRequest answers server requests.
	onNotify  func(method string, params json.RawMessage)
	onRequest func(method string, params json.RawMessage) (any, error)
}

type message struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("lsp error %d: %s", e.Code, e.Message) }

type response struct {
	result json.RawMessage
	err    error
}

func newConn(w io.Writer) *conn {
	return &conn{w: w, pending: map[int]chan response{}}
}

// call sends a request and waits for its response.
func (c *conn) call(ctx context.Context, method string, params, result any) error {
	c.mu.Lock()
	if c.closed != nil {
		c.mu.Unlock()
		return c.closed
	}
	c.nextID++
	id := c.nextID
	ch := make(chan response, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	raw := json.RawMessage(strconv.Itoa(id))
	if err := c.send(message{ID: &raw, Method: method, Params: mustJSON(params)}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		_ = c.notify("$/cancelRequest", map[string]int{"id": id})
		return ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return r.err
		}
		if result == nil || len(r.result) == 0 {
			return nil
		}
		return json.Unmarshal(r.result, result)
	}
}

func (c *conn) notify(method string, params any) error {
	return c.send(message{Method: method, Params: mustJSON(params)})
}

func (c *conn) send(m message) error {
	m.JSONRPC = "2.0"
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	c.wm.Lock()
	defer c.wm.Unlock()
	if _, err := fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = c.w.Write(body)
	return err
}

// read dispatches messages from r until it fails, then fails every pending
// call with that error.
func (c *conn) read(r io.Reader) {
	br := bufio.NewReader(r)
	tp := textproto.NewReader(br)
	var err error
	for {
		var m message
		if m, err = readMessage(tp, br); err != nil {
			break
		}
		switch {
		case m.ID != nil && m.Method != "":
			go c.answer(m)
		case m.ID != nil:
			id, _ := strconv.Atoi(string(*m.ID))
			c.mu.Lock()
			ch := c.pending[id]
			delete(c.pending, id)
			c.mu.Unlock()
			if ch != nil {
				r := response{result: m.Result}
				if m.Error != nil {
					r.err = m.Error
				}
				ch <- r
			}
		case m.Method != "" && c.onNotify != nil:
			c.onNotify(m.Method, m.Params)
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = fmt.Errorf("language server connection closed: %w", err)
	for id, ch := range c.pending {
		ch <- response{err: c.closed}
		delete(c.pending, id)
	}
}

func (c *conn) answer(m message) {
	var (
		result any
		err    error
	)
	if c.onRequest != nil {
		result, err = c.onRequest(m.Method, m.Params)
	}
	reply := message{ID: m.ID, Result: mustJSON(result)}
	if err != nil {
		reply.Result = nil
		reply.Error = &rpcError{Code: -32601, Message: err.Error()}
	}
	_ = c.send(reply)
}

func readMessage(tp *textproto.Reader, br *bufio.Reader) (message, error) {
	header, err := tp.ReadMIMEHeader()
	if err != nil {
		return message{}, err
	}
	n, err := strconv.Atoi(header.Get("Content-Length"))
	if err != nil {
		return message{}, fmt.Errorf("bad Content-Length: %w", err)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(br, body); err != nil {
		return message{}, err
	}
	var m message
	return m, json.Unmarshal(body, &m)
}

func mustJSON(v any) json.RawMessage {
	if v == nil {
		return json.RawMessage("null")
	}
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
