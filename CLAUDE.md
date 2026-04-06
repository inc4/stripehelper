# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Go library package (`package stripehelper`) that wraps the Stripe API (stripe-go v85) to provide helper methods for common Stripe operations: prices, checkout sessions, customers, and webhooks.

## Build & Test

```bash
go build ./...       # build
go vet ./...         # lint
go test ./...        # run all tests
go test -run TestX   # run a single test
```

## Architecture

- **`StripeHelper`** (in `stripe_pay.go`) — central struct created via `NewStripeHelper(key, webhookSecret)`. All Stripe operations (prices, customers, sessions) and the webhook HTTP handler are methods on this struct. Event handlers are registered with `AddEventHandler(eventType, handler)`.
- **`WebhookContext`** (in `webhook.go`) — Gin-style middleware chain for webhook event handling. Handlers are `func(w *WebhookContext)` and can call `Next()` to continue the chain or `Abort()` to stop it. Typed data extractors (`DataCharge()`, `DataInvoice()`, `DataCheckoutSession()`, `DataSubscription()`, `DataPaymentIntent()`, `DataPaymentMethod()`) parse `Event.Data.Raw` into stripe structs.
- **`StripePricesResponse`** (in `get_prices.go`) — flattened DTO for price data. `GetPricesWrapped()` converts raw Stripe prices into this format.

## Conventions

- This is a library package, not a standalone binary — no `main.go`. Consumers import it and wire up the HTTP handlers.
