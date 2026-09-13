package patch

import (
	"fmt"
	yaml "gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"diffusion/internal/config"
)

type PatchesConfig struct {
	Bundles []PatchBundle `yaml:"Bundles,omitempty"`
}

type PatchBundle struct {
	PatchBundleName string
	TasksToPatch    []PatchingTask
	Scenario        string
	RoleName        string
}

type PatchingTask struct {
	TaskId         string             `yaml:"task_id"`
	NewModule      string             `yaml:"new_module,omitempty"`
	NewModuleSetup []PatchModuleSetup `yaml:"new_module_setup,omitempty"`
	NewConditions  []PatchConditions  `yaml:"new_conditions,omitempty"`
	NewBecome      *PatchBecomeSetup  `yaml:"new_become_user,omitempty"`
	NewEnvironment []PatchEnvironment `yaml:"new_environment,omitempty"`
	NewBlock       []PatchingTask     `yaml:"new_block,omitempty"`
	// NewFileSrc overlays a file from the scenario patch folder
	// (scenarios/<scenario>/patch/files/<value>) onto the leaf task's
	// recorded file src. Only the src is replaced; dest is unchanged.
	NewFileSrc string `yaml:"new_file_src,omitempty"`
	// NewTemplateSrc overlays a template from the scenario patch folder
	// (scenarios/<scenario>/patch/templates/<value>) onto the leaf task's
	// recorded template src. Only the src is replaced.
	NewTemplateSrc string `yaml:"new_template_src,omitempty"`
}

type PatchConditions struct {
	Condition string `yaml:"condition,omitempty"`
	Body      string `yaml:"body,omitempty"`
}

type PatchModuleSetup struct {
	Key   any `yaml:"key"`
	Value any `yaml:"value"`
}

type PatchBecomeSetup struct {
	User   string `yaml:"user,omitempty"`
	Become bool   `yaml:"become,omitempty"`
}

type PatchEnvironment struct {
	Key   any `yaml:"key"`
	Value any `yaml:"value"`
}

// maxTaskIDDepth caps dotted task ID depth (e.g. "1.3.1") to guard
// against pathological inputs.
const maxTaskIDDepth = 32

// ParseTaskID parses a dotted task ID ("1", "1.1", "1.3.1") into its
// numeric path segments. Branch prefixes come from include/block nesting
// (see patch_analyzer.go); leaves are the patchable tasks under the main
// plan. Each segment must be a positive integer without leading zeros.
// Surrounding whitespace is tolerated.
func ParseTaskID(taskID string) ([]int, error) {
	trimmed := strings.TrimSpace(taskID)
	if trimmed == "" {
		return nil, fmt.Errorf(config.ColorRed + "task id is empty" + config.ColorReset)
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) > maxTaskIDDepth {
		return nil, fmt.Errorf(config.ColorRed+"task id %q exceeds max depth of %d"+config.ColorReset, taskID, maxTaskIDDepth)
	}
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			return nil, fmt.Errorf(config.ColorRed+"invalid task id %q: empty segment"+config.ColorReset, taskID)
		}
		if len(p) > 1 && p[0] == '0' {
			return nil, fmt.Errorf(config.ColorRed+"invalid task id %q: segment %q has leading zero"+config.ColorReset, taskID, p)
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 {
			return nil, fmt.Errorf(config.ColorRed+"invalid task id %q: segment %q must be a positive integer"+config.ColorReset, taskID, p)
		}
		out = append(out, n)
	}
	return out, nil
}

// ParseTaskId is an alias of ParseTaskID using the "Id" spelling.
func ParseTaskId(taskID string) ([]int, error) {
	return ParseTaskID(taskID)
}

// ParentID returns the parent of a dotted task ID ("1.3.1" -> "1.3").
// It returns "" for top-level IDs ("1" -> "") and for invalid input.
func ParentId(taskID string) string {
	parts, err := ParseTaskID(taskID)
	if err != nil || len(parts) <= 1 {
		return ""
	}
	parent := make([]string, len(parts)-1)
	for i, n := range parts[:len(parts)-1] {
		parent[i] = strconv.Itoa(n)
	}
	return strings.Join(parent, ".")
}

// canonicalTaskID returns the normalized dotted form (" 1.2 " -> "1.2").
func canonicalTaskID(taskID string) (string, error) {
	parts, err := ParseTaskID(taskID)
	if err != nil {
		return "", err
	}
	str := make([]string, len(parts))
	for i, n := range parts {
		str[i] = strconv.Itoa(n)
	}
	return strings.Join(str, "."), nil
}

// roleNameMatches reports whether a bundle role reference identifies the
// same role as a diffusion.toml dependency entry (reconstructed as
// "namespace.rolename" or plain "rolename"). Comparison is
// case-insensitive. A short bundle name ("docker") matches a namespaced
// entry ("geerlingguy.docker"); a namespaced bundle name must match in
// full.
func roleNameMatches(bundleRole, requirementRole string) bool {
	b := strings.TrimSpace(bundleRole)
	r := strings.TrimSpace(requirementRole)
	if b == "" || r == "" {
		return false
	}
	if strings.EqualFold(b, r) {
		return true
	}
	if strings.Contains(b, ".") {
		return false
	}
	short := r
	if idx := strings.LastIndex(r, "."); idx != -1 {
		short = r[idx+1:]
	}
	return strings.EqualFold(b, short)
}

// validateRoleName checks that the bundle's RoleName identifies a role
// actually declared in diffusion.toml under [dependencies] for the
// bundle's scenario. Dependency entries are keyed "scenario.rolename",
// so the scenario prefix is stripped before matching.
func (b *PatchBundle) validateRoleName() error {
	roleName := strings.TrimSpace(b.RoleName)
	if roleName == "" {
		return fmt.Errorf(config.ColorRed+"patch bundle %q: role name is required"+config.ColorReset, b.PatchBundleName)
	}
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf(config.ColorRed+"patch bundle %q: cannot load diffusion.toml: %v"+config.ColorReset, b.PatchBundleName, err)
	}
	if cfg == nil || cfg.DependencyConfig == nil || len(cfg.DependencyConfig.Roles) == 0 {
		return fmt.Errorf(config.ColorRed+"patch bundle %q: no roles declared in diffusion.toml [dependencies]"+config.ColorReset, b.PatchBundleName)
	}
	scenario := strings.TrimSpace(b.Scenario)
	var available []string
	for i := range cfg.DependencyConfig.Roles {
		entry := &cfg.DependencyConfig.Roles[i]
		short := strings.TrimSpace(entry.Name)
		if scenario != "" {
			prefix := scenario + "."
			if !strings.HasPrefix(short, prefix) {
				continue
			}
			short = strings.TrimPrefix(short, prefix)
		} else if idx := strings.Index(short, "."); idx != -1 {
			// Unscoped bundle: accept entries from any scenario by
			// stripping the "scenario." prefix when present.
			short = short[idx+1:]
		}
		full := short
		if ns := strings.TrimSpace(entry.Namespace); ns != "" {
			full = ns + "." + short
		}
		available = append(available, full)
		if roleNameMatches(roleName, full) {
			return nil
		}
	}
	if len(available) == 0 {
		return fmt.Errorf(config.ColorRed+"patch bundle %q: no roles declared for scenario %q in diffusion.toml [dependencies]"+config.ColorReset,
			b.PatchBundleName, scenario)
	}
	return fmt.Errorf(config.ColorRed+"patch bundle %q: role %q not found in diffusion.toml [dependencies] for scenario %q (available roles: %s)"+config.ColorReset,
		b.PatchBundleName, roleName, scenario, strings.Join(available, ", "))
}

// splitTaskID separates the optional 'h' handler prefix from the numeric
// segments: "h2" -> (true, [2]); "1.3.1" -> (false, [1 3 1]). The prefix
// is case-insensitive and canonicalizes to lowercase "h".
func splitTaskID(taskID string) (handler bool, parts []int, err error) {
	trimmed := strings.TrimSpace(taskID)
	if len(trimmed) > 1 && (trimmed[0] == 'h' || trimmed[0] == 'H') {
		handler = true
		trimmed = strings.TrimSpace(trimmed[1:])
	}
	parts, err = ParseTaskID(trimmed)
	if err != nil {
		return false, nil, err
	}
	return handler, parts, nil
}

// canonicalAnyID returns the normalized form of a task or handler ID
// (" 1.2 " -> "1.2", "H2" -> "h2").
func canonicalAnyID(taskID string) (string, error) {
	handler, parts, err := splitTaskID(taskID)
	if err != nil {
		return "", err
	}
	str := make([]string, len(parts))
	for i, n := range parts {
		str[i] = strconv.Itoa(n)
	}
	out := strings.Join(str, ".")
	if handler {
		out = "h" + out
	}
	return out, nil
}

// parentAnyID returns the parent of a task or handler ID ("1.3.1" -> "1.3",
// "h1.2" -> "h1"). It returns "" for top-level IDs and invalid input.
func parentAnyID(taskID string) string {
	handler, parts, err := splitTaskID(taskID)
	if err != nil || len(parts) <= 1 {
		return ""
	}
	str := make([]string, len(parts)-1)
	for i, n := range parts[:len(parts)-1] {
		str[i] = strconv.Itoa(n)
	}
	out := strings.Join(str, ".")
	if handler {
		out = "h" + out
	}
	return out
}

// isHandlerID reports whether the ID addresses a handler ("h1", "h1.2").
func isHandlerID(taskID string) bool {
	handler, _, err := splitTaskID(taskID)
	return err == nil && handler
}

// validate implements Validate. scenario scopes overlay lookups under
// scenarios/<scenario>/patch/{files,templates} (empty means "default").
func (t *PatchingTask) validate(scenario string) error {
	if t == nil {
		return fmt.Errorf(config.ColorRed + "patching bundles is nil" + config.ColorReset)
	}
	id := strings.TrimSpace(t.TaskId)
	if id == "" {
		return fmt.Errorf(config.ColorRed + "patching task: task_id is required" + config.ColorReset)
	} else if _, _, err := splitTaskID(id); err != nil {
		return fmt.Errorf(config.ColorRed+"patching task %q: %w"+config.ColorReset, t.TaskId, err)
	}
	if t.NewModule == "" && len(t.NewModuleSetup) == 0 && len(t.NewConditions) == 0 &&
		t.NewBecome == nil && len(t.NewEnvironment) == 0 && len(t.NewBlock) == 0 &&
		strings.TrimSpace(t.NewFileSrc) == "" && strings.TrimSpace(t.NewTemplateSrc) == "" {
		return fmt.Errorf(config.ColorRed+"patching task %q: no patch action specified"+config.ColorReset, t.TaskId)
	}
	if strings.TrimSpace(t.NewFileSrc) != "" && strings.TrimSpace(t.NewTemplateSrc) != "" {
		return fmt.Errorf(config.ColorRed+"patching task %q: new_file_src and new_template_src are mutually exclusive"+config.ColorReset, t.TaskId)
	}
	if (strings.TrimSpace(t.NewFileSrc) != "" || strings.TrimSpace(t.NewTemplateSrc) != "") &&
		(t.NewModule != "" || len(t.NewModuleSetup) > 0) {
		return fmt.Errorf(config.ColorRed+"patching task %q: src overlay cannot be combined with new_module/new_module_setup"+config.ColorReset, t.TaskId)
	}
	if err := t.validateOverlays(scenario); err != nil {
		return err
	}
	for i := range t.NewConditions {
		c := &t.NewConditions[i]
		if strings.TrimSpace(c.Condition) == "" {
			return fmt.Errorf(config.ColorRed+"patching task %q: condition %d has empty condition"+config.ColorReset, t.TaskId, i)
		}
	}
	for i := range t.NewModuleSetup {
		if t.NewModuleSetup[i].Key == nil {
			return fmt.Errorf(config.ColorRed+"patching task %q: module setup %d has nil key"+config.ColorReset, t.TaskId, i)
		}
		if s, ok := t.NewModuleSetup[i].Key.(string); ok && strings.TrimSpace(s) == "" {
			return fmt.Errorf(config.ColorRed+"patching task %q: module setup %d has empty key"+config.ColorReset, t.TaskId, i)
		}
	}
	for i := range t.NewEnvironment {
		if t.NewEnvironment[i].Key == nil {
			return fmt.Errorf(config.ColorRed+"patching task %q: environment %d has nil key"+config.ColorReset, t.TaskId, i)
		}
		if s, ok := t.NewEnvironment[i].Key.(string); ok && strings.TrimSpace(s) == "" {
			return fmt.Errorf(config.ColorRed+"patching task %q: environment %d has empty key"+config.ColorReset, t.TaskId, i)
		}
	}
	if t.NewBecome != nil && strings.TrimSpace(t.NewBecome.User) == "" && !t.NewBecome.Become {
		return fmt.Errorf(config.ColorRed+"patching task %q: become setup sets neither user nor become"+config.ColorReset, t.TaskId)
	}
	for i := range t.NewBlock {
		if err := t.NewBlock[i].validate(scenario); err != nil {
			return fmt.Errorf(config.ColorRed+"patching task %q block %d: %w"+config.ColorReset, t.TaskId, i, err)
		}
	}
	return nil
}

// validateOverlays checks that file/template src overrides point at real
// files inside the scenario patch folder:
// scenarios/<scenario>/patch/files|templates/<value>. Only the src is
// replaced at apply time; absolute paths and escapes are rejected.
func (t *PatchingTask) validateOverlays(scenario string) error {
	if strings.TrimSpace(scenario) == "" {
		scenario = config.DefaultScenario
	}
	check := func(value, subdir, field string) error {
		v := strings.TrimSpace(value)
		if v == "" {
			return nil
		}
		if filepath.IsAbs(v) {
			return fmt.Errorf(config.ColorRed+"patching task %q: %s must be relative, got %q"+config.ColorReset, t.TaskId, field, value)
		}
		rel := filepath.FromSlash(v)
		if clean := filepath.Clean(rel); clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf(config.ColorRed+"patching task %q: %s escapes the patch folder: %q"+config.ColorReset, t.TaskId, field, value)
		}
		full := overlaySourcePath(scenario, subdir, v)
		info, err := os.Stat(full)
		if err != nil {
			return fmt.Errorf(config.ColorRed+"patching task %q: %s overlay not found: %s"+config.ColorReset, t.TaskId, field, full)
		}
		if info.IsDir() {
			return fmt.Errorf(config.ColorRed+"patching task %q: %s overlay is a directory: %s"+config.ColorReset, t.TaskId, field, full)
		}
		return nil
	}
	if err := check(t.NewFileSrc, "files", "new_file_src"); err != nil {
		return err
	}
	return check(t.NewTemplateSrc, "templates", "new_template_src")
}

// Validate reports whether the bundle is well-formed: non-empty name,
// at least one task, no duplicate task IDs, and every task valid.
func (b *PatchBundle) validate() error {
	if b == nil {
		return fmt.Errorf(config.ColorRed + "patch bundle is nil" + config.ColorReset)
	}
	if strings.TrimSpace(b.PatchBundleName) == "" {
		return fmt.Errorf(config.ColorRed + "patch bundle: bundle name is required" + config.ColorReset)
	}
	if err := b.validateRoleName(); err != nil {
		return err
	}
	if len(b.TasksToPatch) == 0 {
		return fmt.Errorf(config.ColorRed+"patch bundle %q: no tasks to patch"+config.ColorReset, b.PatchBundleName)
	}
	seen := make(map[string]struct{}, len(b.TasksToPatch))
	scenario := strings.TrimSpace(b.Scenario)
	for i := range b.TasksToPatch {
		task := &b.TasksToPatch[i]
		if err := task.validate(scenario); err != nil {
			return fmt.Errorf(config.ColorRed+"patch bundle %q task %d: %w"+config.ColorReset, b.PatchBundleName, i, err)
		}
		canon, err := canonicalAnyID(task.TaskId)
		if err != nil {
			return fmt.Errorf(config.ColorRed+"patch bundle %q task %d: %w"+config.ColorReset, b.PatchBundleName, i, err)
		}
		if _, dup := seen[canon]; dup {
			return fmt.Errorf(config.ColorRed+"patch bundle %q: duplicate task_id %q"+config.ColorReset, b.PatchBundleName, canon)
		}
		seen[canon] = struct{}{}
	}
	return nil
}

// Validate reports whether the patches config is well-formed: bundle
// names are unique and every bundle validates. An empty config (no
// bundles) is valid and means "nothing to patch".
func (p *PatchesConfig) validate() error {
	if p == nil {
		return fmt.Errorf("patches config is nil")
	}
	seen := make(map[string]struct{}, len(p.Bundles))
	for i := range p.Bundles {
		b := &p.Bundles[i]
		name := strings.TrimSpace(b.PatchBundleName)
		if name != "" {
			if _, dup := seen[name]; dup {
				return fmt.Errorf("duplicate patch bundle name %q", name)
			}
			seen[name] = struct{}{}
		}
		if err := b.validate(); err != nil {
			return err
		}
	}
	return nil
}

func (p *PatchesConfig) LoadPatchConfigFrom(scenarioName string) (*PatchesConfig, error) {
	data, err := os.ReadFile(filepath.Join("scenarios", scenarioName, "patch.yml"))
	if err != nil {
		return nil, fmt.Errorf(config.ColorRed+"failed to read patch config file: %v"+config.ColorReset, err)
	}

	var configMap *PatchesConfig
	if err := yaml.Unmarshal(data, &configMap); err != nil {
		return nil, fmt.Errorf(config.ColorRed+"failed to unmarshal patch config: %v"+config.ColorReset, err)
	}
	if err := configMap.validate(); err != nil {
		return nil, fmt.Errorf(config.ColorRed+"invalid patch config: %v"+config.ColorReset, err)
	}
	return configMap, nil
}

func (p *PatchesConfig) SavePatchConfigTo(scenarioName string) error {
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get project directory: %v", err)
	}
	configPath := filepath.Join(projectDir, "scenarios", scenarioName, "patch.yml")

	newData, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf(config.ColorRed+"failed to marshal patch config: %v"+config.ColorReset, err)
	}

	if err := os.WriteFile(configPath, newData, 0644); err != nil {
		return fmt.Errorf(config.ColorRed+"failed to write patch config file: %v"+config.ColorReset, err)
	}

	return nil

}
