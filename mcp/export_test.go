package mcp

import "time"

// SetHTTPTimeout shortens the HTTP timeout for a test; call the result to restore it.
func SetHTTPTimeout(d time.Duration) (restore func()) {
	old := httpTimeout
	httpTimeout = d
	return func() { httpTimeout = old }
}
