package stripehelper

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stripe/stripe-go/v85"
)

func TestGetRawData(t *testing.T) {
	t.Run("reads body successfully", func(t *testing.T) {
		body := []byte(`{"id": "evt_123"}`)
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
		ctx := &WebhookContext{Request: req}

		data, err := ctx.GetRawData()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(data) != string(body) {
			t.Fatalf("got %q, want %q", data, body)
		}
	})

	t.Run("returns error for nil body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
		req.Body = nil
		ctx := &WebhookContext{Request: req}

		_, err := ctx.GetRawData()
		if err == nil {
			t.Fatal("expected error for nil body, got nil")
		}
		if err.Error() != "cannot read nil body" {
			t.Fatalf("got error %q, want %q", err.Error(), "cannot read nil body")
		}
	})

	t.Run("returns error for already read body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader([]byte("data")))
		req.Body = io.NopCloser(&errorReader{})
		ctx := &WebhookContext{Request: req}

		_, err := ctx.GetRawData()
		if err == nil {
			t.Fatal("expected error from failing reader, got nil")
		}
	})
}

type errorReader struct{}

func (e *errorReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func TestStart(t *testing.T) {
	t.Run("calls first handler", func(t *testing.T) {
		called := false
		handler := func(c *WebhookContext) {
			called = true
		}
		ctx := WebhookContext{
			Request:  httptest.NewRequest(http.MethodPost, "/", nil),
			Writer:   httptest.NewRecorder(),
			Event:    stripe.Event{Type: "test"},
			handlers: []EventHandler{handler},
		}
		ctx.Start()
		if !called {
			t.Fatal("expected first handler to be called")
		}
	})

	t.Run("sets index to zero before calling", func(t *testing.T) {
		var receivedIndex int8
		handler := func(c *WebhookContext) {
			receivedIndex = c.index
		}
		ctx := WebhookContext{
			Request:  httptest.NewRequest(http.MethodPost, "/", nil),
			Writer:   httptest.NewRecorder(),
			handlers: []EventHandler{handler},
			index:    5,
		}
		ctx.Start()
		if receivedIndex != 0 {
			t.Fatalf("expected index 0, got %d", receivedIndex)
		}
	})
}

func TestNext(t *testing.T) {
	t.Run("calls all handlers in order", func(t *testing.T) {
		var order []int
		h1 := func(c *WebhookContext) {
			order = append(order, 1)
			c.Next()
		}
		h2 := func(c *WebhookContext) {
			order = append(order, 2)
			c.Next()
		}
		h3 := func(c *WebhookContext) {
			order = append(order, 3)
		}

		ctx := WebhookContext{
			Request:  httptest.NewRequest(http.MethodPost, "/", nil),
			Writer:   httptest.NewRecorder(),
			handlers: []EventHandler{h1, h2, h3},
		}
		ctx.Start()
		expected := []int{1, 2, 3}
		if len(order) != len(expected) {
			t.Fatalf("expected %v, got %v", expected, order)
		}
		for i := range expected {
			if order[i] != expected[i] {
				t.Fatalf("expected %v, got %v", expected, order)
			}
		}
	})

	t.Run("not calling Next does not prevent parent loop from continuing", func(t *testing.T) {
		var order []int
		h1 := func(c *WebhookContext) {
			order = append(order, 1)
			c.Next()
		}
		h2 := func(c *WebhookContext) {
			order = append(order, 2)
			// not calling Next — but parent loop still runs h3
		}
		h3 := func(c *WebhookContext) {
			order = append(order, 3)
		}

		ctx := WebhookContext{
			Request:  httptest.NewRequest(http.MethodPost, "/", nil),
			Writer:   httptest.NewRecorder(),
			handlers: []EventHandler{h1, h2, h3},
		}
		ctx.Start()
		expected := []int{1, 2, 3}
		if len(order) != len(expected) {
			t.Fatalf("expected %v, got %v", expected, order)
		}
		for i := range expected {
			if order[i] != expected[i] {
				t.Fatalf("expected %v, got %v", expected, order)
			}
		}
	})

	t.Run("Abort stops the chain", func(t *testing.T) {
		var order []int
		h1 := func(c *WebhookContext) {
			order = append(order, 1)
			c.Next()
		}
		h2 := func(c *WebhookContext) {
			order = append(order, 2)
			c.Abort()
		}
		h3 := func(c *WebhookContext) {
			order = append(order, 3)
		}

		ctx := WebhookContext{
			Request:  httptest.NewRequest(http.MethodPost, "/", nil),
			Writer:   httptest.NewRecorder(),
			handlers: []EventHandler{h1, h2, h3},
		}
		ctx.Start()
		expected := []int{1, 2}
		if len(order) != len(expected) {
			t.Fatalf("expected %v, got %v", expected, order)
		}
		for i := range expected {
			if order[i] != expected[i] {
				t.Fatalf("expected %v, got %v", expected, order)
			}
		}
		if !ctx.IsAborted() {
			t.Fatal("expected context to be aborted")
		}
	})

	t.Run("skips nil handlers", func(t *testing.T) {
		var called bool
		h1 := func(c *WebhookContext) {
			c.Next()
		}
		h3 := func(c *WebhookContext) {
			called = true
		}

		ctx := WebhookContext{
			Request:  httptest.NewRequest(http.MethodPost, "/", nil),
			Writer:   httptest.NewRecorder(),
			handlers: []EventHandler{h1, nil, h3},
		}
		ctx.Start()
		if !called {
			t.Fatal("expected handler after nil to be called")
		}
	})

	t.Run("does nothing when no more handlers", func(t *testing.T) {
		callCount := 0
		h1 := func(c *WebhookContext) {
			callCount++
			c.Next()
		}

		ctx := WebhookContext{
			Request:  httptest.NewRequest(http.MethodPost, "/", nil),
			Writer:   httptest.NewRecorder(),
			handlers: []EventHandler{h1},
		}
		ctx.Start()
		if callCount != 1 {
			t.Fatalf("expected 1 call, got %d", callCount)
		}
	})

	t.Run("handler can access event and writer", func(t *testing.T) {
		rec := httptest.NewRecorder()
		evt := stripe.Event{Type: "checkout.session.completed"}

		var gotType stripe.EventType
		h := func(c *WebhookContext) {
			gotType = c.Event.Type
			c.Writer.WriteHeader(http.StatusOK)
		}

		ctx := WebhookContext{
			Request:  httptest.NewRequest(http.MethodPost, "/", nil),
			Writer:   rec,
			Event:    evt,
			handlers: []EventHandler{h},
		}
		ctx.Start()

		if gotType != "checkout.session.completed" {
			t.Fatalf("expected event type %q, got %q", "checkout.session.completed", gotType)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("each handler gets same context pointer", func(t *testing.T) {
		var ptrs []*WebhookContext
		h1 := func(c *WebhookContext) {
			ptrs = append(ptrs, c)
			c.Next()
		}
		h2 := func(c *WebhookContext) {
			ptrs = append(ptrs, c)
		}

		ctx := WebhookContext{
			Request:  httptest.NewRequest(http.MethodPost, "/", nil),
			Writer:   httptest.NewRecorder(),
			handlers: []EventHandler{h1, h2},
		}
		ctx.Start()

		if len(ptrs) != 2 {
			t.Fatalf("expected 2 pointers, got %d", len(ptrs))
		}
		if ptrs[0] != ptrs[1] {
			t.Fatal("expected both handlers to receive the same context pointer")
		}
	})
}
