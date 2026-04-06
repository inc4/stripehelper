# stripehelper

Go library that wraps [stripe-go v85](https://github.com/stripe/stripe-go) to provide helper methods for common Stripe operations: prices, checkout sessions, customers, and webhooks.

## Installation

```bash
go get github.com/inc4/stripehelper
```

## Usage

### Initialize

```go
sh := stripehelper.NewStripeHelper("sk_test_...", "whsec_...")
```

### Customers

```go
// Create a customer
cust, err := sh.CreateCustomerWithEmail("user@example.com", map[string]string{
    "plan": "pro",
})

// Get a customer
cust, err := sh.GetCustomer("cus_123", nil)

// Get a customer with expanded fields
cust, err := sh.GetCustomer("cus_123", &stripe.CustomerParams{
    Params: stripe.Params{
        Expand: []*string{stripe.String("subscriptions")},
    },
})

// Delete a customer
err := sh.DeleteCustomer("cus_123", nil)
```

### Prices

```go
// Get a single price
p, err := sh.GetPrice("price_123")

// List all recurring prices (raw Stripe objects)
prices := sh.GetPrices("recurring")

// List prices as flattened DTOs (defaults to "recurring" if empty)
wrapped := sh.GetPricesWrapped("")
for _, p := range wrapped {
    fmt.Printf("%s — %s: %d cents/%s\n", p.ProductID, p.Name, p.UnitAmount, p.Interval)
}
```

### Checkout Sessions

```go
sess, err := sh.GetSession("cs_test_123")
```

### Webhooks

Register event handlers and wire the webhook endpoint to your HTTP server. Handlers follow a Gin-style middleware chain — call `Next()` to continue or `Abort()` to stop.

```go
sh := stripehelper.NewStripeHelper("sk_test_...", "whsec_...")

// Logging middleware
sh.AddEventHandler("checkout.session.completed", func(c *stripehelper.WebhookContext) {
    log.Println("event received:", c.Event.Type)
    c.Next()
})

// Business logic handler
sh.AddEventHandler("checkout.session.completed", func(c *stripehelper.WebhookContext) {
    session, err := c.DataCheckoutSession()
    if err != nil {
        c.Writer.WriteHeader(http.StatusBadRequest)
        c.Abort()
        return
    }
    log.Printf("Checkout completed: %s (customer: %s)\n", session.ID, session.Customer.ID)
    c.Writer.WriteHeader(http.StatusOK)
})

// Mount the webhook handler
http.HandleFunc("/webhook", sh.Webhook)
log.Fatal(http.ListenAndServe(":8080", nil))
```

#### Available data extractors on `WebhookContext`

| Method | Returns |
|---|---|
| `DataCharge()` | `stripe.Charge` |
| `DataInvoice()` | `stripe.Invoice` |
| `DataCheckoutSession()` | `stripe.CheckoutSession` |
| `DataSubscription()` | `stripe.Subscription` |
| `DataPaymentIntent()` | `stripe.PaymentIntent` |
| `DataPaymentMethod()` | `stripe.PaymentMethod` |

## License

MIT
