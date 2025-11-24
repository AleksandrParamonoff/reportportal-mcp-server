package integration

import (
	"encoding/json"
	"fmt"
)

// PostmanCollection represents a Postman Collection v2.1.0 structure
// Based on: https://schema.postman.com/collection/json/v2.1.0/draft-07/collection.json
type PostmanCollection struct {
	Info PostmanInfo   `json:"info"`
	Item []PostmanItem `json:"item"`
}

// PostmanInfo contains collection metadata
type PostmanInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Schema      string `json:"schema"`
	Version     string `json:"_postman_id,omitempty"`
}

// PostmanItem represents a request item in the collection
type PostmanItem struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Request     PostmanRequest    `json:"request"`
	Response    []PostmanResponse `json:"response,omitempty"`
	Item        []PostmanItem     `json:"item,omitempty"` // For folders
}

// PostmanRequest represents an HTTP request
type PostmanRequest struct {
	Method      string              `json:"method"`
	Header      []PostmanHeader     `json:"header,omitempty"`
	Body        *PostmanRequestBody `json:"body,omitempty"`
	URL         PostmanURL          `json:"url"`
	Description string              `json:"description,omitempty"`
	Variable    []PostmanVariable   `json:"variable,omitempty"`
}

// PostmanHeader represents an HTTP header
type PostmanHeader struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	Disabled    bool   `json:"disabled,omitempty"`
}

// PostmanRequestBody represents request body
type PostmanRequestBody struct {
	Mode                    string            `json:"mode,omitempty"` // raw, urlencoded, formdata, file, graphql
	Raw                     string            `json:"raw,omitempty"`
	URLEncoded              []PostmanKeyValue `json:"urlencoded,omitempty"`
	FormData                []PostmanKeyValue `json:"formdata,omitempty"`
	GraphQL                 interface{}       `json:"graphql,omitempty"`
	Options                 interface{}       `json:"options,omitempty"`
	DisablePrerequestEditor bool              `json:"disablePrerequestEditor,omitempty"`
}

// PostmanKeyValue represents a key-value pair
type PostmanKeyValue struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	Disabled    bool   `json:"disabled,omitempty"`
	Type        string `json:"type,omitempty"`
}

// PostmanURL represents a URL
type PostmanURL struct {
	Raw      string              `json:"raw,omitempty"`
	Protocol string              `json:"protocol,omitempty"`
	Host     []string            `json:"host,omitempty"`
	Path     []string            `json:"path,omitempty"`
	Query    []PostmanQueryParam `json:"query,omitempty"`
	Variable []PostmanVariable   `json:"variable,omitempty"`
}

// PostmanQueryParam represents a query parameter
type PostmanQueryParam struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	Disabled    bool   `json:"disabled,omitempty"`
}

// PostmanVariable represents a variable
type PostmanVariable struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	Disabled    bool   `json:"disabled,omitempty"`
}

// PostmanResponse represents an expected response
type PostmanResponse struct {
	Name            string          `json:"name"`
	OriginalRequest PostmanRequest  `json:"originalRequest,omitempty"`
	Status          string          `json:"status,omitempty"`
	Code            int             `json:"code"`
	Header          []PostmanHeader `json:"header,omitempty"`
	Body            string          `json:"body,omitempty"`
	ResponseTime    int             `json:"responseTime,omitempty"`
	ResponseSize    int             `json:"responseSize,omitempty"`
}

// TestCase represents a single integration test case
type TestCase struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	EndpointPath string `json:"endpointPath,omitempty"` // ReportPortal API endpoint path pattern (e.g., "/v1/{projectName}/item/{itemId}")

	// ReportPortal Mock configuration
	ReportPortalMock ReportPortalMockConfig `json:"reportPortalMock"`

	// LLM Client Mock configuration
	LLMClientMock LLMClientMockConfig `json:"llmClientMock"`
}

// ReportPortalMockConfig defines request/response pairs for ReportPortal mock
type ReportPortalMockConfig struct {
	RequestResponsePairs []RequestResponsePair `json:"requestResponsePairs"`
}

// RequestResponsePair defines a single request/response pair
type RequestResponsePair struct {
	Request  PostmanRequest  `json:"request"`
	Response PostmanResponse `json:"response"`
}

// LLMClientMockConfig defines the LLM client request and expected response
type LLMClientMockConfig struct {
	Request          PostmanRequest  `json:"request"`
	ExpectedResponse PostmanResponse `json:"expectedResponse"`
}

// ParsePostmanCollection parses a Postman collection JSON
func ParsePostmanCollection(data []byte) (*PostmanCollection, error) {
	var collection PostmanCollection
	if err := json.Unmarshal(data, &collection); err != nil {
		return nil, fmt.Errorf("failed to parse Postman collection: %w", err)
	}
	return &collection, nil
}

// ParseTestCase parses a test case JSON
func ParseTestCase(data []byte) (*TestCase, error) {
	var testCase TestCase
	if err := json.Unmarshal(data, &testCase); err != nil {
		return nil, fmt.Errorf("failed to parse test case: %w", err)
	}
	return &testCase, nil
}
