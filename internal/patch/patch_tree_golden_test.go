package patch

import (
	"strings"
	"testing"
)

// goldenColor/goldenPlain capture PrintTree's exact output (captured
// BEFORE the structured-row refactor in this change) for writeFixtureRole
// rendered with FilterTags, Overlays, handlers with notify, nested
// include branches, an opaque branch and warnings. "{{ROOT}}" stands in
// for a.RolePath, which varies per test run (a temp dir). PrintTree's
// non-TUI output must stay byte-for-byte identical to these strings
// forever, in both colour and NoColor modes: this is the regression
// guard for the row-model refactor that also backs the TUI.
const goldenColor = "role fixture.role {{ROOT}} (scenario: default) — 8 tasks, 2 handlers, 5 branches\n" +
	"\x1b[38;5;214m├── ▶ 1 [branch include_tasks] tasks/main.yml:2 Setup everything -> tasks/setup.yml ◆ tags:[setup]\x1b[32m ★\x1b[0m\x1b[0m\n" +
	"\x1b[38;5;214m│   ├── ● 1.1 [copy] tasks/setup.yml:2 Install pkg src=foo.conf dest=/etc/foo.conf <= patch/files/banner.custom ⇄ inherits [setup]\x1b[0m\n" +
	"\x1b[38;5;214m│   ├── ● 1.2 [template] tasks/setup.yml:6 Render bar src=bar.j2 dest=/etc/bar.conf ⇄ inherits [setup]\x1b[0m\n" +
	"\x1b[38;5;214m│   ├── ▶ 1.3 [branch include_tasks] tasks/setup.yml:10 More -> tasks/nested.yml\x1b[0m\n" +
	"\x1b[38;5;214m│   │   └── ● 1.3.1 [file] tasks/nested.yml:2 Data dir dest=/data ⇄ inherits [setup]\x1b[0m\n" +
	"\x1b[38;5;214m│   └── ● 1.4 [service] tasks/setup.yml:12 Ensure service ⇄ inherits [setup]\x1b[0m\n" +
	"├── ● 2 [template] tasks/main.yml:6 Deploy app config src=app.j2 dest=/etc/app.conf notify=restart app <= patch/templates/app.custom ◆ tags:[deploy]\x1b[32m ★\x1b[0m\n" +
	"│   └── ↳ h1 [service] restart app\n" +
	"\x1b[38;5;178m├── ▶ 3 [branch block] tasks/main.yml:12 Harden\x1b[0m\n" +
	"\x1b[38;5;178m│   ├── ● 3.1 [copy] tasks/main.yml:14 Copy banner src=banner dest=/etc/banner\x1b[0m\n" +
	"\x1b[38;5;178m│   ├── ▶ 3.2 [branch include_tasks] tasks/main.yml:18 Nested include -> tasks/nested.yml\x1b[0m\n" +
	"\x1b[38;5;178m│   │   └── ● 3.2.1 [file] tasks/nested.yml:2 Data dir dest=/data\x1b[0m\n" +
	"\x1b[38;5;178m│   └── ● 3.3 [file] tasks/main.yml:21 Fallback file dest=/tmp/fallback\x1b[0m\n" +
	"\x1b[38;5;45m└── ▶ 4 [branch include_tasks] tasks/main.yml:25 Ghost include -> tasks/missing.yml (opaque: include target not found)\x1b[0m\n" +
	"handlers:\n" +
	"├── ● h1 [service] handlers/main.yml:2 restart app\n" +
	"└── ● h2 [template] handlers/main.yml:6 reload config src=app.j2 dest=/etc/app.conf ◆ tags:[deploy]\x1b[32m ★\x1b[0m\n" +
	"warnings:\n" +
	"  - \x1b[33mtasks/main.yml:25 (4): include target \"tasks/missing.yml\" not found\x1b[0m\n"

const goldenPlain = "role fixture.role {{ROOT}} (scenario: default) — 8 tasks, 2 handlers, 5 branches\n" +
	"├── ▶ 1 [branch include_tasks] tasks/main.yml:2 Setup everything -> tasks/setup.yml ◆ tags:[setup] ★\n" +
	"│   ├── ● 1.1 [copy] tasks/setup.yml:2 Install pkg src=foo.conf dest=/etc/foo.conf <= patch/files/banner.custom ⇄ inherits [setup]\n" +
	"│   ├── ● 1.2 [template] tasks/setup.yml:6 Render bar src=bar.j2 dest=/etc/bar.conf ⇄ inherits [setup]\n" +
	"│   ├── ▶ 1.3 [branch include_tasks] tasks/setup.yml:10 More -> tasks/nested.yml\n" +
	"│   │   └── ● 1.3.1 [file] tasks/nested.yml:2 Data dir dest=/data ⇄ inherits [setup]\n" +
	"│   └── ● 1.4 [service] tasks/setup.yml:12 Ensure service ⇄ inherits [setup]\n" +
	"├── ● 2 [template] tasks/main.yml:6 Deploy app config src=app.j2 dest=/etc/app.conf notify=restart app <= patch/templates/app.custom ◆ tags:[deploy] ★\n" +
	"│   └── ↳ h1 [service] restart app\n" +
	"├── ▶ 3 [branch block] tasks/main.yml:12 Harden\n" +
	"│   ├── ● 3.1 [copy] tasks/main.yml:14 Copy banner src=banner dest=/etc/banner\n" +
	"│   ├── ▶ 3.2 [branch include_tasks] tasks/main.yml:18 Nested include -> tasks/nested.yml\n" +
	"│   │   └── ● 3.2.1 [file] tasks/nested.yml:2 Data dir dest=/data\n" +
	"│   └── ● 3.3 [file] tasks/main.yml:21 Fallback file dest=/tmp/fallback\n" +
	"└── ▶ 4 [branch include_tasks] tasks/main.yml:25 Ghost include -> tasks/missing.yml (opaque: include target not found)\n" +
	"handlers:\n" +
	"├── ● h1 [service] handlers/main.yml:2 restart app\n" +
	"└── ● h2 [template] handlers/main.yml:6 reload config src=app.j2 dest=/etc/app.conf ◆ tags:[deploy] ★\n" +
	"warnings:\n" +
	"  - tasks/main.yml:25 (4): include target \"tasks/missing.yml\" not found\n"

func TestPrintTreeGoldenOutputUnchanged(t *testing.T) {
	root := writeFixtureRole(t)
	a, err := analyzeRoleAtPath(root, "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	// Branch colours are picked by a seeded shuffle keyed on scenario+path
	// (so the same role is stable across runs, but varies across temp
	// dirs). Re-seed deterministically here so the golden strings below
	// don't depend on the test's random tmp dir.
	AssignBranchColors(a, "golden-fixed-seed")
	opts := TreeOptions{
		FilterTags: []string{"setup", "deploy"},
		Overlays: map[string]string{
			"1.1": "patch/files/banner.custom",
			"2":   "patch/templates/app.custom",
		},
	}

	var sbColor strings.Builder
	a.PrintTree(&sbColor, opts)
	wantColor := strings.ReplaceAll(goldenColor, "{{ROOT}}", a.RolePath)
	if sbColor.String() != wantColor {
		t.Errorf("colour PrintTree output changed.\nGOT:\n%q\nWANT:\n%q", sbColor.String(), wantColor)
	}

	plainOpts := opts
	plainOpts.NoColor = true
	var sbPlain strings.Builder
	a.PrintTree(&sbPlain, plainOpts)
	wantPlain := strings.ReplaceAll(goldenPlain, "{{ROOT}}", a.RolePath)
	if sbPlain.String() != wantPlain {
		t.Errorf("NoColor PrintTree output changed.\nGOT:\n%q\nWANT:\n%q", sbPlain.String(), wantPlain)
	}
}
