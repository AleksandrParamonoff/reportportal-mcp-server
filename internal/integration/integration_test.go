package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mcpreportportal "github.com/reportportal/reportportal-mcp-server/internal/reportportal"
)

// getTestDataDir returns the path to the testdata directory
func getTestDataDir() string {
	_, filename, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(filename)
	// Go up from internal/integration to project root, then to testdata
	projectRoot := filepath.Join(testDir, "..", "..")
	return filepath.Join(projectRoot, "testdata")
}

// TestIntegration runs integration tests based on Postman collection format
func TestIntegration(t *testing.T) {
	// Find all test case files
	testDataDir := getTestDataDir()
	testCaseFiles, err := filepath.Glob(filepath.Join(testDataDir, "*.json"))
	if err != nil {
		t.Fatalf("Failed to find test case files: %v", err)
	}

	if len(testCaseFiles) == 0 {
		t.Skipf("No test case files found in %s directory", testDataDir)
	}

	for _, testFile := range testCaseFiles {
		t.Run(filepath.Base(testFile), func(t *testing.T) {
			runTestCase(t, testFile)
		})
	}
}

// runTestCase runs a single test case
func runTestCase(t *testing.T, testCasePath string) {
	// Read test case file
	//nolint:gosec // testCasePath is from controlled test directory
	data, err := os.ReadFile(
		testCasePath,
	)
	require.NoError(t, err, "Failed to read test case file")

	// Parse test case
	testCase, err := ParseTestCase(data)
	require.NoError(t, err, "Failed to parse test case")

	// Start ReportPortal mock server
	rpMock := NewReportPortalMockServer(testCase.ReportPortalMock.RequestResponsePairs)
	defer rpMock.Close()

	// Parse ReportPortal mock URL
	rpMockURL, err := url.Parse(rpMock.URL())
	require.NoError(t, err, "Failed to parse ReportPortal mock URL")

	// Create and start MCP Server
	mcpServer, err := mcpreportportal.NewHTTPServer(mcpreportportal.HTTPServerConfig{
		Version:         "test-version",
		HostURL:         rpMockURL,
		FallbackRPToken: "",
		AnalyticsOn:     false,
	})
	require.NoError(t, err, "Failed to create MCP server")

	// Start MCP server using httptest
	mcpHTTPServer := httptest.NewServer(mcpServer.Router)
	defer mcpHTTPServer.Close()

	mcpServerURL := mcpHTTPServer.URL

	// Mark server as running for internal state
	err = mcpServer.Start()
	require.NoError(t, err, "Failed to start MCP server")
	defer func() {
		if err := mcpServer.Stop(); err != nil {
			t.Logf("Failed to stop MCP server: %v", err)
		}
	}()

	// Wait a bit for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Create LLM Client Mock
	llmClient := NewLLMClientMock(mcpServerURL)

	// Initialize MCP session first
	sessionID, err := initializeMCPSession(llmClient, mcpServerURL)
	require.NoError(t, err, "Failed to initialize MCP session")

	// Add session ID to request headers
	requestWithSession := testCase.LLMClientMock.Request
	if requestWithSession.Header == nil {
		requestWithSession.Header = []PostmanHeader{}
	}
	requestWithSession.Header = append(requestWithSession.Header, PostmanHeader{
		Key:   "mcp-session-id",
		Value: sessionID,
	})

	// Send request from LLM Client Mock to MCP Server
	resp, err := llmClient.SendRequest(requestWithSession)
	require.NoError(t, err, "Failed to send request from LLM client mock")

	// Read response body for debugging
	bodyBytes, err := io.ReadAll(resp.Body)
	if err == nil {
		t.Logf("MCP Server Response Status: %d", resp.StatusCode)
		t.Logf("MCP Server Response Body: %s", string(bodyBytes))
		// Reset body for validation
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	// Validate response
	err = llmClient.ValidateResponse(resp, testCase.LLMClientMock.ExpectedResponse)
	if err != nil {
		t.Logf("Response validation error: %v", err)
	}
	assert.NoError(t, err, "Response validation failed")

	// Verify that ReportPortal mock received expected requests
	requestLog := rpMock.GetRequestLog()
	if len(testCase.ReportPortalMock.RequestResponsePairs) > 0 {
		assert.NotEmpty(t, requestLog, "ReportPortal mock should have received requests")
		// Log what was received for debugging
		if len(requestLog) > 0 {
			t.Logf("ReportPortal mock received %d request(s)", len(requestLog))
			for i, req := range requestLog {
				t.Logf("  Request %d: %s %s?%s", i+1, req.Method, req.Path, req.QueryParams)
				t.Logf("    Headers: %v", req.Headers)
			}
		}
	}
}

// TestIntegrationFromPostmanCollection runs tests from a Postman collection file
func TestIntegrationFromPostmanCollection(t *testing.T) {
	testDataDir := getTestDataDir()
	collectionFiles, err := filepath.Glob(filepath.Join(testDataDir, "collections", "*.json"))
	if err != nil {
		t.Fatalf("Failed to find collection files: %v", err)
	}

	if len(collectionFiles) == 0 {
		t.Skipf("No Postman collection files found in %s/collections/ directory", testDataDir)
	}

	for _, collectionFile := range collectionFiles {
		t.Run(filepath.Base(collectionFile), func(t *testing.T) {
			runPostmanCollection(t, collectionFile)
		})
	}
}

// runPostmanCollection runs tests from a Postman collection
func runPostmanCollection(t *testing.T, collectionPath string) {
	// Read collection file
	//nolint:gosec // collectionPath is from controlled test directory
	data, err := os.ReadFile(
		collectionPath,
	)
	require.NoError(t, err, "Failed to read collection file")

	// Parse collection
	collection, err := ParsePostmanCollection(data)
	require.NoError(t, err, "Failed to parse Postman collection")

	// Process each item in the collection
	processCollectionItems(t, collection.Item, "")
}

// processCollectionItems recursively processes collection items
func processCollectionItems(t *testing.T, items []PostmanItem, prefix string) {
	for _, item := range items {
		testName := prefix + item.Name

		// If item has nested items, it's a folder
		if len(item.Item) > 0 {
			processCollectionItems(t, item.Item, testName+"/")
			continue
		}

		// If item has a request, it's a test case
		if item.Request.Method != "" {
			t.Run(testName, func(t *testing.T) {
				// Extract test case configuration from item
				// This assumes the item.response contains the ReportPortal mock config
				// and the item.request is the LLM client request

				// For now, we'll create a minimal test case
				// In a real implementation, you'd extract this from the collection structure
				t.Logf("Running test case: %s", testName)
				t.Skip("Postman collection test case extraction not fully implemented")
			})
		}
	}
}

// initializeMCPSession initializes an MCP session and returns the session ID
func initializeMCPSession(llmClient *LLMClientMock, mcpServerURL string) (string, error) {
	// Create initialize request
	initRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialize",
		"id":      0,
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "integration-test",
				"version": "1.0.0",
			},
		},
	}

	jsonData, err := json.Marshal(initRequest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal initialize request: %w", err)
	}

	// Send initialize request
	req, err := http.NewRequest("POST", mcpServerURL+"/mcp", bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send initialize request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read initialize response: %w", err)
	}

	// Extract session ID from response header
	sessionID := resp.Header.Get("mcp-session-id")
	if sessionID == "" {
		// Try to parse response to see if there's an error
		var initResp map[string]interface{}
		if json.Unmarshal(bodyBytes, &initResp) == nil {
			return "", fmt.Errorf("no session ID in response, body: %s", string(bodyBytes))
		}
		return "", fmt.Errorf("no session ID in response headers")
	}

	return sessionID, nil
}
