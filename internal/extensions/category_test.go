package extensions

import (
	"testing"
	"testing/fstest"
)

func TestCategoryLabel_KnownValues(t *testing.T) {
	cases := []struct {
		in   Category
		want string
	}{
		{CategoryCodeIntel, "Code Intelligence"},
		{CategoryMemory, "Memory & Context"},
		{CategoryDocs, "Documentation"},
		{CategoryWebBrowser, "Web & Browser"},
		{CategoryVCS, "Version Control"},
		{CategoryProjectMgmt, "Project Management"},
		{CategoryCommunication, "Communication"},
		{CategoryCloudInfra, "Cloud & Infrastructure"},
		{CategoryDatabases, "Databases"},
		{CategoryDesignUI, "Design & UI"},
		{CategoryTesting, "Testing & QA"},
		{CategorySecurity, "Security"},
		{CategoryAIIntegration, "AI/LLM Integration"},
		{CategoryDevTools, "Developer Tools"},
		{CategoryObservability, "Observability"},
		{CategoryHarnessSpecific, "Harness-specific"},
		{CategoryNiche, "Niche / Specialized"},
	}
	for _, tc := range cases {
		t.Run(string(tc.in), func(t *testing.T) {
			if got := CategoryLabel(tc.in); got != tc.want {
				t.Errorf("CategoryLabel(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCategoryLabel_EmptyAndUnknown(t *testing.T) {
	if got := CategoryLabel(""); got != "Uncategorized" {
		t.Errorf("CategoryLabel(\"\") = %q, want Uncategorized", got)
	}
	// Unknown categories fall through to the raw string — lets
	// contributors invent new categories without a compile step.
	if got := CategoryLabel("totally-made-up"); got != "totally-made-up" {
		t.Errorf("CategoryLabel(unknown) = %q, want raw passthrough", got)
	}
}

func TestAllCategories_Complete(t *testing.T) {
	// AllCategories must include every constant we declare — drives
	// the wizard's group display. If a new constant lands without an
	// entry in AllCategories, the wizard silently drops it.
	wantCount := 17 // bump when adding constants
	if len(AllCategories) != wantCount {
		t.Errorf("AllCategories has %d entries, want %d (did you forget to add a new constant?)",
			len(AllCategories), wantCount)
	}
}

func TestCategoryAndRuntimeNeeds_RoundTripThroughFrontmatter(t *testing.T) {
	// Pin the YAML deserialization for the new fields. If the field
	// tags drift, the loader silently zeroes them and the wizard
	// shows wrong badges. Catch that.
	yaml := `---
name: Test Entry
kind: mcp
source: https://example.com/test
category: databases
runtime_needs:
  local_only: true
  self_hosted: my-stack
  third_party_api: ExampleCorp
  outbound_net: true
---

# Test Entry

Body.
`
	fsys := newFS(map[string]string{
		"extensions/claude-code/test.md": yaml,
	})
	all, err := LoadAll(fsys)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	e, ok := all["claude-code/test"]
	if !ok {
		t.Fatalf("test entry not loaded")
	}
	if e.Category != CategoryDatabases {
		t.Errorf("Category = %q, want %q", e.Category, CategoryDatabases)
	}
	if !e.RuntimeNeeds.LocalOnly {
		t.Error("LocalOnly = false, want true")
	}
	if e.RuntimeNeeds.SelfHosted != "my-stack" {
		t.Errorf("SelfHosted = %q, want my-stack", e.RuntimeNeeds.SelfHosted)
	}
	if e.RuntimeNeeds.ThirdPartyAPI != "ExampleCorp" {
		t.Errorf("ThirdPartyAPI = %q, want ExampleCorp", e.RuntimeNeeds.ThirdPartyAPI)
	}
	if !e.RuntimeNeeds.OutboundNet {
		t.Error("OutboundNet = false, want true")
	}
}

func TestEntry_DescriptionParsesFromFrontmatter(t *testing.T) {
	yaml := `---
name: With Description
description: A one-line summary for the wizard
kind: mcp
source: https://example.com/desc
---

# X
Body.
`
	fsys := newFS(map[string]string{"extensions/claude-code/desc.md": yaml})
	all, err := LoadAll(fsys)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	e := all["claude-code/desc"]
	if e.Description != "A one-line summary for the wizard" {
		t.Errorf("Description = %q, want round-trip from YAML", e.Description)
	}
}

func TestEntry_DescriptionOmittedIsEmpty(t *testing.T) {
	yaml := `---
name: No Description
kind: mcp
source: https://example.com/nodesc
---

# X
Body.
`
	fsys := newFS(map[string]string{"extensions/claude-code/nodesc.md": yaml})
	all, err := LoadAll(fsys)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	e := all["claude-code/nodesc"]
	if e.Description != "" {
		t.Errorf("Description = %q, want empty when omitted", e.Description)
	}
}

func TestCategoryAndRuntimeNeeds_OmittedFieldsAreZero(t *testing.T) {
	// Entries without the new blocks must still load — and the new
	// fields must be zero-valued (no surprise defaults).
	yaml := `---
name: Bare
kind: tool
source: https://example.com/bare
---

# Bare
Body.
`
	fsys := newFS(map[string]string{"extensions/claude-code/bare.md": yaml})
	all, err := LoadAll(fsys)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	e := all["claude-code/bare"]
	if e.Category != "" {
		t.Errorf("Category = %q, want empty", e.Category)
	}
	if e.RuntimeNeeds.LocalOnly || e.RuntimeNeeds.OutboundNet {
		t.Errorf("RuntimeNeeds = %+v, want zero value", e.RuntimeNeeds)
	}
	if e.RuntimeNeeds.SelfHosted != "" || e.RuntimeNeeds.ThirdPartyAPI != "" {
		t.Errorf("RuntimeNeeds = %+v, want empty strings", e.RuntimeNeeds)
	}
}

// Test that unrelated existing entry kinds still load with the new
// fields present in the schema — catches "I broke validation."
func TestAllExistingEntriesParse(t *testing.T) {
	fsys := newFS(map[string]string{
		"extensions/claude-code/plug.md": `---
name: Plugin
kind: plugin
source: https://example.com/p
install: |
  echo install
---

# P
`,
		"extensions/claude-code/skill.md": `---
name: Skill
kind: skill
source: https://example.com/s
---

# S
`,
		"extensions/claude-code/mcp.md": `---
name: MCP
kind: mcp
source: https://example.com/m
---

# M
`,
		"extensions/claude-code/sub.md": `---
name: Sub
kind: subagent
source: https://example.com/sub
---

# Sub
`,
		"extensions/claude-code/tool.md": `---
name: Tool
kind: tool
source: https://example.com/t
---

# T
`,
	})
	got, err := LoadAll(fsys)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(got) != 5 {
		t.Errorf("loaded %d, want 5", len(got))
	}
	for _, k := range AllKinds {
		// Confirm each kind survives a round-trip — defends against
		// future regressions where validation tightens too far.
		found := false
		for _, e := range got {
			if e.Kind == k {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no entry of kind %q parsed", k)
		}
	}
	_ = fstest.MapFS{} // anchor import — keeps go vet quiet on unused-import re-orderings
}
