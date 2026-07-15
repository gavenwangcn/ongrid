package flow

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidJSONRaw(t *testing.T) {
	t.Parallel()
	valid := json.RawMessage(`{"type":"object"}`)
	if got := validJSONRaw(valid); string(got) != string(valid) {
		t.Fatalf("valid schema: got %q want %q", got, valid)
	}
	if got := validJSONRaw(json.RawMessage(`{not-json`)); string(got) != "null" {
		t.Fatalf("invalid schema: got %q want null", got)
	}
	if got := validJSONRaw(nil); got != nil {
		t.Fatalf("nil schema: got %q want nil", got)
	}
}

func TestWriteJSON_invalidEmbeddedRawMessage(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]any{
		"items": []toolMetaDTO{{
			Name:       "bad_tool",
			Parameters: json.RawMessage(`{broken`),
		}},
	})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want 500", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Fatal("expected error JSON body")
	}
}

func TestWriteJSON_sanitizedInvalidRawMessage(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]any{
		"items": []toolMetaDTO{{
			Name:       "bad_tool",
			Parameters: validJSONRaw(json.RawMessage(`{broken`)),
		}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d want 200", w.Code)
	}
	var out struct {
		Items []struct {
			Name       string          `json:"name"`
			Parameters json.RawMessage `json:"parameters"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v body=%q", err, w.Body.String())
	}
	if len(out.Items) != 1 || out.Items[0].Name != "bad_tool" {
		t.Fatalf("unexpected items: %+v", out.Items)
	}
	if string(out.Items[0].Parameters) != "null" {
		t.Fatalf("parameters=%q want null", out.Items[0].Parameters)
	}
}
