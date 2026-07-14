package cost

import (
	"math"
	"testing"
)

func TestUsageCostAndAccumulator(t *testing.T) {
	pricing := Pricing{InputPer1M: 2, OutputPer1M: 8}
	usage := Usage{InputTokens: 500_000, OutputTokens: 250_000}
	if got, want := usage.Cost(pricing), 3.0; math.Abs(got-want) > 1e-9 {
		t.Fatalf("Cost() = %v, want %v", got, want)
	}

	var accumulator Accumulator
	accumulator.Add(usage, pricing)
	accumulator.Add(Usage{InputTokens: 100, OutputTokens: 200}, pricing)
	total, totalCost := accumulator.Snapshot()
	if total.InputTokens != 500_100 || total.OutputTokens != 250_200 {
		t.Fatalf("usage = %#v", total)
	}
	if totalCost <= 3 {
		t.Fatalf("cost = %v", totalCost)
	}
}
