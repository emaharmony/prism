package provider

import (
	"net/http"
	"time"
)

// DefaultTransport is a shared HTTP transport with connection pooling.
// Reusing connections across providers reduces latency and resource usage.
// All providers should use this transport instead of creating their own.
var DefaultTransport = &http.Transport{
	MaxIdleConns:        100,
	MaxIdleConnsPerHost: 20,
	IdleConnTimeout:     90 * time.Second,
}
