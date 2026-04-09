package stripehelper

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"

	"github.com/stripe/stripe-go/v85"
)

type EventHandler func(w *WebhookContext)

type IStripeHelper interface {
	GetPrice(ctx context.Context, priceID string) (*stripe.Price, error)
	GetPrices(ctx context.Context, priceType string) []*stripe.Price
	GetPricesMap(ctx context.Context, priceType string) map[string]StripePricesResponse
	GetCustomer(ctx context.Context, id string, params *stripe.CustomerRetrieveParams) (*stripe.Customer, error)
	DeleteCustomer(ctx context.Context, id string, params *stripe.CustomerDeleteParams) error
	CreateCustomerWithEmail(ctx context.Context, email string, metadata map[string]string) (*stripe.Customer, error)
	CreateCustomer(ctx context.Context, params *stripe.CustomerCreateParams) (*stripe.Customer, error)
	GetSession(ctx context.Context, sessionID string) (*stripe.CheckoutSession, error)
	GetCustomerSubscriptions(ctx context.Context, customerId string) ([]*stripe.Subscription, error)
	GetCustomerSubscriptionsMap(ctx context.Context, customerId string) (map[string]*stripe.Subscription, error)
	AddEventHandler(eventType stripe.EventType, handlers ...EventHandler)
	Webhook(w http.ResponseWriter, req *http.Request)
}

type StripeHelper struct {
	webhookSecret string
	key           string
	handlers      map[stripe.EventType][]EventHandler
	sc            *stripe.Client
}

func NewStripeHelper(key, webhookSecret string) *StripeHelper {
	return &StripeHelper{
		sc:            stripe.NewClient(key),
		key:           key,
		webhookSecret: webhookSecret,
		handlers:      make(map[stripe.EventType][]EventHandler),
	}
}

// GetPrice returns the price details.
func (s *StripeHelper) GetPrice(ctx context.Context, priceID string) (*stripe.Price, error) {
	return s.sc.V1Prices.Retrieve(ctx, priceID, &stripe.PriceRetrieveParams{})
}

func (s *StripeHelper) GetCustomer(ctx context.Context, id string, params *stripe.CustomerRetrieveParams) (*stripe.Customer, error) {
	if params == nil {
		params = &stripe.CustomerRetrieveParams{}
	}
	return s.sc.V1Customers.Retrieve(ctx, id, params)
}

func (s *StripeHelper) DeleteCustomer(ctx context.Context, id string, params *stripe.CustomerDeleteParams) error {
	if params == nil {
		params = &stripe.CustomerDeleteParams{}
	}
	_, err := s.sc.V1Customers.Delete(ctx, id, params)
	return err
}

// CreateCustomerWithEmail creates a new customer with email and metadata.
func (s *StripeHelper) CreateCustomerWithEmail(ctx context.Context, email string, metadata map[string]string) (*stripe.Customer, error) {
	return s.CreateCustomer(ctx, &stripe.CustomerCreateParams{
		Email:    stripe.String(email),
		Metadata: metadata,
	})
}

// CreateCustomer creates a new customer.
func (s *StripeHelper) CreateCustomer(ctx context.Context, params *stripe.CustomerCreateParams) (*stripe.Customer, error) {
	if params == nil {
		params = &stripe.CustomerCreateParams{}
	}
	return s.sc.V1Customers.Create(ctx, params)
}

// GetSession returns the details of the checkout session.
func (s *StripeHelper) GetSession(ctx context.Context, sessionID string) (*stripe.CheckoutSession, error) {
	return s.sc.V1CheckoutSessions.Retrieve(ctx, sessionID, &stripe.CheckoutSessionRetrieveParams{})
}

func (s *StripeHelper) GetCustomerSubscriptions(ctx context.Context, customerId string) ([]*stripe.Subscription, error) {
	subs := make([]*stripe.Subscription, 0)

	params := &stripe.SubscriptionListParams{
		Customer: stripe.String(customerId),
		Status:   stripe.String("active"),
	}

	for sub, err := range s.sc.V1Subscriptions.List(ctx, params).All(ctx) {
		if err != nil {
			return subs, fmt.Errorf("failed to list subscriptions: %v", err)
		}
		subs = append(subs, sub)
	}
	return subs, nil
}

func (s *StripeHelper) GetCustomerSubscriptionsMap(ctx context.Context, customerId string) (map[string]*stripe.Subscription, error) {
	subs := make(map[string]*stripe.Subscription)

	params := &stripe.SubscriptionListParams{
		Customer: stripe.String(customerId),
		Status:   stripe.String("active"),
	}

	for sub, err := range s.sc.V1Subscriptions.List(ctx, params).All(ctx) {
		if err != nil {
			return subs, fmt.Errorf("failed to list subscriptions: %v", err)
		}
		subs[sub.ID] = sub
	}
	return subs, nil
}

func (s *StripeHelper) AddEventHandler(eventType stripe.EventType, handlers ...EventHandler) {
	_, ok := s.handlers[eventType]
	if !ok {
		s.handlers[eventType] = []EventHandler{}
	}
	for _, handler := range handlers {
		s.handlers[eventType] = append(s.handlers[eventType], handler)
	}
}

func (s *StripeHelper) Webhook(w http.ResponseWriter, req *http.Request) {
	const MaxBodyBytes = int64(65536)
	req.Body = http.MaxBytesReader(w, req.Body, MaxBodyBytes)
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		log.Printf("Error reading request body: %v\n", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	event, err := s.sc.ConstructEvent(payload, req.Header.Get("Stripe-Signature"), s.webhookSecret)
	if err != nil {
		slog.Error("Webhook signature verification failed.", slog.Any("error", err))
		w.WriteHeader(http.StatusBadRequest) // Return a 400 error on a bad signature
		return
	}

	// Unmarshal the event data into an appropriate struct depending on its Type
	handlerList, ok := s.handlers[event.Type]
	if !ok {
		slog.Error("Unhandled event type:", slog.Any("type", event.Type))
		w.WriteHeader(http.StatusOK)
		return
	}
	if len(handlerList) == 0 {
		slog.Error("Unhandled event type(no handlers):", slog.Any("type", event.Type))
		w.WriteHeader(http.StatusOK)
		return
	}
	webhookContext := WebhookContext{
		Request:  req,
		Writer:   w,
		Event:    event,
		handlers: handlerList,
	}
	webhookContext.Start()
}
