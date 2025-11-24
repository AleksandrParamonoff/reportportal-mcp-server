package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// LLMClientMock simulates an LLM client that makes requests to the MCP Server
type LLMClientMock struct {
	mcpServerURL string
	httpClient   *http.Client
}

// NewLLMClientMock creates a new LLM client mock
func NewLLMClientMock(mcpServerURL string) *LLMClientMock {
	return &LLMClientMock{
		mcpServerURL: strings.TrimSuffix(mcpServerURL, "/"),
		httpClient:   &http.Client{},
	}
}

// SendRequest sends a request to the MCP Server and returns the response
func (l *LLMClientMock) SendRequest(request PostmanRequest) (*http.Response, error) {
	// Build URL
	url := l.buildURL(request.URL)

	// Create HTTP request
	req, err := http.NewRequest(strings.ToUpper(request.Method), url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	for _, header := range request.Header {
		if !header.Disabled {
			req.Header.Set(header.Key, header.Value)
		}
	}

	// Add body if present
	if request.Body != nil {
		body := l.buildBody(request.Body)
		if body != nil {
			req.Body = io.NopCloser(bytes.NewReader(body))
			req.ContentLength = int64(len(body))
		}
	}

	// Send request
	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	return resp, nil
}

// SendMCPRequest sends a JSON-RPC MCP request to the MCP server
func (l *LLMClientMock) SendMCPRequest(
	method string,
	params map[string]interface{},
	headers map[string]string,
) (*http.Response, error) {
	// Build JSON-RPC request
	jsonRPCRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"id":      1,
		"params":  params,
	}

	jsonData, err := json.Marshal(jsonRPCRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON-RPC request: %w", err)
	}

	// Create HTTP request
	url := l.mcpServerURL + "/mcp"
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set content type
	req.Header.Set("Content-Type", "application/json")

	// Add custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Send request
	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	return resp, nil
}

// ValidateResponse validates that the response matches the expected response
func (l *LLMClientMock) ValidateResponse(actual *http.Response, expected PostmanResponse) error {
	// Check status code
	if actual.StatusCode != expected.Code {
		return fmt.Errorf(
			"status code mismatch: expected %d, got %d",
			expected.Code,
			actual.StatusCode,
		)
	}

	// Check headers
	for _, expectedHeader := range expected.Header {
		if expectedHeader.Disabled {
			continue
		}
		actualValue := actual.Header.Get(expectedHeader.Key)
		if actualValue != expectedHeader.Value {
			return fmt.Errorf(
				"header %s mismatch: expected %s, got %s",
				expectedHeader.Key,
				expectedHeader.Value,
				actualValue,
			)
		}
	}

	// Read and validate body
	bodyBytes, err := io.ReadAll(actual.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}
	defer func() { _ = actual.Body.Close() }()

	// Compare body (with JSON normalization if needed)
	if expected.Body != "" {
		// Try to extract and compare JSON content from text fields for MCP responses
		if l.matchesMCPResponse(string(bodyBytes), expected.Body) {
			return nil
		}
		if !l.matchesBody(string(bodyBytes), expected.Body) {
			return fmt.Errorf(
				"body mismatch: expected %s, got %s",
				expected.Body,
				string(bodyBytes),
			)
		}
	}

	return nil
}

// buildURL constructs the full URL from PostmanURL
func (l *LLMClientMock) buildURL(url PostmanURL) string {
	if url.Raw != "" {
		// If raw URL is provided and it's absolute, use it
		if strings.HasPrefix(url.Raw, "http://") || strings.HasPrefix(url.Raw, "https://") {
			return url.Raw
		}
		// Otherwise, prepend base URL
		return l.mcpServerURL + url.Raw
	}

	// Build from components
	var fullURL strings.Builder
	if url.Protocol != "" {
		fullURL.WriteString(url.Protocol)
		fullURL.WriteString("://")
	} else {
		// Use base URL protocol
		if strings.HasPrefix(l.mcpServerURL, "https://") {
			fullURL.WriteString("https://")
		} else {
			fullURL.WriteString("http://")
		}
	}

	// Host
	if len(url.Host) > 0 {
		fullURL.WriteString(strings.Join(url.Host, "."))
	} else {
		// Extract host from base URL
		baseURL := strings.TrimPrefix(l.mcpServerURL, "http://")
		baseURL = strings.TrimPrefix(baseURL, "https://")
		if idx := strings.Index(baseURL, "/"); idx != -1 {
			fullURL.WriteString(baseURL[:idx])
		} else {
			fullURL.WriteString(baseURL)
		}
	}

	// Path
	if len(url.Path) > 0 {
		fullURL.WriteString("/")
		fullURL.WriteString(strings.Join(url.Path, "/"))
	}

	// Query parameters
	if len(url.Query) > 0 {
		fullURL.WriteString("?")
		queryParts := make([]string, 0)
		for _, param := range url.Query {
			if !param.Disabled {
				queryParts = append(queryParts, fmt.Sprintf("%s=%s", param.Key, param.Value))
			}
		}
		fullURL.WriteString(strings.Join(queryParts, "&"))
	}

	return fullURL.String()
}

// buildBody constructs the request body
func (l *LLMClientMock) buildBody(body *PostmanRequestBody) []byte {
	if body == nil {
		return nil
	}

	switch body.Mode {
	case "raw":
		return []byte(body.Raw)
	case "urlencoded":
		// Build URL-encoded form data
		parts := make([]string, 0)
		for _, kv := range body.URLEncoded {
			if !kv.Disabled {
				parts = append(parts, fmt.Sprintf("%s=%s", kv.Key, kv.Value))
			}
		}
		return []byte(strings.Join(parts, "&"))
	case "formdata":
		// For form data, we'd need multipart/form-data encoding
		// For simplicity, return raw if available
		if body.Raw != "" {
			return []byte(body.Raw)
		}
		return nil
	default:
		if body.Raw != "" {
			return []byte(body.Raw)
		}
		return nil
	}
}

// matchesBody checks if response body matches expected (with JSON normalization)
func (l *LLMClientMock) matchesBody(actual, expected string) bool {
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

// matchesMCPResponse extracts JSON content from MCP response text fields and compares them
func (l *LLMClientMock) matchesMCPResponse(actual, expected string) bool {
	var actualResp, expectedResp map[string]interface{}

	if err := json.Unmarshal([]byte(actual), &actualResp); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(expected), &expectedResp); err != nil {
		return false
	}

	// Extract text content from result.content[0].text
	actualText := l.extractMCPTextContent(actualResp)
	expectedText := l.extractMCPTextContent(expectedResp)

	if actualText == "" || expectedText == "" {
		return false
	}

	// Compare the JSON content inside the text field
	return l.matchesBody(actualText, expectedText)
}

// extractMCPTextContent extracts the text content from an MCP response
func (l *LLMClientMock) extractMCPTextContent(resp map[string]interface{}) string {
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return ""
	}

	content, ok := result["content"].([]interface{})
	if !ok || len(content) == 0 {
		return ""
	}

	firstItem, ok := content[0].(map[string]interface{})
	if !ok {
		return ""
	}

	text, ok := firstItem["text"].(string)
	if !ok {
		return ""
	}

	return text
}
