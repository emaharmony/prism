package cost

// PricingTable provides estimated costs per 1K tokens for known models.
// All prices are in USD per 1,000 tokens (input pricing).
// These are approximate and should be updated as provider pricing changes.
var PricingTable = map[string]float64{
	// OpenAI
	"gpt-4o":                0.0025,
	"gpt-4o-mini":           0.00015,
	"gpt-4-turbo":           0.01,
	"gpt-4":                  0.03,
	"gpt-3.5-turbo":         0.0005,

	// Anthropic
	"claude-sonnet-4-20250514": 0.003,
	"claude-3-5-sonnet-20241022": 0.003,
	"claude-3-haiku-20240307":   0.00025,

	// Gemini
	"gemini-2.0-flash": 0.0001,
	"gemini-1.5-pro":    0.00125,
	"gemini-1.5-flash":  0.000075,

	// Ollama (local — free)
	"llama3":     0.0,
	"llama3.1":   0.0,
	"mistral":    0.0,
	"codellama":  0.0,
	"qwen2":      0.0,
	"gemma2":     0.0,
	"phi3":       0.0,
	"deepseek-v2": 0.0,
	"glm-4":      0.0,
}

// EstimateCost calculates the estimated cost for a given model and token count.
// Returns 0.0 if the model is not in the pricing table (likely local/free).
func EstimateCost(model string, totalTokens int) float64 {
	pricePer1K, ok := PricingTable[model]
	if !ok {
		return 0.0 // Unknown model, assume free/local
	}
	return float64(totalTokens) / 1000.0 * pricePer1K
}