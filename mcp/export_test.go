package mcp

import (
	"io"
	"time"
)

// SetHelpListAllMax changes the size limit for listing every tool in
// help("ns"); call the result to restore it.
func SetHelpListAllMax(n int) (restore func()) {
	old := helpListAllMax
	helpListAllMax = n
	return func() { helpListAllMax = old }
}

// SetHTTPTimeout shortens the HTTP timeout for a test; call the result to restore it.
func SetHTTPTimeout(d time.Duration) (restore func()) {
	old := httpTimeout
	httpTimeout = d
	return func() { httpTimeout = old }
}

// SetInternalErrorLog redirects internal-error reports; call the result to restore it.
func SetInternalErrorLog(w io.Writer) (restore func()) {
	old := internalErrorLog
	internalErrorLog = w
	return func() { internalErrorLog = old }
}
