package stripehelper

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/stripe/stripe-go/v84"
)

type WebhookContext struct {
	Request  *http.Request
	Writer   http.ResponseWriter
	Event    stripe.Event
	handlers []EventHandler
	index    int8
}

// Next advances the handler chain by calling subsequent handlers in order.
// It shares the index across all handlers via pointer receiver, following
// the same middleware pattern as Gin's Context.Next.
//
// Call Next inside a handler to pass control to the next handler in the chain.
// Code after Next runs after all deeper handlers have returned, enabling
// pre/post processing patterns.
//
// If a handler does not call Next, the parent caller's loop still continues
// to the remaining handlers. To fully stop the chain, use [WebhookContext.Abort].
//
// Usage:
//
//	// Logging middleware — runs before and after subsequent handlers.
//	func loggingMiddleware(c *stripehelper.WebhookContext) {
//	    log.Println("before:", c.Event.Type)
//	    c.Next()
//	    log.Println("after:", c.Event.Type)
//	}
//
//	// Auth middleware — aborts the chain on failure.
//	func authMiddleware(c *stripehelper.WebhookContext) {
//	    if !isAuthorized(c.Request) {
//	        c.Writer.WriteHeader(http.StatusUnauthorized)
//	        c.Abort()
//	        return
//	    }
//	    c.Next()
//	}
//
//	// Terminal handler — no need to call Next.
//	func handleCheckout(c *stripehelper.WebhookContext) {
//	    // process event...
//	    c.Writer.WriteHeader(http.StatusOK)
//	}
func (c *WebhookContext) Next() {
	c.index++
	for c.index < int8(len(c.handlers)) {
		if c.handlers[c.index] != nil {
			c.handlers[c.index](c)
		}
		c.index++
	}
}

const abortIndex int8 = 63

// Abort stops the handler chain. Remaining handlers will not be executed.
func (c *WebhookContext) Abort() {
	c.index = abortIndex
}

// IsAborted returns true if the handler chain has been aborted.
func (c *WebhookContext) IsAborted() bool {
	return c.index >= abortIndex
}

func (c *WebhookContext) Start() {
	c.index = 0
	c.handlers[c.index](c)
}

// GetRawData returns stream data.
func (c *WebhookContext) GetRawData() ([]byte, error) {
	if c.Request.Body == nil {
		return nil, errors.New("cannot read nil body")
	}
	return io.ReadAll(c.Request.Body)
}

func (c *WebhookContext) parseData(body any) error {
	err := json.Unmarshal(c.Event.Data.Raw, body)
	if err != nil {
		return fmt.Errorf("Error parsing webhook JSON: %v\n", err)
	}
	return nil
}

func (c *WebhookContext) DataCharge() (data stripe.Charge, err error) {
	err = c.parseData(&data)
	return data, err
}

func (c *WebhookContext) DataInvoice() (data stripe.Invoice, err error) {
	err = c.parseData(&data)
	return data, err
}

func (c *WebhookContext) DataCheckoutSession() (data stripe.CheckoutSession, err error) {
	err = c.parseData(&data)
	return data, err
}

func (c *WebhookContext) DataSubscription() (data stripe.Subscription, err error) {
	err = c.parseData(&data)
	return data, err
}

func (c *WebhookContext) DataPaymentIntent() (data stripe.PaymentIntent, err error) {
	err = c.parseData(&data)
	return data, err
}

func (c *WebhookContext) DataPaymentMethod() (data stripe.PaymentMethod, err error) {
	err = c.parseData(&data)
	return data, err
}
