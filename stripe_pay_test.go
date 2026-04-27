package stripehelper

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stripe/stripe-go/v84"
)

func newTestHelper(serverURL string) *StripeHelper {
	backends := stripe.NewBackendsWithConfig(&stripe.BackendConfig{
		URL: stripe.String(serverURL),
	})
	sc := stripe.NewClient("sk_test_123", stripe.WithBackends(backends))
	return &StripeHelper{
		sc:            sc,
		key:           "sk_test_123",
		webhookSecret: "whsec_test",
		handlers:      make(map[stripe.EventType][]EventHandler),
	}
}

func TestNewStripeHelper(t *testing.T) {
	sh := NewStripeHelper("sk_test_123", "whsec_456")

	if sh.key != "sk_test_123" {
		t.Fatalf("expected key %q, got %q", "sk_test_123", sh.key)
	}
	if sh.webhookSecret != "whsec_456" {
		t.Fatalf("expected webhookSecret %q, got %q", "whsec_456", sh.webhookSecret)
	}
	if sh.handlers == nil {
		t.Fatal("expected handlers map to be initialized")
	}
	if len(sh.handlers) != 0 {
		t.Fatalf("expected empty handlers map, got %d entries", len(sh.handlers))
	}
}

func TestAddEventHandler(t *testing.T) {
	t.Run("adds first handler for event type", func(t *testing.T) {
		sh := NewStripeHelper("key", "secret")
		handler := func(c *WebhookContext) {}
		sh.AddEventHandler("checkout.session.completed", handler)

		handlers := sh.handlers["checkout.session.completed"]
		if len(handlers) != 1 {
			t.Fatalf("expected 1 handler, got %d", len(handlers))
		}
	})

	t.Run("appends multiple handlers for same event type", func(t *testing.T) {
		sh := NewStripeHelper("key", "secret")
		sh.AddEventHandler("invoice.paid", func(c *WebhookContext) {})
		sh.AddEventHandler("invoice.paid", func(c *WebhookContext) {})
		sh.AddEventHandler("invoice.paid", func(c *WebhookContext) {})

		handlers := sh.handlers["invoice.paid"]
		if len(handlers) != 3 {
			t.Fatalf("expected 3 handlers, got %d", len(handlers))
		}
	})

	t.Run("different event types are independent", func(t *testing.T) {
		sh := NewStripeHelper("key", "secret")
		sh.AddEventHandler("invoice.paid", func(c *WebhookContext) {})
		sh.AddEventHandler("charge.succeeded", func(c *WebhookContext) {})

		if len(sh.handlers["invoice.paid"]) != 1 {
			t.Fatal("expected 1 handler for invoice.paid")
		}
		if len(sh.handlers["charge.succeeded"]) != 1 {
			t.Fatal("expected 1 handler for charge.succeeded")
		}
	})
}

func TestWebhook(t *testing.T) {
	t.Run("returns 400 for invalid JSON body", func(t *testing.T) {
		sh := NewStripeHelper("key", "whsec_test")
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader("not json"))

		sh.Webhook(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", rec.Code)
		}
	})

	t.Run("returns 400 for invalid signature", func(t *testing.T) {
		sh := NewStripeHelper("key", "whsec_test")
		event := stripe.Event{
			ID:   "evt_123",
			Type: "checkout.session.completed",
		}
		body, _ := json.Marshal(event)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
		req.Header.Set("Stripe-Signature", "invalid_signature")

		sh.Webhook(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", rec.Code)
		}
	})

	t.Run("returns 503 for oversized body", func(t *testing.T) {
		sh := NewStripeHelper("key", "whsec_test")
		// Create a body larger than MaxBodyBytes (65536)
		largeBody := make([]byte, 70000)
		for i := range largeBody {
			largeBody[i] = 'a'
		}

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(largeBody))

		sh.Webhook(rec, req)

		// MaxBytesReader causes ReadAll to fail, returning 503
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected status 503, got %d", rec.Code)
		}
	})
}

func TestDataExtractors(t *testing.T) {
	t.Run("DataCheckoutSession parses valid data", func(t *testing.T) {
		raw, _ := json.Marshal(map[string]any{
			"id":       "cs_test_123",
			"customer": "cus_456",
		})
		ctx := &WebhookContext{
			Event: stripe.Event{
				Data: &stripe.EventData{Raw: raw},
			},
		}

		session, err := ctx.DataCheckoutSession()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if session.ID != "cs_test_123" {
			t.Fatalf("expected ID %q, got %q", "cs_test_123", session.ID)
		}
	})

	t.Run("DataInvoice parses valid data", func(t *testing.T) {
		raw, _ := json.Marshal(map[string]any{
			"id":     "in_test_789",
			"status": "paid",
		})
		ctx := &WebhookContext{
			Event: stripe.Event{
				Data: &stripe.EventData{Raw: raw},
			},
		}

		invoice, err := ctx.DataInvoice()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if invoice.ID != "in_test_789" {
			t.Fatalf("expected ID %q, got %q", "in_test_789", invoice.ID)
		}
	})

	t.Run("DataCharge parses valid data", func(t *testing.T) {
		raw, _ := json.Marshal(map[string]any{
			"id":     "ch_test_abc",
			"amount": 5000,
		})
		ctx := &WebhookContext{
			Event: stripe.Event{
				Data: &stripe.EventData{Raw: raw},
			},
		}

		charge, err := ctx.DataCharge()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if charge.ID != "ch_test_abc" {
			t.Fatalf("expected ID %q, got %q", "ch_test_abc", charge.ID)
		}
		if charge.Amount != 5000 {
			t.Fatalf("expected amount 5000, got %d", charge.Amount)
		}
	})

	t.Run("DataSubscription parses valid data", func(t *testing.T) {
		raw, _ := json.Marshal(map[string]any{
			"id":     "sub_test_def",
			"status": "active",
		})
		ctx := &WebhookContext{
			Event: stripe.Event{
				Data: &stripe.EventData{Raw: raw},
			},
		}

		sub, err := ctx.DataSubscription()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sub.ID != "sub_test_def" {
			t.Fatalf("expected ID %q, got %q", "sub_test_def", sub.ID)
		}
	})

	t.Run("DataPaymentIntent parses valid data", func(t *testing.T) {
		raw, _ := json.Marshal(map[string]any{
			"id":     "pi_test_ghi",
			"amount": 2000,
		})
		ctx := &WebhookContext{
			Event: stripe.Event{
				Data: &stripe.EventData{Raw: raw},
			},
		}

		pi, err := ctx.DataPaymentIntent()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pi.ID != "pi_test_ghi" {
			t.Fatalf("expected ID %q, got %q", "pi_test_ghi", pi.ID)
		}
		if pi.Amount != 2000 {
			t.Fatalf("expected amount 2000, got %d", pi.Amount)
		}
	})

	t.Run("DataPaymentMethod parses valid data", func(t *testing.T) {
		raw, _ := json.Marshal(map[string]any{
			"id":   "pm_test_jkl",
			"type": "card",
		})
		ctx := &WebhookContext{
			Event: stripe.Event{
				Data: &stripe.EventData{Raw: raw},
			},
		}

		pm, err := ctx.DataPaymentMethod()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if pm.ID != "pm_test_jkl" {
			t.Fatalf("expected ID %q, got %q", "pm_test_jkl", pm.ID)
		}
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		ctx := &WebhookContext{
			Event: stripe.Event{
				Data: &stripe.EventData{Raw: json.RawMessage(`{invalid}`)},
			},
		}

		_, err := ctx.DataCharge()
		if err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}

		_, err = ctx.DataInvoice()
		if err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}

		_, err = ctx.DataCheckoutSession()
		if err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}

		_, err = ctx.DataSubscription()
		if err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}

		_, err = ctx.DataPaymentIntent()
		if err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}

		_, err = ctx.DataPaymentMethod()
		if err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		}
	})

	t.Run("returns error for null raw data", func(t *testing.T) {
		ctx := &WebhookContext{
			Event: stripe.Event{
				Data: &stripe.EventData{Raw: nil},
			},
		}

		_, err := ctx.DataCharge()
		if err == nil {
			t.Fatal("expected error for nil raw data, got nil")
		}
	})
}

func TestGetCustomerSubscriptions(t *testing.T) {
	t.Run("returns subscriptions for customer", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/subscriptions" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if r.URL.Query().Get("customer") != "cus_123" {
				t.Fatalf("expected customer=cus_123, got %s", r.URL.Query().Get("customer"))
			}
			if r.URL.Query().Get("status") != "active" {
				t.Fatalf("expected status=active, got %s", r.URL.Query().Get("status"))
			}
			resp := map[string]any{
				"object":   "list",
				"has_more": false,
				"data": []map[string]any{
					{"id": "sub_1", "object": "subscription", "status": "active", "customer": "cus_123"},
					{"id": "sub_2", "object": "subscription", "status": "active", "customer": "cus_123"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		sh := newTestHelper(server.URL)
		subs, err := sh.GetCustomerSubscriptions(context.Background(), "cus_123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(subs) != 2 {
			t.Fatalf("expected 2 subscriptions, got %d", len(subs))
		}
		if subs[0].ID != "sub_1" {
			t.Fatalf("expected first sub ID %q, got %q", "sub_1", subs[0].ID)
		}
		if subs[1].ID != "sub_2" {
			t.Fatalf("expected second sub ID %q, got %q", "sub_2", subs[1].ID)
		}
	})

	t.Run("returns empty slice for customer with no subscriptions", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]any{
				"object":   "list",
				"has_more": false,
				"data":     []map[string]any{},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		sh := newTestHelper(server.URL)
		subs, err := sh.GetCustomerSubscriptions(context.Background(), "cus_empty")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(subs) != 0 {
			t.Fatalf("expected 0 subscriptions, got %d", len(subs))
		}
	})

	t.Run("returns error on API failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			resp := map[string]any{
				"error": map[string]any{
					"type":    "api_error",
					"message": "internal server error",
				},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		sh := newTestHelper(server.URL)
		_, err := sh.GetCustomerSubscriptions(context.Background(), "cus_fail")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestIsAborted(t *testing.T) {
	t.Run("returns false when not aborted", func(t *testing.T) {
		ctx := &WebhookContext{}
		if ctx.IsAborted() {
			t.Fatal("expected IsAborted to return false for fresh context")
		}
	})

	t.Run("returns true after Abort", func(t *testing.T) {
		ctx := &WebhookContext{}
		ctx.Abort()
		if !ctx.IsAborted() {
			t.Fatal("expected IsAborted to return true after Abort")
		}
	})
}
