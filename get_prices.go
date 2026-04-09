package stripehelper

import (
	"context"

	"github.com/stripe/stripe-go/v85"
)

// GetPrices returns a list of prices.
func (s *StripeHelper) GetPrices(ctx context.Context, priceType string) []*stripe.Price {
	prices := make([]*stripe.Price, 0)

	for p, err := range s.sc.V1Prices.List(ctx, &stripe.PriceListParams{
		Type: stripe.String(priceType),
	}).All(ctx) {
		if err != nil {
			break
		}
		prices = append(prices, p)
	}

	return prices
}

type StripePricesResponse struct {
	PriceID         string            `json:"price_id"`
	UnitAmount      int64             `json:"unit_amount"`
	Active          bool              `json:"active"`
	ProductID       string            `json:"id"`
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	Interval        string            `json:"interval"`
	IntervalCount   int64             `json:"interval_count"`
	TrialPeriodDays int64             `json:"trial_period_days"`
	Type            string            `json:"type"`
	Product         *stripe.Product   `json:"product,omitempty"`
	Metadata        map[string]string `json:"metadata"`
}

func (s *StripeHelper) GetPricesMap(ctx context.Context, priceType string) map[string]StripePricesResponse {
	if priceType == "" {
		priceType = "recurring"
	}
	prices := s.GetPrices(ctx, priceType)

	pricesResponse := make(map[string]StripePricesResponse, len(prices))
	for _, p := range prices {
		resp := StripePricesResponse{
			PriceID:     p.ID,
			UnitAmount:  p.UnitAmount,
			Active:      p.Active,
			ProductID:   p.Product.ID,
			Name:        p.Product.Name,
			Description: p.Product.Description,
			Product:     p.Product,
			Type:        string(p.Type),
			Metadata:    p.Metadata,
		}
		if p.Recurring != nil {
			resp.Interval = string(p.Recurring.Interval)
			resp.IntervalCount = p.Recurring.IntervalCount
			resp.TrialPeriodDays = p.Recurring.TrialPeriodDays
		}
		pricesResponse[p.ID] = resp
	}
	return pricesResponse
}
