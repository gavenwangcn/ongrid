package report

import "encoding/json"

// llmOutputJSONSchema is the strict JSON Schema passed to OpenAI-compatible
// structured-output APIs for the Pass-2 content extractor.
var llmOutputJSONSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "version": { "type": "string" },
    "narrative": {
      "type": "object",
      "properties": {
        "headline": { "type": "string" },
        "paragraphs": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "text": { "type": "string" }
            },
            "required": ["text"],
            "additionalProperties": false
          }
        }
      },
      "required": ["headline", "paragraphs"],
      "additionalProperties": false
    },
    "advice": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "text": { "type": "string" }
        },
        "required": ["text"],
        "additionalProperties": false
      }
    }
  },
  "required": ["narrative"],
  "additionalProperties": false
}`)

// RequiredLLMOutputSchema returns the canonical ContentJSON subset the
// reporter worker must emit (narrative + advice only). Embedded in prompts
// and schema-retry messages so every model sees the same contract.
func RequiredLLMOutputSchema() string {
	return requiredLLMOutputSchemaFlat()
}

// RequiredLLMOutputSchemaForFacts picks flat vs per-system schema based on
// collected facts shape.
func RequiredLLMOutputSchemaForFacts(facts *ReportFacts) string {
	if facts != nil && len(facts.Systems) > 0 {
		return requiredLLMOutputSchemaPerSystem()
	}
	return requiredLLMOutputSchemaFlat()
}

func requiredLLMOutputSchemaFlat() string {
	return `{
  "version": "1",
  "narrative": {
    "headline": "<one sentence summarizing the period>",
    "paragraphs": [
      {"text": "<prose paragraph; may embed {{entity:kind:id|name}} tokens>"}
    ]
  },
  "advice": [
    {"text": "<actionable recommendation>"}
  ]
}`
}

func requiredLLMOutputSchemaPerSystem() string {
	return `{
  "version": "1",
  "systems": [
    {
      "system_name": "<exact system_name from facts.systems[].system_name — do NOT invent>",
      "narrative": {
        "headline": "<one sentence summarizing THIS system only>",
        "paragraphs": [
          {"text": "<prose for THIS system; may embed {{entity:kind:id|name}} tokens>"}
        ]
      },
      "advice": [
        {"text": "<actionable recommendation for THIS system>"}
      ]
    }
  ]
}`
}

const contentExtractorSystemPrompt = `You are a report content extractor for an AIOps platform.
Given a draft operations report (any JSON shape or prose), output ONE JSON object
that matches the required ContentJSON schema exactly.

Rules:
- Output ONLY the JSON object — no markdown fences, no commentary.
- Copy facts and wording from the draft; do NOT invent numbers or entities.
- narrative.headline is required (one sentence).
- narrative.paragraphs is an array of {"text":"..."} objects (2–4 paragraphs when the draft has enough material).
- advice is an array of {"text":"..."}; use [] when there is nothing actionable.
- Do NOT include hero, resource, fleet, logs, or other numeric sections — the system injects those from SQL facts.
- When the draft has a systems[] array, output systems[] with one entry per business system — do NOT merge systems into one narrative.
- Each systems[].system_name must match the draft facts exactly; never aggregate cross-system prose.
- Preserve {{entity:kind:id|display}} tokens verbatim when present.`
