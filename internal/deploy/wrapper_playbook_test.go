package deploy

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateWrapperPlaybook_Basic(t *testing.T) {
	cfg := WrapperConfig{
		UserPlaybook:       "/deploy/playbook/site.yml",
		SkipIfSucceededFor: 0,
		RunID:              "abc123def456",
		DiffusionVersion:   "1.0.0",
	}
	out, err := GenerateWrapperPlaybook(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)

	// Should contain the run ID.
	if !strings.Contains(s, "abc123def456") {
		t.Error("expected RunID in wrapper output")
	}
	// Should reference the user playbook.
	if !strings.Contains(s, "/deploy/playbook/site.yml") {
		t.Error("expected user playbook path in wrapper output")
	}
	// Should contain state check play.
	if !strings.Contains(s, "state check") {
		t.Error("expected state check play")
	}
	// Should use stat instead of slurp directly.
	if !strings.Contains(s, "ansible.builtin.stat") {
		t.Error("expected stat task for file existence check")
	}
	// Should not contain skip logic when SkipIfSucceededFor is 0.
	if strings.Contains(s, "Evaluate skip condition") {
		t.Error("unexpected skip condition when SkipIfSucceededFor is 0")
	}
	// Should set diffusion_skip to false.
	if !strings.Contains(s, "diffusion_skip: false") {
		t.Error("expected diffusion_skip: false when skip disabled")
	}
}

func TestGenerateWrapperPlaybook_WithSkip(t *testing.T) {
	cfg := WrapperConfig{
		UserPlaybook:       "/deploy/playbook/site.yml",
		SkipIfSucceededFor: 24 * time.Hour,
		RunID:              "skip123",
		DiffusionVersion:   "2.0.0",
	}
	out, err := GenerateWrapperPlaybook(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)

	// Should contain skip evaluation.
	if !strings.Contains(s, "Evaluate skip condition") {
		t.Error("expected skip condition task")
	}
	// Should contain skip seconds (86400 for 24h).
	if !strings.Contains(s, "86400") {
		t.Error("expected 86400 seconds in skip condition")
	}
	// Should contain skip notice.
	if !strings.Contains(s, "Skip notice") {
		t.Error("expected skip notice task")
	}
}

func TestGenerateWrapperPlaybook_JinjaBraces(t *testing.T) {
	cfg := WrapperConfig{
		UserPlaybook:       "/deploy/playbook/site.yml",
		SkipIfSucceededFor: 0,
		RunID:              "jinja-test",
		DiffusionVersion:   "1.0.0",
	}
	out, err := GenerateWrapperPlaybook(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)

	// Should contain proper Jinja2 braces for Ansible (not Go template syntax).
	if !strings.Contains(s, "{{") {
		t.Error("expected Jinja2 '{{' in rendered output")
	}
	if !strings.Contains(s, "}}") {
		t.Error("expected Jinja2 '}}' in rendered output")
	}
	// Should NOT contain Go template artifacts.
	if strings.Contains(s, "{{ .") {
		t.Error("unexpected Go template syntax in rendered output")
	}
}

func TestGenerateWrapperPlaybook_SuccessState(t *testing.T) {
	cfg := WrapperConfig{
		UserPlaybook:       "/deploy/playbook/site.yml",
		SkipIfSucceededFor: 0,
		RunID:              "state-test",
		DiffusionVersion:   "3.0.0",
	}
	out, err := GenerateWrapperPlaybook(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)

	// Should write success state.
	if !strings.Contains(s, "Write diffusion state (success)") {
		t.Error("expected success state write task")
	}
	if !strings.Contains(s, "status: \"success\"") {
		t.Error("expected status success in state content")
	}
	if !strings.Contains(s, "3.0.0") {
		t.Error("expected diffusion version in state content")
	}
}

func TestGenerateWrapperPlaybook_EnsureDirectory(t *testing.T) {
	cfg := WrapperConfig{
		UserPlaybook: "/deploy/playbook/site.yml",
		RunID:        "dir-test",
	}
	out, err := GenerateWrapperPlaybook(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)

	if !strings.Contains(s, "Ensure ~/.diffusion directory exists") {
		t.Error("expected directory creation task")
	}
	if !strings.Contains(s, "~/.diffusion") {
		t.Error("expected ~/.diffusion path")
	}
}

func TestGenerateWrapperPlaybook_StatBeforeSlurp(t *testing.T) {
	cfg := WrapperConfig{
		UserPlaybook: "/deploy/playbook/site.yml",
		RunID:        "stat-test",
	}
	out, err := GenerateWrapperPlaybook(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)

	statIdx := strings.Index(s, "ansible.builtin.stat")
	slurpIdx := strings.Index(s, "ansible.builtin.slurp")

	if statIdx < 0 {
		t.Fatal("expected stat task in wrapper")
	}
	if slurpIdx < 0 {
		t.Fatal("expected slurp task in wrapper")
	}
	if statIdx > slurpIdx {
		t.Error("expected stat to appear BEFORE slurp")
	}
}
