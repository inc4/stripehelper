package stripehelper

import (
	"encoding/json"
	"io"
	"log"
	"log/slog"
	"net/http"

	"github.com/stripe/stripe-go/v85"
	"github.com/stripe/stripe-go/v85/checkout/session"
	"github.com/stripe/stripe-go/v85/customer"
	"github.com/stripe/stripe-go/v85/price"
	"github.com/stripe/stripe-go/v85/webhook"
)

type EventHandler func(w *WebhookContext)

type IStripeHelper interface {
	GetPrice(priceID string) (*stripe.Price, error)
	GetPrices(priceType string) []*stripe.Price
	GetPricesMap(priceType string) map[string]StripePricesResponse
	GetCustomer(id string, params *stripe.CustomerParams) (*stripe.Customer, error)
	DeleteCustomer(id string, params *stripe.CustomerParams) error
	CreateCustomerWithEmail(email string, metadata map[string]string) (*stripe.Customer, error)
	CreateCustomer(params *stripe.CustomerParams) (*stripe.Customer, error)
	GetSession(sessionID string) (*stripe.CheckoutSession, error)
	AddEventHandler(eventType stripe.EventType, handlers ...EventHandler)
	Webhook(w http.ResponseWriter, req *http.Request)
}

type StripeHelper struct {
	webhookSecret string
	key           string
	handlers      map[stripe.EventType][]EventHandler
}

func NewStripeHelper(key, webhookSecret string) *StripeHelper {
	stripe.Key = key
	return &StripeHelper{
		key:           key,
		webhookSecret: webhookSecret,
		handlers:      make(map[stripe.EventType][]EventHandler),
	}
}

// GetPrice returns the price details.
func (a *StripeHelper) GetPrice(priceID string) (*stripe.Price, error) {
	return price.Get(priceID, &stripe.PriceParams{})
}

func (s *StripeHelper) GetCustomer(id string, params *stripe.CustomerParams) (*stripe.Customer, error) {
	if params == nil {
		params = &stripe.CustomerParams{}
	}
	return customer.Get(id, params)
}

func (s *StripeHelper) DeleteCustomer(id string, params *stripe.CustomerParams) error {
	if params == nil {
		params = &stripe.CustomerParams{}
	}
	_, err := customer.Del(id, params)
	return err
}

// CreateCustomer creates a new customer.
func (s *StripeHelper) CreateCustomerWithEmail(email string, metadata map[string]string) (*stripe.Customer, error) {
	return s.CreateCustomer(&stripe.CustomerParams{
		Email:    stripe.String(email),
		Metadata: metadata,
	})
}

// CreateCustomer creates a new customer.
func (s *StripeHelper) CreateCustomer(params *stripe.CustomerParams) (*stripe.Customer, error) {
	if params == nil {
		params = &stripe.CustomerParams{}
	}
	return customer.New(params)
}

// GetSession returns the details of the checkout session.
func (s *StripeHelper) GetSession(sessionID string) (*stripe.CheckoutSession, error) {
	return session.Get(sessionID, &stripe.CheckoutSessionParams{})
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

	event := stripe.Event{}

	if err := json.Unmarshal(payload, &event); err != nil {
		slog.Error("Failed to parse webhook body json", slog.Any("error", err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	event, err = webhook.ConstructEvent(payload, req.Header.Get("Stripe-Signature"), s.webhookSecret)
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
