package report

import "testing"

func TestNormalizeScopeMergesLegacySystemName(t *testing.T) {
	s := NormalizeScope(Scope{SystemName: "gpaas"})
	if len(s.SystemNames) != 1 || s.SystemNames[0] != "gpaas" {
		t.Fatalf("SystemNames = %v, want [gpaas]", s.SystemNames)
	}
}

func TestNormalizeScopeDedupesSystemNames(t *testing.T) {
	s := NormalizeScope(Scope{SystemNames: []string{"b", "a", "b", " "}})
	if len(s.SystemNames) != 2 || s.SystemNames[0] != "a" || s.SystemNames[1] != "b" {
		t.Fatalf("SystemNames = %v, want [a b]", s.SystemNames)
	}
}

func TestScopeForSystem(t *testing.T) {
	base := Scope{EnvironmentTag: "prod", Sections: []string{"cluster"}}
	s := ScopeForSystem(base, "gpaas")
	if s.SystemName != "gpaas" || len(s.SystemNames) != 1 || s.SystemNames[0] != "gpaas" {
		t.Fatalf("scope = %+v", s)
	}
	if s.EnvironmentTag != "prod" {
		t.Fatalf("env lost: %+v", s)
	}
	if len(s.EdgeIDs) != 0 {
		t.Fatalf("edge ids should be cleared for re-resolve")
	}
}

func TestParseScopeNormalizes(t *testing.T) {
	s := ParseScope(`{"system_name":"legacy"}`)
	if len(s.SystemNames) != 1 || s.SystemNames[0] != "legacy" {
		t.Fatalf("ParseScope = %+v", s)
	}
	s2 := ParseScope(`{"system_names":["a","b"]}`)
	if len(s2.SystemNames) != 2 {
		t.Fatalf("ParseScope multi = %+v", s2)
	}
}

func TestContentValidateSystemsMode(t *testing.T) {
	c := &Content{
		Systems: []SystemContentBlock{{
			SystemName: "gpaas",
			Narrative:  Narrative{Headline: "ok"},
		}},
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
