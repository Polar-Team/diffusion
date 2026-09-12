package dependency

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// chdirTemp switches to a fresh temporary directory for the duration of a test.
func chdirTemp(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("chdir back: %v", err)
		}
	})

	return dir
}

func TestDiscoverScenariosFallsBackToDefault(t *testing.T) {
	chdirTemp(t)

	got := DiscoverScenarios()
	want := []string{"default"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiscoverScenarios() = %v, want %v", got, want)
	}
}

func TestResolveScenarios(t *testing.T) {
	dir := chdirTemp(t)

	for _, name := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(dir, "scenarios", name), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}

	tests := []struct {
		name     string
		scenario string
		want     []string
		wantErr  bool
	}{
		{name: "empty selects all", scenario: "", want: []string{"a", "b"}},
		{name: "explicit existing", scenario: "a", want: []string{"a"}},
		{name: "explicit existing b", scenario: "b", want: []string{"b"}},
		{name: "unknown", scenario: "zzz", wantErr: true},
		{name: "default without dir", scenario: "default", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveScenarios(tt.scenario)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveScenarios(%q) expected error, got %v", tt.scenario, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveScenarios(%q) error = %v", tt.scenario, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ResolveScenarios(%q) = %v, want %v", tt.scenario, got, tt.want)
			}
		})
	}
}

func TestResolveScenariosDefaultWithoutScenariosDir(t *testing.T) {
	chdirTemp(t)

	got, err := ResolveScenarios("default")
	if err != nil {
		t.Fatalf("ResolveScenarios(default) error = %v", err)
	}
	if !reflect.DeepEqual(got, []string{"default"}) {
		t.Fatalf("ResolveScenarios(default) = %v, want [default]", got)
	}
}

func names(entries []LockFileEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name)
	}
	return out
}

func TestMergeLockFileForScenario(t *testing.T) {
	existing := &LockFile{
		Hash: "old",
		Collections: []LockFileEntry{
			{Name: "default.general", ResolvedVersion: "1.0.0"},
			{Name: "extra.posix", ResolvedVersion: "2.0.0"},
			{Name: "default.crypto", ResolvedVersion: "3.0.0"},
		},
		Roles: []LockFileEntry{
			{Name: "extra.docker", ResolvedVersion: "7.0.0"},
			{Name: "default.nginx", ResolvedVersion: "1.1.1"},
		},
		Tools: []LockFileEntry{{Name: "ansible", ResolvedVersion: "9.0.0"}},
	}

	fresh := &LockFile{
		Hash:        "new",
		Collections: []LockFileEntry{{Name: "default.general", ResolvedVersion: "1.5.0"}},
		Roles:       []LockFileEntry{{Name: "default.nginx", ResolvedVersion: "2.2.2"}},
		Tools:       []LockFileEntry{{Name: "ansible", ResolvedVersion: "10.0.0"}},
	}

	tests := []struct {
		name        string
		existing    *LockFile
		fresh       *LockFile
		scenario    string
		wantCols    []string
		wantRoles   []string
		wantHash    string
		wantToolVer string
	}{
		{
			name:        "merge default scenario",
			existing:    existing,
			fresh:       fresh,
			scenario:    "default",
			wantCols:    []string{"extra.posix", "default.general"},
			wantRoles:   []string{"extra.docker", "default.nginx"},
			wantHash:    "new",
			wantToolVer: "10.0.0",
		},
		{
			name:        "no existing lock file",
			existing:    nil,
			fresh:       fresh,
			scenario:    "default",
			wantCols:    []string{"default.general"},
			wantRoles:   []string{"default.nginx"},
			wantHash:    "new",
			wantToolVer: "10.0.0",
		},
		{
			name:        "empty scenario returns fresh",
			existing:    existing,
			fresh:       fresh,
			scenario:    "",
			wantCols:    []string{"default.general"},
			wantRoles:   []string{"default.nginx"},
			wantHash:    "new",
			wantToolVer: "10.0.0",
		},
		{
			name:        "unrelated scenario keeps everything",
			existing:    existing,
			fresh:       &LockFile{Hash: "new", Tools: fresh.Tools},
			scenario:    "other",
			wantCols:    []string{"default.general", "extra.posix", "default.crypto"},
			wantRoles:   []string{"extra.docker", "default.nginx"},
			wantHash:    "new",
			wantToolVer: "10.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeLockFileForScenario(tt.existing, tt.fresh, tt.scenario)
			if !reflect.DeepEqual(names(got.Collections), tt.wantCols) {
				t.Errorf("collections = %v, want %v", names(got.Collections), tt.wantCols)
			}
			if !reflect.DeepEqual(names(got.Roles), tt.wantRoles) {
				t.Errorf("roles = %v, want %v", names(got.Roles), tt.wantRoles)
			}
			if got.Hash != tt.wantHash {
				t.Errorf("hash = %q, want %q", got.Hash, tt.wantHash)
			}
			if len(got.Tools) != 1 || got.Tools[0].ResolvedVersion != tt.wantToolVer {
				t.Errorf("tools = %+v, want ansible %s", got.Tools, tt.wantToolVer)
			}
		})
	}

	// The existing lock file must not be mutated.
	if len(existing.Collections) != 3 || len(existing.Roles) != 2 {
		t.Fatalf("existing lock file was mutated: %+v", existing)
	}
}

func TestMergeLockFileForScenarioNilFresh(t *testing.T) {
	existing := &LockFile{Hash: "old"}
	if got := MergeLockFileForScenario(existing, nil, "default"); got != existing {
		t.Fatalf("expected existing lock file to be returned unchanged")
	}
}
