package cost

import "testing"

// A config override replaces the built-in price for a known model and adds an
// otherwise-unknown one, while leaving un-overridden models on their defaults.
func TestPricer_Overrides(t *testing.T) {
	p := NewPricer(map[string]ModelPrice{
		"gpt-4o":       {Input: 0.005, Output: 0.02}, // override built-in
		"acme-model-1": {Input: 0.01, Output: 0.03},  // new, not in built-ins
	})

	// Overridden input price is applied.
	if got := p.EstimateCost("gpt-4o", 1000); got != 0.005 {
		t.Errorf("gpt-4o override input = %v, want 0.005", got)
	}
	// Split uses overridden input + output.
	if got := p.EstimateCostSplit("gpt-4o", 1000, 1000); got != 0.005+0.02 {
		t.Errorf("gpt-4o split = %v, want %v", got, 0.005+0.02)
	}
	// Newly added model is priced.
	if got := p.EstimateCostSplit("acme-model-1", 1000, 1000); got != 0.01+0.03 {
		t.Errorf("acme split = %v, want %v", got, 0.01+0.03)
	}
	// Un-overridden model keeps its built-in price.
	if got := p.EstimateCost("gpt-4o-mini", 1000); got != 0.00015 {
		t.Errorf("gpt-4o-mini = %v, want built-in 0.00015", got)
	}
}

// SetPricingOverrides swaps the package-level pricer used by the free functions,
// and restoring nil returns to built-in defaults.
func TestSetPricingOverrides(t *testing.T) {
	t.Cleanup(func() { SetPricingOverrides(nil) })

	SetPricingOverrides(map[string]ModelPrice{
		"gpt-4o": {Input: 0.05, Output: 0.1},
	})
	if got := EstimateCost("gpt-4o", 1000); got != 0.05 {
		t.Errorf("after override EstimateCost = %v, want 0.05", got)
	}

	SetPricingOverrides(nil)
	if got := EstimateCost("gpt-4o", 1000); got != 0.0025 {
		t.Errorf("after reset EstimateCost = %v, want built-in 0.0025", got)
	}
}
