// Package patch implements ephemeral patching of external Ansible roles.
//
// patch_analyzer.go inspects a role's tasks/ tree and enumerates every
// executable leaf task with a stable dotted ID:
//
//	tasks/main.yml top-level tasks          -> 1, 2, 3, ...
//	tasks included from branch 1            -> 1.1, 1.2, ...
//	tasks included from nested branch 1.3   -> 1.3.1, 1.3.2, ...
//	block/rescue/always subtasks of task 2  -> 2.1, 2.2, ...
//
// Branches (include_tasks/import_tasks/include_role/import_role/block) are
// visual-only groupings used by PrintTree: they are never patch targets and
// patch.go must reject them. Leaves (real module executions) are the
// patchable units under the main plan; file/template sources and handler
// notifications are recorded as evidence on the owning leaf.
package patch

import (
	"fmt"
	"hash/fnv"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"diffusion/internal/cache"
	"diffusion/internal/config"
	"diffusion/internal/utils"
)

// Artifact kinds recorded as evidence on a leaf task.
const (
	ArtifactFile     = "file"
	ArtifactTemplate = "template"
	ArtifactHandler  = "handler"
)

// Branch kinds for visual-only grouping nodes.
const (
	BranchIncludeTasks = "include_tasks"
	BranchImportTasks  = "import_tasks"
	BranchIncludeRole  = "include_role"
	BranchImportRole   = "import_role"
	BranchBlock        = "block"
)

const ansiReset = "\033[0m"

// branchPalette holds candidate colors for top-level branches. Status
// colors (red/green/yellow) are deliberately excluded so errors, matches
// and warnings stay readable against branch colors.
var branchPalette = []string{
	config.ColorPAnalyzerCyan,
	config.ColorPAnalyzerPink,
	config.ColorPAnalyzerPurple,
	config.ColorPAnalyzerOrange,
	config.ColorPAnalyzerSky,
	config.ColorPAnalyzerMagenta,
	config.ColorPAnalyzerLime,
	config.ColorPAnalyzerViolet,
	config.ColorPAnalyzerTeal,
	config.ColorPAnalyzerPaleYellow,
	config.ColorPAnalyzerCoral,
	config.ColorPAnalyzerGold,
	config.ColorPAnalyzerBlue,
	config.ColorPAnalyzerOrchid,
	config.ColorPAnalyzerYellowGreen,
	config.ColorPAnalyzerSalmon,
	config.ColorPAnalyzerLightSteelBlue,
	config.ColorPAnalyzerGreen,
}

// FileIndex locates one task statement: Index-th entry of File's task
// list (0-based). A leaf's Path chains these hops from the entry file,
// so patch.go can re-descend to the exact YAML node.
type FileIndex struct {
	File  string `yaml:"file"`
	Index int    `yaml:"index"`
}

// ArtifactLeaf is display evidence attached to a leaf task: a file or
// template reference, or a handler notification. Leaves are addressable;
// artifacts are not (they share the owning leaf's ID).
type ArtifactLeaf struct {
	Kind string `yaml:"kind"`
	Ref  string `yaml:"ref"`
	Dest string `yaml:"dest,omitempty"`
}

// TaskNode is one patchable leaf task.
type TaskNode struct {
	ID     string   `yaml:"id"`
	File   string   `yaml:"file"` // role-relative, slash-separated
	Line   int      `yaml:"line"`
	Name   string   `yaml:"name,omitempty"`
	Module string   `yaml:"module"`
	Src    string   `yaml:"src,omitempty"`
	Dest   string   `yaml:"dest,omitempty"`
	Notify []string `yaml:"notify,omitempty"`
	// Listen topics this handler responds to (handlers only).
	Listen    []string       `yaml:"listen,omitempty"`
	Tags      []string       `yaml:"tags,omitempty"`
	Branch    string         `yaml:"branch,omitempty"` // enclosing top-level branch ("1"), "" for top-level leaves
	Artifacts []ArtifactLeaf `yaml:"artifacts,omitempty"`
	Path      []FileIndex    `yaml:"path,omitempty"` // hop chain from the entry file to this task
}

// BranchNode is a visual-only include/block grouping. It is never a patch
// target: patch.go resolves dotted IDs through branches to leaves.
type BranchNode struct {
	ID      string      `yaml:"id"`
	File    string      `yaml:"file"` // file holding the include/block statement
	Line    int         `yaml:"line"`
	Name    string      `yaml:"name,omitempty"`
	Kind    string      `yaml:"kind"`
	Target  string      `yaml:"target,omitempty"` // included file (role-relative) or role name
	Opaque  bool        `yaml:"opaque,omitempty"` // dynamic/missing/cyclic target: not expanded
	Tags    []string    `yaml:"tags,omitempty"`
	Notify  []string    `yaml:"notify,omitempty"`
	Warning string      `yaml:"warning,omitempty"`
	Color   string      `yaml:"-"`
	Path    []FileIndex `yaml:"path,omitempty"` // hop chain from the entry file to this statement
}

// RoleAnalysis is the result of analyzing one role's tasks/ tree.
type RoleAnalysis struct {
	RoleName  string              `yaml:"role_name"`
	RolePath  string              `yaml:"role_path"`
	Scenario  string              `yaml:"scenario"`
	Leaves    []*TaskNode         `yaml:"leaves"`
	Branches  []*BranchNode       `yaml:"branches"`
	Files     map[string][]string `yaml:"files,omitempty"`
	Templates map[string][]string `yaml:"templates,omitempty"`
	Handlers  map[string][]string `yaml:"handlers,omitempty"`
	Warnings  []string            `yaml:"warnings,omitempty"`
}

// TreeOptions controls PrintTree rendering.
type TreeOptions struct {
	// HideTags suppresses tag markers (shown by default).
	HideTags bool
	// FilterTags highlights leaves/branches carrying any of these tags.
	FilterTags []string
	// NoColor disables all ANSI colors (use for CI/piped output).
	NoColor bool
	// Overlays annotates leaves with overlay sources: leaf ID (any form)
	// -> display path (e.g. "patch/files/banner.custom"), rendered as
	// "<= <path>" on the leaf line.
	Overlays map[string]string
}

type RoleDownloaderContainerConfig struct {
	ContainerRegistry *config.ContainerRegistry
}

// reservedTaskKeys are task mapping keys that never denote a module.
var reservedTaskKeys = map[string]bool{
	"name": true, "when": true, "tags": true, "notify": true,
	"become": true, "become_user": true, "become_method": true, "become_flags": true,
	"environment": true, "register": true, "failed_when": true, "changed_when": true,
	"ignore_errors": true, "ignore_unreachable": true, "check_mode": true, "diff": true,
	"run_once": true, "delegate_to": true, "loop": true, "loop_control": true,
	"with_items": true, "with_dict": true, "with_fileglob": true, "with_nested": true,
	"with_together": true, "with_subelements": true, "with_sequence": true,
	"with_random_choice": true, "with_first_found": true, "with_inventory_hostnames": true,
	"until": true, "retries": true, "delay": true, "async": true, "poll": true,
	"no_log": true, "debugger": true, "any_errors_fatal": true, "collections": true,
	"vars": true, "args": true, "action": true, "local_action": true, "listen": true,
	"block": true, "rescue": true, "always": true,
	"include_tasks": true, "import_tasks": true, "include_role": true, "import_role": true,
	"include": true, "import_playbook": true,
}

// fileModules are leaf modules whose src/dest evidence is tracked.
var fileModules = map[string]bool{
	"template": true, "copy": true, "file": true, "get_url": true,
	"unarchive": true, "assemble": true, "synchronize": true, "patch": true,
}

// normalizeModule strips the ansible.builtin. collection prefix so that
// "ansible.builtin.copy" and "copy" classify identically.
func normalizeModule(name string) string {
	if rest, ok := strings.CutPrefix(name, "ansible.builtin."); ok {
		return rest
	}
	return name
}

// isJinja reports whether a value contains an unrenderable Jinja expression.
func isJinja(s string) bool {
	return strings.Contains(s, "{{") || strings.Contains(s, "{%")
}

// dottedID renders numeric path segments as a dotted task ID.
func dottedID(level []int) string {
	parts := make([]string, len(level))
	for i, n := range level {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ".")
}

// scalarString returns the scalar value of n or "".
func scalarString(n *yaml.Node) string {
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
}

// parseTags parses an Ansible tags value (scalar or list) into tag names.
func parseTags(n *yaml.Node) []string {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.ScalarNode {
		return splitTagString(n.Value)
	}
	if n.Kind == yaml.SequenceNode {
		var out []string
		for _, item := range n.Content {
			if s := strings.TrimSpace(scalarString(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// splitTagString splits "a, b" or "a" into tags.
func splitTagString(s string) []string {
	var out []string
	for p := range strings.SplitSeq(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// parseNotify parses an Ansible notify value (scalar or list) into names.
func parseNotify(n *yaml.Node) []string {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.ScalarNode {
		if s := strings.TrimSpace(n.Value); s != "" {
			return []string{s}
		}
		return nil
	}
	if n.Kind == yaml.SequenceNode {
		var out []string
		for _, item := range n.Content {
			if s := strings.TrimSpace(scalarString(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// moduleArgs decodes a module value node into a parameter map. Mapping
// values decode directly; legacy "k=v k2=v2" strings are split on fields.
func moduleArgs(n *yaml.Node) map[string]any {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.MappingNode {
		var m map[string]any
		if err := n.Decode(&m); err == nil && m != nil {
			return m
		}
		return nil
	}
	if n.Kind == yaml.ScalarNode {
		out := map[string]any{}
		for field := range strings.FieldsSeq(n.Value) {
			k, v, ok := strings.Cut(field, "=")
			if !ok || k == "" {
				continue
			}
			out[k] = strings.Trim(v, `"'`)
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

// stringParam returns the first non-empty string parameter found under keys.
func stringParam(args map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := args[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

// walker accumulates analysis state while expanding the tasks/ tree.
type walker struct {
	roleRoot   string
	idPrefix   string // "h" while expanding handlers/, "" for tasks/
	leaves     []*TaskNode
	branches   []*BranchNode
	files      map[string][]string
	templates  map[string][]string
	handlers   map[string][]string
	warnings   []string
	branchByID map[string]*BranchNode
	stack      []string // in-progress files for cycle detection (role-relative, slash)
}

// idOf renders numeric segments with the walker's namespace prefix.
func (w *walker) idOf(level []int) string {
	id := dottedID(level)
	if w.idPrefix != "" {
		id = w.idPrefix + id
	}
	return id
}

// extendPath returns base plus one hop, without aliasing base.
func extendPath(base []FileIndex, file string, index int) []FileIndex {
	out := make([]FileIndex, len(base)+1)
	copy(out, base)
	out[len(base)] = FileIndex{File: file, Index: index}
	return out
}

// warn records a diagnostic without aborting the walk.
func (w *walker) warn(format string, args ...any) {
	w.warnings = append(w.warnings, fmt.Sprintf(format, args...))
}

// onStack reports whether rel is currently being expanded.
func (w *walker) onStack(rel string) bool {
	return slices.Contains(w.stack, rel)
}

// insideRoot reports whether target (joined to roleRoot) stays inside roleRoot.
func (w *walker) insideRoot(targetRel string) bool {
	rel, err := filepath.Rel(w.roleRoot, filepath.Join(w.roleRoot, filepath.FromSlash(targetRel)))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// walkFile expands one tasks/*.yml or handlers/*.yml file, numbering its
// entries under prefix. base is the hop chain leading to this file.
func (w *walker) walkFile(rel string, prefix []int, base []FileIndex) {
	full := filepath.Join(w.roleRoot, filepath.FromSlash(rel))
	data, err := os.ReadFile(full)
	if err != nil {
		w.warn("%s: cannot read: %v", rel, err)
		return
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		w.warn("%s: cannot parse YAML: %v", rel, err)
		return
	}
	if len(doc.Content) == 0 {
		return
	}
	root := doc.Content[0]
	var items []*yaml.Node
	switch root.Kind {
	case yaml.SequenceNode:
		items = root.Content
	case yaml.MappingNode:
		items = []*yaml.Node{root} // lenient: single-task file
	default:
		w.warn("%s: expected a task list", rel)
		return
	}
	w.stack = append(w.stack, rel)
	defer func() { w.stack = w.stack[:len(w.stack)-1] }()
	w.walkList(items, rel, prefix, base)
}

// mappingEntries returns the key order and first-seen value per key.
func mappingEntries(n *yaml.Node) ([]string, map[string]*yaml.Node) {
	var order []string
	vals := map[string]*yaml.Node{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k := scalarString(n.Content[i])
		if k == "" {
			continue
		}
		if _, seen := vals[k]; !seen {
			order = append(order, k)
			vals[k] = n.Content[i+1]
		}
	}
	return order, vals
}

// walkList enumerates one task list, assigning IDs prefix+index.
func (w *walker) walkList(items []*yaml.Node, fileRel string, prefix []int, base []FileIndex) {
	for i, item := range items {
		level := append(append([]int{}, prefix...), i+1)
		id := w.idOf(level)
		hop := extendPath(base, fileRel, i)
		if item == nil || item.Kind != yaml.MappingNode {
			w.warn("%s: entry %d is not a task mapping, skipped", fileRel, i+1)
			continue
		}
		_, vals := mappingEntries(item)
		name := scalarString(vals["name"])
		tags := parseTags(vals["tags"])
		notify := parseNotify(vals["notify"])

		// block/rescue/always form a transparent branch: subtasks are
		// numbered continuously across block, rescue, always.
		if vals["block"] != nil || vals["rescue"] != nil || vals["always"] != nil {
			w.addBranch(&BranchNode{
				ID: id, File: fileRel, Line: item.Line, Name: name,
				Kind: BranchBlock, Tags: tags, Notify: notify, Path: hop,
			})
			var sub []*yaml.Node
			for _, key := range []string{"block", "rescue", "always"} {
				if seq := vals[key]; seq != nil && seq.Kind == yaml.SequenceNode {
					sub = append(sub, seq.Content...)
				}
			}
			w.walkList(sub, fileRel, level, hop)
			continue
		}

		// include_tasks/import_tasks form a file branch.
		if mod, key := w.branchKey(vals, BranchIncludeTasks, BranchImportTasks); key != "" {
			w.walkInclude(id, hop, fileRel, item.Line, name, mod, vals[key], tags, notify)
			continue
		}

		// include_role/import_role are opaque branches in v1: the target
		// lives outside this role's tasks/ tree.
		if mod, key := w.branchKey(vals, BranchIncludeRole, BranchImportRole); key != "" {
			target := w.roleTarget(vals[key])
			w.addBranch(&BranchNode{
				ID: id, File: fileRel, Line: item.Line, Name: name,
				Kind: mod, Target: target, Opaque: true, Tags: tags, Notify: notify, Path: hop,
			})
			continue
		}

		w.addLeaf(id, level, hop, fileRel, item, vals, name, tags, notify)
	}
}

// branchKey returns the normalized module name when one of the given branch
// modules is present in the task mapping.
func (w *walker) branchKey(vals map[string]*yaml.Node, mods ...string) (string, string) {
	for k := range vals {
		norm := normalizeModule(k)
		for _, m := range mods {
			if norm == m {
				return m, k
			}
		}
	}
	return "", ""
}

// walkInclude expands an include_tasks/import_tasks branch or records it
// as opaque when the target is dynamic, missing, cyclic or out of root.
func (w *walker) walkInclude(id string, hop []FileIndex, fileRel string, line int, name, mod string, val *yaml.Node, tags, notify []string) {
	target, ok := includeTarget(val)
	branch := &BranchNode{
		ID: id, File: fileRel, Line: line, Name: name,
		Kind: mod, Tags: tags, Notify: notify, Path: hop,
	}
	if !ok || strings.TrimSpace(target) == "" {
		branch.Opaque = true
		branch.Warning = "cannot determine include target"
		w.warn("%s:%d (%s): cannot determine include target", fileRel, line, id)
		w.addBranch(branch)
		return
	}
	target = strings.TrimSpace(target)
	if isJinja(target) {
		branch.Opaque = true
		branch.Target = target
		branch.Warning = "dynamic include target, not expanded"
		w.warn("%s:%d (%s): dynamic include target %q, not expanded", fileRel, line, id, target)
		w.addBranch(branch)
		return
	}
	targetRel := filepath.ToSlash(filepath.Join(filepath.Dir(fileRel), filepath.FromSlash(target)))
	if !w.insideRoot(targetRel) {
		branch.Opaque = true
		branch.Target = target
		branch.Warning = "include target escapes role root"
		w.warn("%s:%d (%s): include target %q escapes role root", fileRel, line, id, target)
		w.addBranch(branch)
		return
	}
	if w.onStack(targetRel) {
		branch.Opaque = true
		branch.Target = targetRel
		branch.Warning = "include cycle detected"
		w.warn("%s:%d (%s): include cycle at %q", fileRel, line, id, targetRel)
		w.addBranch(branch)
		return
	}
	if _, err := os.Stat(filepath.Join(w.roleRoot, filepath.FromSlash(targetRel))); err != nil {
		branch.Opaque = true
		branch.Target = targetRel
		branch.Warning = "include target not found"
		w.warn("%s:%d (%s): include target %q not found", fileRel, line, id, targetRel)
		w.addBranch(branch)
		return
	}
	branch.Target = targetRel
	w.addBranch(branch)
	w.walkFile(targetRel, mustParseParts(id), hop)
}

// includeTarget extracts the included file from an include_tasks value:
// either a scalar path or a mapping with a file: key.
func includeTarget(val *yaml.Node) (string, bool) {
	if val == nil {
		return "", false
	}
	if val.Kind == yaml.ScalarNode {
		return val.Value, true
	}
	if val.Kind == yaml.MappingNode {
		_, vals := mappingEntries(val)
		if f := vals["file"]; f != nil && f.Kind == yaml.ScalarNode {
			return f.Value, true
		}
	}
	return "", false
}

// roleTarget extracts the role name from an include_role value.
func (w *walker) roleTarget(val *yaml.Node) string {
	if val == nil {
		return ""
	}
	if val.Kind == yaml.ScalarNode {
		return strings.TrimSpace(val.Value)
	}
	if val.Kind == yaml.MappingNode {
		_, vals := mappingEntries(val)
		return strings.TrimSpace(scalarString(vals["name"]))
	}
	return ""
}

// mustParseParts converts a walker-assigned ID back to numeric segments,
// dropping the handler prefix. Walker IDs are always well-formed by
// construction; on failure it returns nil and the caller treats the
// subtree as top-level.
func mustParseParts(id string) []int {
	_, parts, err := splitTaskID(id)
	if err != nil {
		return nil
	}
	return parts
}

// checkNotifies warns about notify targets with no handler definition
// (neither a matching handler name nor a listen topic).
func (w *walker) checkNotifies() {
	for _, l := range w.leaves {
		for _, n := range l.Notify {
			if len(handlerDefIDs(w.handlers, n)) == 0 {
				w.warn("%s:%d (%s): notifies undefined handler %q", l.File, l.Line, l.ID, n)
			}
		}
	}
	for _, b := range w.branches {
		for _, n := range b.Notify {
			if len(handlerDefIDs(w.handlers, n)) == 0 {
				w.warn("%s:%d (%s): notifies undefined handler %q", b.File, b.Line, b.ID, n)
			}
		}
	}
}

// handlerDefIDs returns handler definition leaf IDs (h-prefix) indexed
// under name (handler name or listen topic).
func handlerDefIDs(index map[string][]string, name string) []string {
	var out []string
	for _, id := range index[name] {
		if handler, _, _ := splitTaskID(id); handler {
			out = append(out, id)
		}
	}
	return out
}

// addBranch registers a visual-only branch node.
func (w *walker) addBranch(b *BranchNode) {
	w.branches = append(w.branches, b)
	w.branchByID[b.ID] = b
}

// addLeaf records one patchable leaf task with its evidence.
func (w *walker) addLeaf(id string, level []int, hop []FileIndex, fileRel string, item *yaml.Node, vals map[string]*yaml.Node, name string, tags, notify []string) {
	order, _ := mappingEntries(item)
	module := "unknown"
	var argsNode *yaml.Node
	for _, k := range order {
		if reservedTaskKeys[k] {
			continue
		}
		module = normalizeModule(k)
		argsNode = vals[k]
		break
	}
	// action:/local_action: indirect module forms.
	if module == "unknown" {
		if mod, args := indirectModule(vals); mod != "" {
			module = mod
			argsNode = args
		}
	}
	if module == "unknown" {
		w.warn("%s:%d (%s): no module detected", fileRel, item.Line, id)
	}
	args := moduleArgs(argsNode)
	leaf := &TaskNode{
		ID: id, File: fileRel, Line: item.Line, Name: name,
		Module: module, Notify: notify, Tags: tags, Path: hop,
		Listen: parseNotify(vals["listen"]),
	}
	if len(level) > 1 {
		leaf.Branch = strconv.Itoa(level[0])
	}
	if fileModules[module] {
		src := stringParam(args, "src")
		dest := stringParam(args, "dest", "path", "name")
		leaf.Src, leaf.Dest = src, dest
		kind := ArtifactFile
		if module == "template" {
			kind = ArtifactTemplate
		}
		ref := src
		if ref == "" {
			ref = dest
		}
		if ref != "" {
			leaf.Artifacts = append(leaf.Artifacts, ArtifactLeaf{Kind: kind, Ref: ref, Dest: dest})
			if kind == ArtifactTemplate {
				w.templates[ref] = append(w.templates[ref], id)
			} else {
				w.files[ref] = append(w.files[ref], id)
			}
		}
	}
	for _, n := range notify {
		leaf.Artifacts = append(leaf.Artifacts, ArtifactLeaf{Kind: ArtifactHandler, Ref: n})
		w.handlers[n] = append(w.handlers[n], id)
	}
	if w.idPrefix == "h" && strings.TrimSpace(name) != "" {
		// Handler definition site: visible next to notifier IDs so the
		// Handlers map shows both who notifies and where it is defined.
		w.handlers[name] = append(w.handlers[name], id)
	}
	if w.idPrefix == "h" {
		// Handler listen topics answer notifies addressed at the topic.
		for _, topic := range leaf.Listen {
			duplicate := false
			duplicate = slices.ContainsFunc(w.handlers[topic], func(existing string) bool { return existing == id })
			if !duplicate {
				w.handlers[topic] = append(w.handlers[topic], id)
			}
		}
	}
	w.leaves = append(w.leaves, leaf)
}

// indirectModule resolves action:/local_action: indirect module references.
func indirectModule(vals map[string]*yaml.Node) (string, *yaml.Node) {
	for _, key := range []string{"action", "local_action"} {
		n := vals[key]
		if n == nil {
			continue
		}
		if n.Kind == yaml.ScalarNode {
			fields := strings.Fields(n.Value)
			if len(fields) > 0 {
				return normalizeModule(fields[0]), nil
			}
			continue
		}
		if n.Kind == yaml.MappingNode {
			_, sub := mappingEntries(n)
			if m := scalarString(sub["module"]); m != "" {
				return normalizeModule(m), n
			}
		}
	}
	return "", nil
}

// AnalyzeRoleAtPath analyzes the role directory at rolePath. Unlike
// AnalyzeExternalRole it performs no resolution or installation: the
// caller supplies the exact directory (e.g. a staged copy or --path).
func AnalyzeRoleAtPath(rolePath, scenario, roleName string) (*RoleAnalysis, error) {
	return analyzeRoleAtPath(rolePath, scenario, roleName)
}

// analyzeRoleAtPath inspects the role directory at rolePath and enumerates
// its leaf tasks with dotted IDs. Task IDs are plain ("1.3.1"); handler
// IDs carry the "h" prefix ("h1", "h1.2").
func analyzeRoleAtPath(rolePath, scenario, roleName string) (*RoleAnalysis, error) {
	root := filepath.Clean(rolePath)
	entry := filepath.Join(root, "tasks", "main.yml")
	if _, err := os.Stat(entry); err != nil {
		return nil, fmt.Errorf("no tasks/main.yml under %s: not an analyzable role", root)
	}
	if strings.TrimSpace(roleName) == "" {
		roleName = filepath.Base(root)
	}
	w := &walker{
		roleRoot:   root,
		leaves:     []*TaskNode{},
		branches:   []*BranchNode{},
		files:      map[string][]string{},
		templates:  map[string][]string{},
		handlers:   map[string][]string{},
		branchByID: map[string]*BranchNode{},
	}
	w.walkFile(filepath.ToSlash("tasks/main.yml"), nil, nil)
	if _, err := os.Stat(filepath.Join(root, "handlers", "main.yml")); err == nil {
		w.idPrefix = "h"
		w.walkFile(filepath.ToSlash("handlers/main.yml"), nil, nil)
		w.idPrefix = ""
	}
	w.checkNotifies()
	a := &RoleAnalysis{
		RoleName: strings.TrimSpace(roleName), RolePath: root, Scenario: scenario,
		Leaves: w.leaves, Branches: w.branches,
		Files: w.files, Templates: w.templates, Handlers: w.handlers,
		Warnings: w.warnings,
	}
	AssignBranchColors(a, scenario+"\x00"+root)
	return a, nil
}

// AnalyzeExternalRole analyzes the installed role roleName for scenario.
// Resolution order: host search (diffusion cache, molecule working copies,
// Galaxy roles path), then a read-only copy-out of the role from a running
// molecule container. Nothing is ever installed or downloaded.
func AnalyzeExternalRole(scenario, roleName string) (*RoleAnalysis, error) {
	if err := utils.ValidateCLIArgument("role", roleName); err != nil {
		return nil, err
	}
	if path, err := ResolveInstalledRolePath(roleName, scenario); err == nil {
		return analyzeRoleAtPath(path, scenario, roleName)
	}
	container, err := FindMoleculeContainer()
	if err != nil {
		return nil, fmt.Errorf("role %q is not installed on this host: %w", roleName, err)
	}
	path, cleanup, err := CopyRoleFromContainer(container, roleName)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return analyzeRoleAtPath(path, scenario, roleName)
}

// resolveRoleCandidates lists installed-role search paths in priority
// order. Empty base dirs are skipped. Pure function for testability.
func resolveRoleCandidates(cacheDir, cwd, home, roleName string) []string {
	var out []string
	if cacheDir != "" {
		for _, v := range roleDirVariants(roleName) {
			out = append(out, filepath.Join(cacheDir, config.CacheRolesDir, v))
		}
	}
	if cwd != "" {
		for _, v := range roleDirVariants(roleName) {
			out = append(out, filepath.Join(cwd, config.MoleculeDir, v))
		}
	}
	if home != "" {
		for _, v := range roleDirVariants(roleName) {
			out = append(out, filepath.Join(home, ".ansible", "roles", v))
		}
	}
	return out
}

// shortRoleBases lists base dirs for short-name ("docker" -> "ns.docker")
// directory scans. Pure function for testability.
func shortRoleBases(cacheDir, cwd, home string) []string {
	var bases []string
	if cacheDir != "" {
		bases = append(bases, filepath.Join(cacheDir, config.CacheRolesDir))
	}
	if cwd != "" {
		bases = append(bases, filepath.Join(cwd, config.MoleculeDir))
	}
	if home != "" {
		bases = append(bases, filepath.Join(home, ".ansible", "roles"))
	}
	return bases
}

// findInstalledRole returns the first candidate holding a tasks/ dir,
// falling back to short-name directory scans. It returns the hit and the
// full tried list for error reporting.
func findInstalledRole(candidates, bases []string, roleName string) (string, []string) {
	tried := append([]string{}, candidates...)
	for _, p := range candidates {
		if st, err := os.Stat(filepath.Join(p, "tasks")); err == nil && st.IsDir() {
			return p, tried
		}
	}
	// Short names ("docker") also match namespaced install dirs
	// ("geerlingguy.docker") via directory scan.
	if !strings.Contains(roleName, ".") {
		short := strings.TrimSpace(roleName)
		for _, base := range bases {
			if p, ok := matchShortRoleDir(base, short); ok {
				tried = append(tried, p)
				return p, tried
			}
		}
	}
	return "", tried
}

// ResolveInstalledRolePath returns the installed directory of roleName.
//
// It is a pure search over already-installed locations (diffusion cache,
// cwd molecule working copies, user Galaxy roles path) with no side
// effects: no download, no container, no installation. Patching during a
// molecule run does not use it — see container.go, which patches inside
// the running molecule container. It stays available for inspection
// commands operating on a local copy.
func ResolveInstalledRolePath(roleName, scenario string) (string, error) {
	if err := utils.ValidateCLIArgument("role", roleName); err != nil {
		return "", err
	}
	if strings.TrimSpace(roleName) == "" {
		return "", fmt.Errorf("role name cannot be empty")
	}
	if err := utils.ValidateCLIArgument("scenario", scenario); err != nil {
		return "", err
	}

	var cacheDir string
	if cfg, err := config.LoadConfig(); err == nil && cfg != nil &&
		cfg.CacheConfig != nil && cfg.CacheConfig.Enabled && cfg.CacheConfig.CacheID != "" {
		cacheDir, _ = cache.GetCacheDir(cfg.CacheConfig.CacheID, cfg.CacheConfig.CachePath)
	}
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()

	candidates := resolveRoleCandidates(cacheDir, cwd, home, roleName)
	bases := shortRoleBases(cacheDir, cwd, home)
	hit, tried := findInstalledRole(candidates, bases, roleName)
	if hit == "" {
		return "", fmt.Errorf("role %q is not installed on this host (tried: %s)", roleName, strings.Join(tried, ", "))
	}
	return hit, nil
}

// matchShortRoleDir finds an installed dir for a short role name inside
// baseDir: exact match first, then "<namespace>.<short>".
func matchShortRoleDir(baseDir, short string) (string, bool) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if e.Name() == short {
			if st, err := os.Stat(filepath.Join(baseDir, e.Name(), "tasks")); err == nil && st.IsDir() {
				return filepath.Join(baseDir, e.Name()), true
			}
		}
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "."+short) {
			if st, err := os.Stat(filepath.Join(baseDir, e.Name(), "tasks")); err == nil && st.IsDir() {
				return filepath.Join(baseDir, e.Name()), true
			}
		}
	}
	return "", false
}

// roleDirVariants returns candidate directory names for a role reference:
// the full "namespace.name" form plus the short name.
func roleDirVariants(roleName string) []string {
	name := strings.TrimSpace(roleName)
	if name == "" {
		return nil
	}
	if strings.Contains(name, ".") {
		parts := strings.Split(name, ".")
		if short := parts[len(parts)-1]; short != "" && short != name {
			return []string{name, short}
		}
	}
	return []string{name}
}

// AnalyzeInstalledRole locates an installed external role and analyzes it.
// It is a thin wrapper over AnalyzeExternalRole kept for callers that
// already resolved nothing themselves.
func AnalyzeInstalledRole(scenario, roleName string) (*RoleAnalysis, error) {
	return AnalyzeExternalRole(scenario, roleName)
}

// FindLeafByID returns the leaf (task or handler) with the given ID
// (whitespace and non-canonical forms tolerated) or nil. Shared with
// patch.go so inspection and patching resolve IDs identically.
func (a *RoleAnalysis) FindLeafByID(id string) *TaskNode {
	if a == nil {
		return nil
	}
	canon, err := canonicalAnyID(id)
	if err != nil {
		return nil
	}
	for _, l := range a.Leaves {
		if l.ID == canon {
			return l
		}
	}
	return nil
}

// handlerDefs returns the handler definition leaves answering a notify
// name (handler name or listen topic).
func (a *RoleAnalysis) handlerDefs(name string) []*TaskNode {
	if a == nil {
		return nil
	}
	var out []*TaskNode
	for _, id := range handlerDefIDs(a.Handlers, name) {
		if l := a.FindLeafByID(id); l != nil {
			out = append(out, l)
		}
	}
	return out
}

// FindBranchByID returns the visual branch node with the given ID or nil.
func (a *RoleAnalysis) FindBranchByID(id string) *BranchNode {
	if a == nil {
		return nil
	}
	canon, err := canonicalAnyID(id)
	if err != nil {
		return nil
	}
	for _, b := range a.Branches {
		if b.ID == canon {
			return b
		}
	}
	return nil
}

// AssignBranchColors gives each top-level branch a distinct random-picked
// color. The shuffle is seeded by seed (scenario + role path), so the same
// role renders with stable colors across runs. A single branch always uses
// the default aquamarine; multiple branches draw without replacement.
func AssignBranchColors(a *RoleAnalysis, seed string) {
	if a == nil {
		return
	}
	var tops []string
	seen := map[string]bool{}
	for _, b := range a.Branches {
		if strings.Contains(b.ID, ".") || seen[b.ID] {
			continue
		}
		seen[b.ID] = true
		tops = append(tops, b.ID)
	}
	if len(tops) == 0 {
		return
	}
	colors := append([]string{}, branchPalette...)
	if len(tops) > 1 {
		h := fnv.New64a()
		_, _ = h.Write([]byte(seed))
		r := rand.New(rand.NewSource(int64(h.Sum64())))
		r.Shuffle(len(colors), func(i, j int) { colors[i], colors[j] = colors[j], colors[i] })
	} else {
		colors[0] = "\033[38;2;127;255;212m" // aquamarine single-branch default
	}
	topColor := map[string]string{}
	for i, id := range tops {
		topColor[id] = colors[i%len(colors)]
	}
	for _, b := range a.Branches {
		top := b.ID
		if idx := strings.Index(top, "."); idx != -1 {
			top = top[:idx]
		}
		b.Color = topColor[top]
	}
}

// colorize wraps s in code unless NoColor is set.
func colorize(s, code string, noColor bool) string {
	if noColor || code == "" {
		return s
	}
	return code + s + ansiReset
}

// treeRowKind classifies a structured tree row (see treeRow / buildTreeRows).
type treeRowKind int

const (
	rowHeader treeRowKind = iota
	rowBranch
	rowLeaf
	rowNotifyRef
	rowSection // "handlers:" / "warnings:"
	rowWarning
)

// treeRow is one structured row of the analyzed tree, shared by PrintTree
// (which formats it in FULL form) and the TUI (which formats it in SLIM
// form). Rows are built once by buildTreeRows from a single walk, so the
// two renderers can never drift relative to each other or to the
// underlying analysis.
type treeRow struct {
	Kind   treeRowKind
	Prefix string // tree indentation + glyph, e.g. "│   ├── "
	Color  string // subtree/branch colour ("" for none); always the
	// real colour regardless of NoColor -- formatting decides whether to
	// emit it.
	Branch *BranchNode // rowBranch
	Leaf   *TaskNode   // rowLeaf; rowNotifyRef: resolved handler def (nil if undefined)
	Notify string      // rowNotifyRef: the notify name
	Owner  string      // rowNotifyRef: ID of the branch/leaf that owns this notify
	Text   string      // rowHeader/rowSection/rowWarning text
	Match  bool        // --tag filter match (★)

	// Ancestor is the nearest enclosing tagged branch for a rowLeaf, used
	// both by PrintTree's "⇄ inherits" marker and by the TUI's Details
	// "inherits" field. nil when none or not a rowLeaf.
	Ancestor *BranchNode
	// Overlay is the resolved "<= <path>" overlay annotation for a
	// rowLeaf, "" when none.
	Overlay string
}

// notifyDep is one resolved (or undefined) notify target, used while
// building the ↳ rows under a branch/leaf that notifies handlers.
type notifyDep struct {
	name string
	leaf *TaskNode // nil when undefined
}

// appendNotifyRows appends one rowNotifyRef per resolved (or undefined)
// notify target of owner, in the given prefix/color, with ├──/└── glyphs
// assigned by position.
func appendNotifyRows(rows *[]treeRow, a *RoleAnalysis, ownerID, prefix, color string, notify []string) {
	var deps []notifyDep
	for _, n := range notify {
		defs := a.handlerDefs(n)
		if len(defs) == 0 {
			deps = append(deps, notifyDep{name: n})
			continue
		}
		for _, h := range defs {
			deps = append(deps, notifyDep{name: n, leaf: h})
		}
	}
	for di, d := range deps {
		glyph := "├── "
		if di == len(deps)-1 {
			glyph = "└── "
		}
		*rows = append(*rows, treeRow{
			Kind: rowNotifyRef, Prefix: prefix + glyph, Color: color,
			Leaf: d.leaf, Notify: d.name, Owner: ownerID,
		})
	}
}

// buildTreeRows walks a's branch/leaf tree once and returns the ordered,
// structured rows PrintTree and the TUI both render: task roots first (in
// the same order PrintTree has always used), then a "handlers:" section
// when handler leaves exist, then a "warnings:" section. This is the
// single source of truth for tree structure, sorting, colours and
// tag/overlay resolution; PrintTree and the TUI only differ in how they
// format each row's text.
func buildTreeRows(a *RoleAnalysis, opts TreeOptions) []treeRow {
	if a == nil {
		return nil
	}
	filter := map[string]bool{}
	for _, t := range opts.FilterTags {
		filter[t] = true
	}
	matches := func(tags []string) bool {
		if len(filter) == 0 {
			return false
		}
		for _, t := range tags {
			if filter[t] {
				return true
			}
		}
		return false
	}
	overlays := map[string]string{}
	for id, ref := range opts.Overlays {
		if canon, err := canonicalAnyID(id); err == nil {
			overlays[canon] = ref
		}
	}
	// subtreeColor returns the top-branch colour enclosing id, so every
	// row inside a branch subtree (nested branches and leaves) renders in
	// the branch colour. Top-level leaves and invalid IDs get "". Always
	// computed (independent of opts.NoColor): formatting decides whether
	// to actually emit it, so the same rows serve both colour and
	// NoColor renders.
	subtreeColor := func(id string) string {
		handler, parts, err := splitTaskID(id)
		if err != nil || len(parts) <= 1 {
			return ""
		}
		top := dottedID(parts[:1])
		if handler {
			top = "h" + top
		}
		if b := a.FindBranchByID(top); b != nil {
			return b.Color
		}
		return ""
	}

	type item struct {
		id     string
		branch *BranchNode
		leaf   *TaskNode
	}
	byID := map[string]*item{}
	children := map[string][]*item{}
	addChild := func(parent string, it *item) {
		byID[it.id] = it
		children[parent] = append(children[parent], it)
	}
	for _, b := range a.Branches {
		addChild(parentAnyID(b.ID), &item{id: b.ID, branch: b})
	}
	for _, l := range a.Leaves {
		addChild(parentAnyID(l.ID), &item{id: l.ID, leaf: l})
	}
	for parent := range children {
		sibs := children[parent]
		sort.Slice(sibs, func(i, j int) bool {
			_, pi, _ := splitTaskID(sibs[i].id)
			_, pj, _ := splitTaskID(sibs[j].id)
			if len(pi) == 0 || len(pj) == 0 {
				return sibs[i].id < sibs[j].id
			}
			return pi[len(pi)-1] < pj[len(pj)-1]
		})
	}
	// branchMatches reports whether a branch subtree or its own tags
	// match. Handler branches only see handler leaves (h-prefix), task
	// branches only task leaves.
	branchMatches := func(id string) bool {
		if b := a.FindBranchByID(id); b != nil && matches(b.Tags) {
			return true
		}
		handler, _, _ := splitTaskID(id)
		for _, l := range a.Leaves {
			lh, _, _ := splitTaskID(l.ID)
			if lh != handler {
				continue
			}
			if l.ID == id || strings.HasPrefix(l.ID, id+".") {
				if matches(l.Tags) {
					return true
				}
			}
		}
		return false
	}

	var rows []treeRow

	tasks, handlers := 0, 0
	for _, l := range a.Leaves {
		if handler, _, _ := splitTaskID(l.ID); handler {
			handlers++
		} else {
			tasks++
		}
	}
	rows = append(rows, treeRow{Kind: rowHeader, Text: fmt.Sprintf(
		"role %s %s (scenario: %s) — %d tasks, %d handlers, %d branches",
		a.RoleName, a.RolePath, a.Scenario, tasks, handlers, len(a.Branches),
	)})

	var render func(parent, prefix string)
	render = func(parent, prefix string) {
		sibs := children[parent]
		for i, it := range sibs {
			last := i == len(sibs)-1
			glyph, cont := "├── ", "│   "
			if last {
				glyph, cont = "└── ", "    "
			}
			if it.branch != nil {
				b := it.branch
				rows = append(rows, treeRow{
					Kind: rowBranch, Prefix: prefix + glyph, Color: b.Color, Branch: b,
					Match: len(filter) > 0 && branchMatches(b.ID),
				})
				if len(b.Notify) > 0 {
					appendNotifyRows(&rows, a, b.ID, prefix+cont, b.Color, b.Notify)
				}
				render(it.id, prefix+cont)
				continue
			}
			l := it.leaf
			rows = append(rows, treeRow{
				Kind: rowLeaf, Prefix: prefix + glyph, Color: subtreeColor(l.ID), Leaf: l,
				Ancestor: a.nearestTaggedBranch(l.ID),
				Overlay:  overlays[l.ID],
				Match:    len(filter) > 0 && matches(l.Tags),
			})
			if len(l.Notify) > 0 {
				appendNotifyRows(&rows, a, l.ID, prefix+cont, subtreeColor(l.ID), l.Notify)
			}
		}
	}

	// Tasks tree first, then the handlers section.
	var taskRoots, handlerRoots []string
	for _, it := range children[""] {
		if handler, _, _ := splitTaskID(it.id); handler {
			handlerRoots = append(handlerRoots, it.id)
		} else {
			taskRoots = append(taskRoots, it.id)
		}
	}
	saved := children[""]
	children[""] = nil
	for _, id := range taskRoots {
		children[""] = append(children[""], byID[id])
	}
	render("", "")
	if len(handlerRoots) > 0 {
		rows = append(rows, treeRow{Kind: rowSection, Text: "handlers:"})
		children[""] = nil
		for _, id := range handlerRoots {
			children[""] = append(children[""], byID[id])
		}
		render("", "")
	}
	children[""] = saved

	if len(a.Warnings) > 0 {
		rows = append(rows, treeRow{Kind: rowSection, Text: "warnings:"})
		for _, warn := range a.Warnings {
			rows = append(rows, treeRow{Kind: rowWarning, Text: warn})
		}
	}

	return rows
}

// showTagsFor reports whether tag markers should be rendered, matching
// PrintTree's historical rule.
func showTagsFor(opts TreeOptions) bool {
	return !opts.HideTags || len(opts.FilterTags) > 0
}

// formatFullBranch renders row (Kind == rowBranch) in PrintTree's
// historical full format.
func formatFullBranch(row treeRow, opts TreeOptions) string {
	b := row.Branch
	line := fmt.Sprintf("%s▶ %s [branch %s] %s:%d", row.Prefix, b.ID, b.Kind, b.File, b.Line)
	if b.Name != "" {
		line += " " + b.Name
	}
	if b.Target != "" {
		line += " -> " + b.Target
	}
	if b.Opaque {
		line += " (opaque"
		if b.Warning != "" {
			line += ": " + b.Warning
		}
		line += ")"
	}
	if showTagsFor(opts) {
		line += tagMarker(b.Tags, nil)
	}
	if row.Match {
		line += colorize(" ★", "\033[32m", opts.NoColor)
	}
	return colorize(line, b.Color, opts.NoColor)
}

// formatFullLeaf renders row (Kind == rowLeaf) in PrintTree's historical
// full format.
func formatFullLeaf(row treeRow, opts TreeOptions) string {
	l := row.Leaf
	line := fmt.Sprintf("%s● %s [%s] %s:%d", row.Prefix, l.ID, l.Module, l.File, l.Line)
	if l.Name != "" {
		line += " " + l.Name
	}
	if l.Src != "" {
		line += " src=" + l.Src
	}
	if l.Dest != "" {
		line += " dest=" + l.Dest
	}
	if len(l.Notify) > 0 {
		line += " notify=" + strings.Join(l.Notify, ",")
	}
	if row.Overlay != "" {
		line += " <= " + row.Overlay
	}
	if showTagsFor(opts) {
		line += tagMarker(l.Tags, row.Ancestor)
	}
	if row.Match {
		line += colorize(" ★", "\033[32m", opts.NoColor)
	}
	return colorize(line, row.Color, opts.NoColor)
}

// formatFullNotifyRef renders row (Kind == rowNotifyRef) in PrintTree's
// historical full format.
func formatFullNotifyRef(row treeRow, opts TreeOptions) string {
	var s string
	if row.Leaf == nil {
		s = fmt.Sprintf("↳ ? [handler] %s (undefined)", row.Notify)
	} else {
		h := row.Leaf
		s = fmt.Sprintf("↳ %s [%s] %s", h.ID, h.Module, h.Name)
		if showTagsFor(opts) && len(h.Tags) > 0 {
			s += tagMarker(h.Tags, nil)
		}
	}
	return colorize(row.Prefix+s, row.Color, opts.NoColor)
}

// nearestTaggedBranch returns the closest enclosing branch of leafID that
// carries tags, or nil.
func (a *RoleAnalysis) nearestTaggedBranch(leafID string) *BranchNode {
	for parent := parentAnyID(leafID); parent != ""; parent = parentAnyID(parent) {
		if b := a.FindBranchByID(parent); b != nil && len(b.Tags) > 0 {
			return b
		}
	}
	return nil
}

// branchChain returns the enclosing branch IDs of id from the outermost
// to the innermost (e.g. ["1", "1.3"] for a leaf under include_tasks 1
// then block 1.3), for the TUI's Details "branch" field.
func (a *RoleAnalysis) branchChain(id string) []*BranchNode {
	var chain []*BranchNode
	for parent := parentAnyID(id); parent != ""; parent = parentAnyID(parent) {
		if b := a.FindBranchByID(parent); b != nil {
			chain = append(chain, b)
		}
	}
	slices.Reverse(chain)
	return chain
}

// tagMarker builds the ◆/⇄ intersection marker for a leaf or branch.
func tagMarker(tags []string, ancestor *BranchNode) string {
	var sb strings.Builder
	if len(tags) > 0 {
		sb.WriteString(" ◆ tags:[")
		sb.WriteString(strings.Join(tags, ","))
		sb.WriteString("]")
	}
	if ancestor != nil {
		if len(tags) > 0 {
			sb.WriteString(" ⇄ ∩ [")
			sb.WriteString(strings.Join(ancestor.Tags, ","))
			sb.WriteString("]")
		} else {
			sb.WriteString(" ⇄ inherits [")
			sb.WriteString(strings.Join(ancestor.Tags, ","))
			sb.WriteString("]")
		}
	}
	return sb.String()
}

// PrintTree renders the branch/leaf tree with per-branch colors and tag
// intersection markers. Branches print with ▶, leaves with ●. Handler
// leaves render in a trailing "handlers:" section. Its output is built
// from buildTreeRows, the same structured rows the interactive TUI
// renders (in a slimmer format); this function must keep producing
// byte-for-byte identical output to before the row-model refactor (see
// the golden test in patch_tree_golden_test.go).
func (a *RoleAnalysis) PrintTree(w io.Writer, opts TreeOptions) {
	if a == nil || w == nil {
		return
	}
	for _, row := range buildTreeRows(a, opts) {
		var line string
		switch row.Kind {
		case rowHeader, rowSection:
			line = row.Text
		case rowBranch:
			line = formatFullBranch(row, opts)
		case rowLeaf:
			line = formatFullLeaf(row, opts)
		case rowNotifyRef:
			line = formatFullNotifyRef(row, opts)
		case rowWarning:
			line = "  - " + colorize(row.Text, config.ColorYellow, opts.NoColor)
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			fmt.Printf(config.ColorRed+"failed to write tree output: %v"+config.ColorReset+"\n", err)
		}
	}
}

// ToYAML serializes the analysis (leaves, branches, maps, warnings).
func (a *RoleAnalysis) ToYAML() ([]byte, error) {
	if a == nil {
		return nil, fmt.Errorf("analysis is nil")
	}
	return yaml.Marshal(a)
}
