package librarychannels

import "testing"

func TestBuiltInTemplatesAreUniqueAndScheduleSafe(t *testing.T) {
	templates := BuiltInChannelTemplates()
	if len(templates) != 27 {
		t.Fatalf("template count = %d, want 27", len(templates))
	}
	seen := make(map[string]struct{}, len(templates))
	for _, template := range templates {
		if template.Key == "" || template.Name == "" || template.Description == "" {
			t.Errorf("template has incomplete identity: %+v", template)
		}
		if _, exists := seen[template.Key]; exists {
			t.Errorf("duplicate template key %q", template.Key)
		}
		seen[template.Key] = struct{}{}
		if template.MinimumCandidates < 1 {
			t.Errorf("template %q has unsafe candidate minimum %d", template.Key, template.MinimumCandidates)
		}
		if err := ValidateScheduleQuery(template.Query); err != nil {
			t.Errorf("template %q query is not schedule-safe: %v", template.Key, err)
		}
	}
}

func TestBuiltInBlockPresetsAreValid(t *testing.T) {
	seen := make(map[string]struct{})
	for _, preset := range BuiltInBlockPresets() {
		if _, exists := seen[preset.Key]; exists {
			t.Errorf("duplicate preset key %q", preset.Key)
		}
		seen[preset.Key] = struct{}{}
		block := WeeklyBlock{
			ID: preset.Key, ChannelID: "channel", RuleID: "rule", Name: preset.Name,
			Enabled: true, Weekdays: preset.Weekdays, StartMinute: preset.StartMinute,
			EndMinute: preset.EndMinute, Anchored: preset.Anchored, AllowOverrun: preset.AllowOverrun,
		}
		rules := map[string]Rule{"rule": {ID: "rule", ChannelID: "channel"}}
		if err := ValidateBlock(block, "channel", rules); err != nil {
			t.Errorf("preset %q is invalid: %v", preset.Key, err)
		}
	}
}

func TestRecentlyAddedHonorsRecencyOrderingAndApplicability(t *testing.T) {
	var recent ChannelTemplate
	for _, template := range BuiltInChannelTemplates() {
		if template.Key == "recently-added" {
			recent = template
			break
		}
	}
	if recent.SelectionMode != SelectionSequential || recent.RecencyDays != 90 || recent.CandidateLimit != 200 {
		t.Fatalf("recently added policy = %+v", recent)
	}
	if EvaluateTemplateApplicability(recent, TemplateInventory{CandidateCount: 12, EntityKinds: map[string]int{"movie": 12}}) != true {
		t.Fatal("applicable recently-added inventory was rejected")
	}
	if EvaluateTemplateApplicability(recent, TemplateInventory{CandidateCount: 11, EntityKinds: map[string]int{"movie": 11}}) {
		t.Fatal("undersized recently-added inventory was accepted")
	}
}

func TestTelevisionTemplateRequiresDistinctSeries(t *testing.T) {
	var television ChannelTemplate
	for _, template := range BuiltInChannelTemplates() {
		if template.Key == "all-television" {
			television = template
		}
	}
	if EvaluateTemplateApplicability(television, TemplateInventory{CandidateCount: 30, DistinctSeries: 1, EntityKinds: map[string]int{"show": 30}}) {
		t.Fatal("single-series inventory was presented as a general television channel")
	}
	if !EvaluateTemplateApplicability(television, TemplateInventory{CandidateCount: 30, DistinctSeries: 3, EntityKinds: map[string]int{"show": 30}}) {
		t.Fatal("valid television inventory was rejected")
	}
}
