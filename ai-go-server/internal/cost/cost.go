// Package cost tracks model usage independently of any provider wire format.
package cost

import "sync"

// Pricing is expressed in currency units per one million tokens.
type Pricing struct {
	InputPer1M  float64
	OutputPer1M float64
}

// Usage contains normalized token counts from one model response.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Cost calculates the price of this usage under pricing.
func (u Usage) Cost(pricing Pricing) float64 {
	return float64(u.InputTokens)/1e6*pricing.InputPer1M +
		float64(u.OutputTokens)/1e6*pricing.OutputPer1M
}

// Accumulator safely totals usage and cost over a conversation or workflow.
type Accumulator struct {
	mu    sync.Mutex
	usage Usage
	cost  float64
}

// Add records one provider response using the provider/model price in effect.
func (a *Accumulator) Add(usage Usage, pricing Pricing) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.usage.InputTokens += usage.InputTokens
	a.usage.OutputTokens += usage.OutputTokens
	a.cost += usage.Cost(pricing)
}

// Snapshot returns a consistent copy of the accumulated values.
func (a *Accumulator) Snapshot() (Usage, float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.usage, a.cost
}
