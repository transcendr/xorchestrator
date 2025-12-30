// Package types provides shared types for MCP protocol handling.
// These types are extracted to avoid import cycles between the mcp and v2/adapter packages.
package types

// ToolCallResult is the response for tools/call.
// JSON tags use camelCase as required by MCP protocol specification.
type ToolCallResult struct {
	Content           []ContentItem `json:"content"`
	IsError           bool          `json:"isError,omitempty"`           //nolint:tagliatelle // MCP protocol requires camelCase
	StructuredContent any           `json:"structuredContent,omitempty"` //nolint:tagliatelle // MCP protocol requires camelCase
}

// ContentItem represents a single content item in a tool result.
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	// Additional fields for image, audio, resource_link can be added as needed
}

// SuccessResult creates a successful tool result with text content.
func SuccessResult(text string) *ToolCallResult {
	return &ToolCallResult{
		Content: []ContentItem{
			{Type: "text", Text: text},
		},
	}
}

// ErrorResult creates an error tool result with text content.
func ErrorResult(text string) *ToolCallResult {
	return &ToolCallResult{
		Content: []ContentItem{
			{Type: "text", Text: text},
		},
		IsError: true,
	}
}

// StructuredResult creates a successful tool result with both text and structured content.
func StructuredResult(text string, structuredContent any) *ToolCallResult {
	return &ToolCallResult{
		Content: []ContentItem{
			{Type: "text", Text: text},
		},
		StructuredContent: structuredContent,
	}
}
