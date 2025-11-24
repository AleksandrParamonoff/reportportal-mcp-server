package integration

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// ReportPortalMockServer is a mock HTTP server that simulates ReportPortal API
type ReportPortalMockServer struct {
	server       *httptest.Server
	requestPairs []RequestResponsePair
	mutex        sync.RWMutex
	requestLog   []MockRequestLog
}

// MockRequestLog logs incoming requests for debugging
type MockRequestLog struct {
	Method      string
	Path        string
	QueryParams map[string][]string
	Headers     map[string]string
	Body        string
}

// NewReportPortalMockServer creates a new ReportPortal mock server
func NewReportPortalMockServer(requestPairs []RequestResponsePair) *ReportPortalMockServer {
	mock := &ReportPortalMockServer{
		requestPairs: requestPairs,
		requestLog:   make([]MockRequestLog, 0),
	}

	mock.server = httptest.NewServer(http.HandlerFunc(mock.handleRequest))

	return mock
}

// URL returns the base URL of the mock server
func (m *ReportPortalMockServer) URL() string {
	return m.server.URL
}

// Close shuts down the mock server
func (m *ReportPortalMockServer) Close() {
	m.server.Close()
}

// GetRequestLog returns the log of all requests received
func (m *ReportPortalMockServer) GetRequestLog() []MockRequestLog {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.requestLog
}

// ClearRequestLog clears the request log
func (m *ReportPortalMockServer) ClearRequestLog() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.requestLog = make([]MockRequestLog, 0)
}

// handleRequest handles incoming HTTP requests and matches them to predefined responses
func (m *ReportPortalMockServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	// Read request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()

	// Log the request
	m.logRequest(r, string(bodyBytes))

	// Find matching request/response pair
	m.mutex.RLock()
	pairs := m.requestPairs
	m.mutex.RUnlock()

	for i, pair := range pairs {
		slog.Debug(
			"Trying to match request",
			"pairIndex",
			i,
			"method",
			r.Method,
			"path",
			r.URL.Path,
			"expectedMethod",
			pair.Request.Method,
			"expectedPath",
			m.buildPath(pair.Request.URL),
		)
		if m.matchesRequest(r, string(bodyBytes), pair.Request) {
			slog.Debug("Request matched", "pairIndex", i)
			// Write response headers
			for _, header := range pair.Response.Header {
				if !header.Disabled {
					w.Header().Set(header.Key, header.Value)
				}
			}

			// Set status code
			statusCode := pair.Response.Code
			if statusCode == 0 {
				statusCode = http.StatusOK
			}
			w.WriteHeader(statusCode)

			// Write response body
			if pair.Response.Body != "" {
				_, err := w.Write([]byte(pair.Response.Body))
				if err != nil {
					slog.Error("Failed to write response body", "error", err)
				}
			}

			return
		}
		slog.Debug("Request did not match", "pairIndex", i)
	}

	// No matching request found
	slog.Warn(
		"No matching request found",
		"method",
		r.Method,
		"path",
		r.URL.Path,
		"query",
		r.URL.RawQuery,
	)
	http.Error(w, "No matching mock response found", http.StatusNotFound)
}

// logRequest logs the incoming request
func (m *ReportPortalMockServer) logRequest(r *http.Request, body string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	headers := make(map[string]string)
	for key, values := range r.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	queryParams := make(map[string][]string)
	for key, values := range r.URL.Query() {
		queryParams[key] = values
	}

	m.requestLog = append(m.requestLog, MockRequestLog{
		Method:      r.Method,
		Path:        r.URL.Path,
		QueryParams: queryParams,
		Headers:     headers,
		Body:        body,
	})
}

// matchesRequest checks if the incoming request matches the expected request
func (m *ReportPortalMockServer) matchesRequest(
	r *http.Request,
	body string,
	expected PostmanRequest,
) bool {
	// Check HTTP method
	if !strings.EqualFold(expected.Method, r.Method) {
		slog.Debug("Method mismatch", "expected", expected.Method, "got", r.Method)
		return false
	}

	// Check URL path
	expectedPath := m.buildPath(expected.URL)
	if expectedPath != r.URL.Path {
		slog.Debug("Path mismatch", "expected", expectedPath, "got", r.URL.Path)
		return false
	}

	// Check query parameters
	if !m.matchesQueryParams(r, expected.URL.Query) {
		return false
	}

	// Check headers
	if !m.matchesHeaders(r, expected.Header) {
		return false
	}

	// Check body (if present)
	if expected.Body != nil && expected.Body.Raw != "" {
		// Normalize JSON for comparison
		if !m.matchesBody(body, expected.Body.Raw) {
			return false
		}
	}

	return true
}

// buildPath constructs the path from PostmanURL
func (m *ReportPortalMockServer) buildPath(url PostmanURL) string {
	// If path segments are provided, use them (more reliable)
	if len(url.Path) > 0 {
		return "/" + strings.Join(url.Path, "/")
	}

	// Otherwise, extract from raw URL
	if url.Raw != "" {
		// Extract path from raw URL
		parts := strings.Split(url.Raw, "?")
		pathPart := parts[0]
		// Remove protocol and host
		if idx := strings.Index(pathPart, "://"); idx != -1 {
			pathPart = pathPart[idx+3:]
			if pathIdx := strings.Index(pathPart, "/"); pathIdx != -1 {
				pathPart = pathPart[pathIdx:]
			} else {
				pathPart = "/"
			}
		}
		return pathPart
	}

	return "/"
}

// matchesQueryParams checks if query parameters match
func (m *ReportPortalMockServer) matchesQueryParams(
	r *http.Request,
	expected []PostmanQueryParam,
) bool {
	if len(expected) == 0 {
		return true
	}

	for _, param := range expected {
		if param.Disabled {
			continue
		}
		values, ok := r.URL.Query()[param.Key]
		if !ok || len(values) == 0 {
			return false
		}
		// Check if any value matches
		found := false
		for _, value := range values {
			if value == param.Value {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// matchesHeaders checks if headers match
func (m *ReportPortalMockServer) matchesHeaders(r *http.Request, expected []PostmanHeader) bool {
	if len(expected) == 0 {
		return true
	}

	for _, header := range expected {
		if header.Disabled {
			continue
		}
		value := r.Header.Get(header.Key)
		if value != header.Value {
			slog.Debug(
				"Header mismatch",
				"key",
				header.Key,
				"expected",
				header.Value,
				"actual",
				value,
			)
			return false
		}
	}

	return true
}

// matchesBody checks if request body matches (with JSON normalization)
func (m *ReportPortalMockServer) matchesBody(actual, expected string) bool {
	// Try to parse as JSON and compare
	var actualJSON, expectedJSON interface{}

	if err := json.Unmarshal([]byte(actual), &actualJSON); err != nil {
		// Not JSON, do string comparison
		return strings.TrimSpace(actual) == strings.TrimSpace(expected)
	}

	if err := json.Unmarshal([]byte(expected), &expectedJSON); err != nil {
		// Expected is not JSON, do string comparison
		return strings.TrimSpace(actual) == strings.TrimSpace(expected)
	}

	// Both are JSON, compare normalized
	actualBytes, _ := json.Marshal(actualJSON)
	expectedBytes, _ := json.Marshal(expectedJSON)
	return string(actualBytes) == string(expectedBytes)
}

// AddRequestPair adds a new request/response pair dynamically
func (m *ReportPortalMockServer) AddRequestPair(pair RequestResponsePair) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.requestPairs = append(m.requestPairs, pair)
}

// Reset resets the mock server with new request pairs
func (m *ReportPortalMockServer) Reset(requestPairs []RequestResponsePair) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.requestPairs = requestPairs
	m.requestLog = make([]MockRequestLog, 0)
}
