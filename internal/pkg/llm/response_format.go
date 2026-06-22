package llm

import "encoding/json"

// ResponseFormat optionally constrains chat completion output to JSON.
// Nil leaves the provider default (plain text). OpenAI-compatible
// endpoints honour json_object / json_schema; others may ignore or error.
type ResponseFormat struct {
	// Type is "json_object" or "json_schema".
	Type string
	// SchemaName is required when Type is "json_schema".
	SchemaName string
	// JSONSchema is a JSON Schema object when Type is "json_schema".
	JSONSchema json.RawMessage
}

// JSONObjectFormat requests a single JSON object response (no markdown).
func JSONObjectFormat() *ResponseFormat {
	return &ResponseFormat{Type: "json_object"}
}

// JSONSchemaFormat requests strict JSON matching schema (OpenAI-style).
func JSONSchemaFormat(name string, schema json.RawMessage) *ResponseFormat {
	return &ResponseFormat{
		Type:       "json_schema",
		SchemaName: name,
		JSONSchema: schema,
	}
}
