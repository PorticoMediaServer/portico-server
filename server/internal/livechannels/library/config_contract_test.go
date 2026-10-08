package librarychannels

import (
	"context"
	"encoding/json"
	"portico.local/server/internal/apispec"
	"strings"
	"testing"
)

func TestConfigCollectionsAndDormantBounds(t *testing.T) {
	f := openFixture(t)
	s, _ := New(f.db)
	c := channel("config-contract", Query{LibraryIDs: []string{"movies"}, Kinds: []string{"movie"}, Order: "title"}, "sequential", "none")
	c.Enabled = false
	saved, err := s.Save(context.Background(), ownerAuthority(nil), SaveInput{RequestID: "save-config", Config: c})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := s.List(context.Background(), ownerAuthority(nil))
	if err != nil || len(listed) != 1 {
		t.Fatalf("list %v %v", listed, err)
	}
	replay, err := s.Save(context.Background(), ownerAuthority(nil), SaveInput{RequestID: "save-config", Config: c})
	if err != nil {
		t.Fatal(err)
	}
	doc, schema, err := apispec.Schema("LibraryChannelConfig")
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []Config{c, saved.Config, listed[0].Config, replay.Config} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if issues := doc.ValidateJSON(schema, raw); len(issues) > 0 {
			t.Fatalf("%v: %s", issues, raw)
		}
		var obj map[string]any
		json.Unmarshal(raw, &obj)
		obj["blocks"] = nil
		bad, _ := json.Marshal(obj)
		if len(doc.ValidateJSON(schema, bad)) == 0 {
			t.Fatal("schema accepts null blocks")
		}
	}
	for name, edit := range map[string]func(*Config){
		"template":       func(c *Config) { c.TemplateID = strings.Repeat("x", 129) },
		"corner":         func(c *Config) { c.Overlay.Corner = strings.Repeat("x", 41) },
		"treatment":      func(c *Config) { c.Overlay.Treatment = strings.Repeat("x", 41) },
		"negative-size":  func(c *Config) { c.Overlay.SizePercent = -1 },
		"size":           func(c *Config) { c.Overlay.SizePercent = 101 },
		"negative-inset": func(c *Config) { c.Overlay.InsetPercent = -1 },
		"inset":          func(c *Config) { c.Overlay.InsetPercent = 101 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := c
			edit(&bad)
			if _, err := s.Save(context.Background(), ownerAuthority(nil), SaveInput{RequestID: "bad-" + name, ExpectedRevision: saved.Revision, Config: bad}); err == nil {
				t.Fatal("accepted invalid dormant value")
			}
		})
	}
	// Marshalling must not mutate the caller's slice elements.
	if c.Blocks != nil || c.Rules[0].Weights != nil || c.Rules[0].Query.ShowIDs != nil {
		t.Fatal("wire normalization mutated input")
	}
}
