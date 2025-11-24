package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
)

// TestCase represents a test fixture structure
type TestCase struct {
	Name             string        `json:"name"`
	Description      string        `json:"description,omitempty"`
	LLMClientMock    LLMClientMock `json:"llmClientMock"`
	ReportPortalMock interface{}   `json:"reportPortalMock,omitempty"`
}

// LLMClientMock represents the LLM client mock configuration
type LLMClientMock struct {
	Request          Request  `json:"request"`
	ExpectedResponse Response `json:"expectedResponse"`
}

// Request represents an HTTP request
type Request struct {
	Method string      `json:"method"`
	Header []Header    `json:"header,omitempty"`
	Body   RequestBody `json:"body,omitempty"`
	URL    URL         `json:"url"`
}

// Header represents an HTTP header
type Header struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// RequestBody represents request body
type RequestBody struct {
	Mode string `json:"mode,omitempty"`
	Raw  string `json:"raw,omitempty"`
}

// URL represents a URL structure
type URL struct {
	Raw  string   `json:"raw,omitempty"`
	Path []string `json:"path,omitempty"`
}

// Response represents an HTTP response
type Response struct {
	Name   string   `json:"name,omitempty"`
	Code   int      `json:"code"`
	Header []Header `json:"header,omitempty"`
	Body   string   `json:"body,omitempty"`
}

// MCPResponse represents a JSON-RPC response from MCP server
type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  *MCPResult  `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

// MCPResult represents the result field in MCP response
type MCPResult struct {
	Content []MCPContent `json:"content,omitempty"`
	IsError bool         `json:"isError,omitempty"`
}

// MCPContent represents content in MCP result
type MCPContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// MCPError represents an error in MCP response
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// InitializeRequest represents MCP initialize request
type InitializeRequest struct {
	JSONRPC string                 `json:"jsonrpc"`
	Method  string                 `json:"method"`
	ID      int                    `json:"id"`
	Params  map[string]interface{} `json:"params"`
}

var (
	mcpServerURL = flag.String("url", "http://localhost:8080/mcp", "MCP server URL")
	testDataDir  = flag.String("dir", "testdata", "Test data directory")
	verbose      = flag.Bool("v", false, "Verbose output")
)

func main() {
	flag.Parse()

	// Color setup
	cyan := color.New(color.FgCyan, color.Bold)
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)
	yellow := color.New(color.FgYellow)

	// Check required environment variables
	rpToken := os.Getenv("RP_API_TOKEN")
	rpProject := os.Getenv("RP_PROJECT")

	if rpToken == "" {
		_, _ = red.Println("Error: RP_API_TOKEN environment variable is required")
		os.Exit(1)
	}

	if rpProject == "" {
		_, _ = red.Println("Error: RP_PROJECT environment variable is required")
		os.Exit(1)
	}

	_, _ = cyan.Printf("==> Verifying test fixtures against MCP server at %s\n", *mcpServerURL)
	_, _ = cyan.Printf("    Using project: %s\n\n", rpProject)

	// Step 1: Initialize MCP session
	_, _ = yellow.Println("[1/3] Initializing MCP session...")

	sessionID, err := initializeMCPSession(*mcpServerURL, rpToken, rpProject)
	if err != nil {
		_, _ = red.Printf("Failed to initialize MCP session: %v\n", err)
		os.Exit(1)
	}

	_, _ = green.Printf("    Session ID: %s\n", sessionID)

	// Step 2: Discover test files
	_, _ = yellow.Println("\n[2/3] Discovering test fixtures...")

	testFiles, err := discoverTestFiles(*testDataDir)
	if err != nil {
		_, _ = red.Printf("Failed to discover test files: %v\n", err)
		os.Exit(1)
	}

	if len(testFiles) == 0 {
		_, _ = yellow.Printf("No test files found in %s\n", *testDataDir)
		os.Exit(0)
	}

	_, _ = green.Printf("    Found %d test file(s)\n", len(testFiles))

	// Step 3: Verify each test case
	_, _ = yellow.Println("\n[3/3] Verifying test cases...")

	results := struct {
		Total   int
		Success int
		Failed  int
		Skipped int
	}{}

	for _, testFile := range testFiles {
		results.Total++
		_, _ = cyan.Printf("\n  Testing: %s\n", filepath.Base(testFile))

		success, err := verifyTestCase(testFile, *mcpServerURL, sessionID, rpToken, rpProject)
		if err != nil {
			if strings.Contains(err.Error(), "skipped") {
				_, _ = yellow.Printf("    ⚠ Skipped: %v\n", err)
				results.Skipped++
			} else {
				_, _ = red.Printf("    ✗ Failed: %v\n", err)
				results.Failed++
			}
			continue
		}

		if success {
			_, _ = green.Println("    ✓ Passed: Received valid response from MCP server")
			results.Success++
		} else {
			_, _ = red.Println("    ✗ Failed: Tool execution returned error")
			results.Failed++
		}
	}

	// Summary
	_, _ = cyan.Println("\n" + strings.Repeat("=", 60))
	_, _ = cyan.Println("Verification Summary:")
	_, _ = cyan.Println(strings.Repeat("=", 60))
	fmt.Printf("  Total:   %d\n", results.Total)
	_, _ = green.Printf("  Success: %d\n", results.Success)
	if results.Failed > 0 {
		_, _ = red.Printf("  Failed:  %d\n", results.Failed)
	} else {
		fmt.Printf("  Failed:  %d\n", results.Failed)
	}
	_, _ = yellow.Printf("  Skipped: %d\n", results.Skipped)

	if results.Failed > 0 {
		_, _ = red.Println("\n⚠ Some tests failed. Check the output above for details.")
		os.Exit(1)
	} else {
		_, _ = green.Println("\n✓ All tests passed!")
		os.Exit(0)
	}
}

// initializeMCPSession initializes an MCP session and returns the session ID
func initializeMCPSession(serverURL, token, project string) (string, error) {
	initReq := InitializeRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		ID:      0,
		Params: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo": map[string]interface{}{
				"name":    "testdata-verifier",
				"version": "1.0.0",
			},
		},
	}

	body, err := json.Marshal(initReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal initialize request: %w", err)
	}

	req, err := http.NewRequest("POST", serverURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Project", project)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf(
			"initialize failed with status %d: %s",
			resp.StatusCode,
			string(bodyBytes),
		)
	}

	sessionID := resp.Header.Get("mcp-session-id")
	if sessionID == "" {
		return "", fmt.Errorf("no mcp-session-id in response headers")
	}

	return sessionID, nil
}

// discoverTestFiles finds all JSON test files in the directory
func discoverTestFiles(dir string) ([]string, error) {
	var files []string

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if filepath.Ext(entry.Name()) == ".json" && entry.Name() != "README.md" {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}

	return files, nil
}

// verifyTestCase verifies a single test case against the MCP server
func verifyTestCase(testFile, serverURL, sessionID, token, project string) (bool, error) {
	// Read and parse test case
	data, err := os.ReadFile(testFile) //nolint:gosec // testFile is from controlled test directory
	if err != nil {
		return false, fmt.Errorf("failed to read file: %w", err)
	}

	var testCase TestCase
	if err := json.Unmarshal(data, &testCase); err != nil {
		return false, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Check if test case has required fields
	if testCase.LLMClientMock.Request.Body.Raw == "" {
		return false, fmt.Errorf("skipped: no request body found")
	}

	// Build request
	req, err := http.NewRequest(
		"POST",
		serverURL,
		strings.NewReader(testCase.LLMClientMock.Request.Body.Raw),
	)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Project", project)
	req.Header.Set("mcp-session-id", sessionID)

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	if *verbose {
		fmt.Printf("      Response: %s\n", string(bodyBytes))
	}

	// Parse MCP response
	var mcpResp MCPResponse
	if err := json.Unmarshal(bodyBytes, &mcpResp); err != nil {
		return false, fmt.Errorf("failed to parse MCP response: %w", err)
	}

	// Check for errors
	if mcpResp.Error != nil {
		return false, fmt.Errorf("MCP error: %s", mcpResp.Error.Message)
	}

	if mcpResp.Result != nil && mcpResp.Result.IsError {
		if len(mcpResp.Result.Content) > 0 {
			fmt.Printf(
				"      %s\n",
				color.New(color.FgHiBlack).Sprint(mcpResp.Result.Content[0].Text),
			)
		}
		return false, nil
	}

	return true, nil
}
