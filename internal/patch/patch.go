package patch

// patch.go is the main patching module: it applies validated PatchBundles
// to an ephemeral staged copy of an installed external role.
//
// Flow: analyze (patch_analyzer.go) -> validate (patch_config.go) ->
// StageRoleForPatch -> ApplyBundle -> converge/test -> RevertPatches (or
// drop the stage). The source of truth is never mutated: all writes go to
// the staged copy, and a backup enables revert.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"diffusion/internal/config"
	"diffusion/internal/utils"
)

// ApplyOptions controls ApplyBundle.
type ApplyOptions struct {
	// Scenario defaults to the bundle's scenario (or "default").
	Scenario string
	// RolePath is the staged role directory to patch. Empty resolves the
	// installed role via ResolveInstalledRolePath (not recommended for
	// real runs: stage a copy first for ephemerality).
	RolePath string
	// DryRun computes changes without writing anything (no backup either).
	DryRun bool
	// Force applies despite stale-analysis drift (line/name mismatch).
	Force bool
	// BackupDir holds the pre-apply backup. Empty uses a temp dir.
	BackupDir string
}

// ApplyResult describes one applied bundle.
type ApplyResult struct {
	Bundle        string
	Role          string
	Scenario      string
	BackupPath    string
	Patched       []string // task/handler IDs with YAML mutations
	Overlaid      []string // "id <= patch/files|templates/name" entries
	Files         []string // changed files, role-relative slash paths
	DryRun        bool
	staleWarnings []string
}

// Summary renders a human-readable change report.
func (r *ApplyResult) Summary() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "bundle %q role %s (scenario %s)", r.Bundle, r.Role, r.Scenario)
	if r.DryRun {
		sb.WriteString(" [dry-run: no writes performed]")
	}
	sb.WriteString("\n")
	for _, p := range r.Patched {
		fmt.Fprintf(&sb, "  task %s\n", p)
	}
	for _, o := range r.Overlaid {
		fmt.Fprintf(&sb, "  task %s\n", o)
	}
	if len(r.Files) > 0 {
		fmt.Fprintf(&sb, "  files changed: %s\n", strings.Join(r.Files, ", "))
	}
	for _, wmsg := range r.staleWarnings {
		fmt.Fprintf(&sb, "  warning: %s\n", wmsg)
	}
	if r.BackupPath != "" {
		fmt.Fprintf(&sb, "  backup: %s\n", r.BackupPath)
	}
	return sb.String()
}

// overlaySourcePath resolves a patch-folder overlay file for scenario.
// Shared with patch_config.go validation so both agree on layout.
func overlaySourcePath(scenario, subdir, value string) string {
	if strings.TrimSpace(scenario) == "" {
		scenario = config.DefaultScenario
	}
	return filepath.Join("scenarios", strings.TrimSpace(scenario), "patch", subdir, filepath.FromSlash(value))
}

// MatchesRole reports whether the bundle targets the given role reference
// (full "namespace.name" or short name, case-insensitive).
func (b *PatchBundle) MatchesRole(role string) bool {
	if b == nil {
		return false
	}
	return roleNameMatches(b.RoleName, role)
}

// StageRoleForPatch copies an installed role to a temp staging dir for
// ephemeral patching. Returns the stage path and a cleanup func.
func StageRoleForPatch(installedPath string) (string, func(), error) {
	tmp, err := os.MkdirTemp("", "diffusion-patch-stage-")
	if err != nil {
		return "", func() {}, fmt.Errorf("failed to create stage dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	stage := filepath.Join(tmp, "role")
	if err := utils.CopyDir(installedPath, stage); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("failed to stage role: %w", err)
	}
	return stage, cleanup, nil
}

// backupRole copies rolePath into backupDir (or a temp dir) and returns
// the backup copy path for RevertPatches.
func backupRole(rolePath, backupDir string) (string, error) {
	if strings.TrimSpace(backupDir) == "" {
		tmp, err := os.MkdirTemp("", "diffusion-patch-backup-")
		if err != nil {
			return "", fmt.Errorf("failed to create backup dir: %w", err)
		}
		backupDir = tmp
	} else if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create backup dir: %w", err)
	}
	dst := filepath.Join(backupDir, "role")
	if err := utils.CopyDir(rolePath, dst); err != nil {
		return "", fmt.Errorf("failed to back up role: %w", err)
	}
	return dst, nil
}

// RevertPatches restores rolePath from a backup copy made by ApplyBundle.
func RevertPatches(backupPath, rolePath string) error {
	if strings.TrimSpace(backupPath) == "" || strings.TrimSpace(rolePath) == "" {
		return fmt.Errorf("backup path and role path are required")
	}
	if st, err := os.Stat(backupPath); err != nil || !st.IsDir() {
		return fmt.Errorf("backup not found: %s", backupPath)
	}
	if err := os.RemoveAll(rolePath); err != nil {
		return fmt.Errorf("failed to clear role path: %w", err)
	}
	if err := utils.CopyDir(backupPath, rolePath); err != nil {
		return fmt.Errorf("failed to restore role from backup: %w", err)
	}
	return nil
}

// CheckAgainst verifies bundle task IDs against an analysis: every ID must
// resolve to a leaf (branches rejected explicitly) and src overlays must
// match the leaf's recorded kind. Pure (no writes).
func (b *PatchBundle) CheckAgainst(a *RoleAnalysis) error {
	if b == nil {
		return fmt.Errorf("patch bundle is nil")
	}
	if a == nil {
		return fmt.Errorf("analysis is required")
	}
	for i := range b.TasksToPatch {
		pt := &b.TasksToPatch[i]
		canon, err := canonicalAnyID(pt.TaskId)
		if err != nil {
			return fmt.Errorf("patch bundle %q task %d: %w", b.PatchBundleName, i, err)
		}
		leaf := a.FindLeafByID(canon)
		if leaf == nil {
			if a.FindBranchByID(canon) != nil {
				return fmt.Errorf("patch bundle %q: task_id %q is a branch (visual-only, patch a leaf instead)", b.PatchBundleName, canon)
			}
			return fmt.Errorf("patch bundle %q: unknown task_id %q (re-run analyze?)", b.PatchBundleName, canon)
		}
		if err := overlayKindCheck(leaf, pt); err != nil {
			return fmt.Errorf("patch bundle %q task %s: %w", b.PatchBundleName, canon, err)
		}
	}
	return nil
}

// overlayKindCheck ensures a src overlay matches the leaf's recorded kind:
// NewFileSrc needs a file-module leaf with a resolvable src, NewTemplateSrc
// needs a template leaf. Only the src is ever replaced.
func overlayKindCheck(leaf *TaskNode, pt *PatchingTask) error {
	check := func(src, wantKind string) error {
		if src == "" {
			return nil
		}
		if leaf.Src == "" {
			return fmt.Errorf("task %s (%s) records no src to overlay", leaf.ID, leaf.Module)
		}
		if isJinja(leaf.Src) {
			return fmt.Errorf("task %s has a dynamic src %q (unsupported)", leaf.ID, leaf.Src)
		}
		if filepath.IsAbs(leaf.Src) {
			return fmt.Errorf("task %s has an absolute src %q (unsupported)", leaf.ID, leaf.Src)
		}
		if wantKind == ArtifactTemplate && leaf.Module != "template" {
			return fmt.Errorf("task %s uses module %q: use new_file_src for non-template tasks", leaf.ID, leaf.Module)
		}
		if wantKind == ArtifactFile && leaf.Module == "template" {
			return fmt.Errorf("task %s uses module template: use new_template_src", leaf.ID)
		}
		if wantKind == ArtifactFile && !fileModules[leaf.Module] {
			return fmt.Errorf("task %s uses module %q which references no files", leaf.ID, leaf.Module)
		}
		return nil
	}
	if err := check(strings.TrimSpace(pt.NewFileSrc), ArtifactFile); err != nil {
		return err
	}
	return check(strings.TrimSpace(pt.NewTemplateSrc), ArtifactTemplate)
}

// overlayTarget resolves the in-role destination of a leaf src:
// templates/<src> for template tasks, files/<src> otherwise.
func overlayTarget(roleRoot string, leaf *TaskNode) string {
	base := "files"
	if leaf.Module == "template" {
		base = "templates"
	}
	return filepath.Join(roleRoot, filepath.FromSlash(base), filepath.FromSlash(leaf.Src))
}

// scalarNode builds a string scalar node.
func scalarNode(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

// encodeValue builds a YAML node for any value.
func encodeValue(v any) *yaml.Node {
	n := &yaml.Node{}
	if err := n.Encode(v); err != nil {
		return scalarNode(fmt.Sprintf("%v", v))
	}
	return n
}

// setMappingKey sets (or appends) key=value in a mapping node.
func setMappingKey(mapNode *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(mapNode.Content); i += 2 {
		if mapNode.Content[i].Kind == yaml.ScalarNode && mapNode.Content[i].Value == key {
			mapNode.Content[i+1] = val
			return
		}
	}
	mapNode.Content = append(mapNode.Content, scalarNode(key), val)
}

// detectModuleKey returns the module key/value of a task mapping, or an
// error for action:-form tasks (unsupported in v1).
func detectModuleKey(taskNode *yaml.Node) (string, *yaml.Node, error) {
	order, vals := mappingEntries(taskNode)
	for _, k := range order {
		if reservedTaskKeys[k] {
			continue
		}
		return k, vals[k], nil
	}
	if mod, _ := indirectModule(vals); mod != "" {
		return "", nil, fmt.Errorf("action:/local_action: tasks are not supported in v1")
	}
	return "", nil, fmt.Errorf("no module found")
}

// buildConditions renders PatchConditions as a when: value node:
// "Condition Body", or bare Condition when Body is empty.
func buildConditions(conds []PatchConditions) *yaml.Node {
	items := make([]string, 0, len(conds))
	for _, c := range conds {
		cond, body := strings.TrimSpace(c.Condition), strings.TrimSpace(c.Body)
		if body == "" {
			items = append(items, cond)
		} else {
			items = append(items, cond+" "+body)
		}
	}
	if len(items) == 1 {
		return scalarNode(items[0])
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, it := range items {
		seq.Content = append(seq.Content, scalarNode(it))
	}
	return seq
}

// applyTaskMutation mutates one task mapping in place. Returns a short
// summary of what changed.
func applyTaskMutation(taskNode *yaml.Node, pt *PatchingTask) (string, error) {
	var changes []string
	if strings.TrimSpace(pt.NewModule) != "" || len(pt.NewModuleSetup) > 0 {
		modKey, modVal, err := detectModuleKey(taskNode)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(pt.NewModule) != "" {
			for i := 0; i+1 < len(taskNode.Content); i += 2 {
				if taskNode.Content[i].Kind == yaml.ScalarNode && taskNode.Content[i].Value == modKey {
					taskNode.Content[i].Value = strings.TrimSpace(pt.NewModule)
					break
				}
			}
			changes = append(changes, fmt.Sprintf("module %s->%s", modKey, strings.TrimSpace(pt.NewModule)))
			modKey = strings.TrimSpace(pt.NewModule)
		}
		if len(pt.NewModuleSetup) > 0 {
			if strings.TrimSpace(pt.NewModule) != "" {
				fresh := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				for _, kv := range pt.NewModuleSetup {
					fresh.Content = append(fresh.Content, scalarNode(keyString(kv.Key)), encodeValue(kv.Value))
				}
				for i := 0; i+1 < len(taskNode.Content); i += 2 {
					if taskNode.Content[i].Kind == yaml.ScalarNode && taskNode.Content[i].Value == modKey {
						taskNode.Content[i+1] = fresh
						break
					}
				}
				changes = append(changes, "module args replaced")
			} else {
				if modVal == nil || modVal.Kind != yaml.MappingNode {
					return "", fmt.Errorf("module args are not a mapping: use new_module to replace")
				}
				for _, kv := range pt.NewModuleSetup {
					setMappingKey(modVal, keyString(kv.Key), encodeValue(kv.Value))
				}
				changes = append(changes, "module args merged")
			}
		}
		_ = modVal
	}
	if len(pt.NewConditions) > 0 {
		setMappingKey(taskNode, "when", buildConditions(pt.NewConditions))
		changes = append(changes, fmt.Sprintf("when set (%d)", len(pt.NewConditions)))
	}
	if pt.NewBecome != nil {
		if strings.TrimSpace(pt.NewBecome.User) != "" {
			setMappingKey(taskNode, "become_user", scalarNode(strings.TrimSpace(pt.NewBecome.User)))
			changes = append(changes, "become_user set")
		}
		if pt.NewBecome.Become {
			setMappingKey(taskNode, "become", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"})
			changes = append(changes, "become set")
		}
	}
	if len(pt.NewEnvironment) > 0 {
		env := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, kv := range pt.NewEnvironment {
			env.Content = append(env.Content, scalarNode(keyString(kv.Key)), encodeValue(kv.Value))
		}
		setMappingKey(taskNode, "environment", env)
		changes = append(changes, "environment set")
	}
	if len(pt.NewBlock) > 0 {
		return "", fmt.Errorf("new_block apply is not supported in v1")
	}
	return strings.Join(changes, "; "), nil
}

// keyString stringifies a module/environment key (validated non-nil).
func keyString(k any) string {
	if s, ok := k.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprintf("%v", k))
}

// parsedFile caches one parsed YAML file for multi-op application.
type parsedFile struct {
	doc   *yaml.Node
	dirty bool
}

// locateTaskNode re-descends roleRoot along path and returns the parsed
// document, the target task mapping and the final file. The final node is
// verified against want (line + name) unless force skips the check.
func locateTaskNode(roleRoot string, path []FileIndex, want *TaskNode, force bool, cache map[string]*parsedFile) (*yaml.Node, *yaml.Node, string, error) {
	if len(path) == 0 {
		return nil, nil, "", fmt.Errorf("empty index path")
	}
	load := func(rel string) ([]*yaml.Node, *yaml.Node, error) {
		if pf, ok := cache[rel]; ok {
			return listOf(pf.doc), pf.doc, nil
		}
		data, err := os.ReadFile(filepath.Join(roleRoot, filepath.FromSlash(rel)))
		if err != nil {
			return nil, nil, fmt.Errorf("cannot read %s: %w", rel, err)
		}
		var doc yaml.Node
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, nil, fmt.Errorf("cannot parse %s: %w", rel, err)
		}
		if len(doc.Content) == 0 {
			return nil, nil, fmt.Errorf("empty document: %s", rel)
		}
		cache[rel] = &parsedFile{doc: &doc}
		return listOf(&doc), &doc, nil
	}
	var list []*yaml.Node
	var doc *yaml.Node
	curFile := ""
	var node *yaml.Node
	for hi, hop := range path {
		if hop.File != curFile || list == nil {
			var err error
			list, doc, err = load(hop.File)
			if err != nil {
				return nil, nil, "", err
			}
			curFile = hop.File
		}
		if hop.Index < 0 || hop.Index >= len(list) {
			return nil, nil, "", fmt.Errorf("task %s no longer exists in %s (role changed since analyze?)", want.ID, hop.File)
		}
		node = list[hop.Index]
		if node == nil || node.Kind != yaml.MappingNode {
			return nil, nil, "", fmt.Errorf("task %s: entry %d in %s is not a mapping (role changed?)", want.ID, hop.Index, hop.File)
		}
		if hi == len(path)-1 {
			break
		}
		next := path[hi+1]
		if next.File == curFile {
			sub, err := branchSubList(node)
			if err != nil {
				return nil, nil, "", fmt.Errorf("task %s: %w", want.ID, err)
			}
			list = sub
			continue
		}
		list = nil // next hop re-parses its own file
	}
	if !force {
		_, vals := mappingEntries(node)
		if node.Line != want.Line || scalarString(vals["name"]) != want.Name {
			return nil, nil, "", fmt.Errorf("task %s drifted since analyze (run analyze again or use --force)", want.ID)
		}
	}
	return doc, node, curFile, nil
}

// listOf returns the task sequence of a parsed document.
func listOf(doc *yaml.Node) []*yaml.Node {
	if doc == nil || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	switch root.Kind {
	case yaml.SequenceNode:
		return root.Content
	case yaml.MappingNode:
		return []*yaml.Node{root}
	default:
		return nil
	}
}

// branchSubList returns the concatenated child list of a block branch
// (block, rescue, always in analyzer order).
func branchSubList(node *yaml.Node) ([]*yaml.Node, error) {
	_, vals := mappingEntries(node)
	var sub []*yaml.Node
	for _, key := range []string{"block", "rescue", "always"} {
		if seq := vals[key]; seq != nil && seq.Kind == yaml.SequenceNode {
			sub = append(sub, seq.Content...)
		}
	}
	if sub == nil {
		return nil, fmt.Errorf("path traverses a non-branch task")
	}
	return sub, nil
}

// marshalTaskDoc encodes a task file document with 4-space indent.
func marshalTaskDoc(doc *yaml.Node) ([]byte, error) {
	var root *yaml.Node = doc
	if doc != nil && doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		root = doc.Content[0]
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(4)
	if err := enc.Encode(root); err != nil {
		_ = enc.Close()
		return nil, err
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}

// ApplyBundle applies one validated bundle to the staged role at
// opts.RolePath (resolved from the installed role when empty).
func ApplyBundle(bundle *PatchBundle, analysis *RoleAnalysis, opts ApplyOptions) (*ApplyResult, error) {
	if bundle == nil {
		return nil, fmt.Errorf("patch bundle is nil")
	}
	if err := bundle.validate(); err != nil {
		return nil, err
	}
	if analysis == nil {
		return nil, fmt.Errorf("analysis is required (run analyze first)")
	}
	scenario := strings.TrimSpace(opts.Scenario)
	if scenario == "" {
		scenario = strings.TrimSpace(bundle.Scenario)
	}
	rolePath := strings.TrimSpace(opts.RolePath)
	if rolePath == "" {
		var err error
		rolePath, err = ResolveInstalledRolePath(bundle.RoleName)
		if err != nil {
			return nil, err
		}
	}
	rolePath = filepath.Clean(rolePath)
	if st, err := os.Stat(rolePath); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("role path not found: %s", rolePath)
	}
	if !roleNameMatches(bundle.RoleName, analysis.RoleName) && strings.TrimSpace(analysis.RoleName) != "" {
		return nil, fmt.Errorf("analysis is for role %q, bundle targets %q", analysis.RoleName, bundle.RoleName)
	}
	if !opts.Force {
		if st, err := os.Stat(analysis.RolePath); err == nil && st.IsDir() &&
			filepath.Clean(analysis.RolePath) != rolePath {
			return nil, fmt.Errorf("analysis is for %s, refusing to patch %s (re-run analyze or use --force)", analysis.RolePath, rolePath)
		}
	}
	if err := bundle.CheckAgainst(analysis); err != nil {
		return nil, err
	}

	res := &ApplyResult{
		Bundle: bundle.PatchBundleName, Role: bundle.RoleName,
		Scenario: scenario, DryRun: opts.DryRun,
	}
	if !opts.DryRun {
		backup, err := backupRole(rolePath, opts.BackupDir)
		if err != nil {
			return nil, err
		}
		res.BackupPath = backup
	}

	type op struct {
		pt   *PatchingTask
		leaf *TaskNode
	}
	byFile := map[string][]op{}
	var order []string
	for i := range bundle.TasksToPatch {
		pt := &bundle.TasksToPatch[i]
		canon, _ := canonicalAnyID(pt.TaskId)
		leaf := analysis.FindLeafByID(canon)
		if leaf == nil { // guarded by CheckAgainst; kept for safety
			return nil, fmt.Errorf("unknown task_id %q", canon)
		}
		last := leaf.Path[len(leaf.Path)-1]
		if _, ok := byFile[last.File]; !ok {
			order = append(order, last.File)
		}
		byFile[last.File] = append(byFile[last.File], op{pt: pt, leaf: leaf})
	}

	cache := map[string]*parsedFile{}
	changed := map[string]bool{}
	overlayScenario := scenario
	if overlayScenario == "" {
		overlayScenario = config.DefaultScenario
	}
	for _, rel := range order {
		for _, o := range byFile[rel] {
			pt, leaf := o.pt, o.leaf
			canon, _ := canonicalAnyID(pt.TaskId)
			// Src overlays never touch YAML: copy overlay content over
			// the leaf's recorded src inside the staged copy.
			if v := strings.TrimSpace(pt.NewFileSrc); v != "" {
				if !opts.DryRun {
					if err := applyOverlay(overlaySourcePath(overlayScenario, "files", v), overlayTarget(rolePath, leaf)); err != nil {
						return nil, fmt.Errorf("task %s: %w", canon, err)
					}
				}
				res.Overlaid = append(res.Overlaid, canon+" <= patch/files/"+v)
				continue
			}
			if v := strings.TrimSpace(pt.NewTemplateSrc); v != "" {
				if !opts.DryRun {
					if err := applyOverlay(overlaySourcePath(overlayScenario, "templates", v), overlayTarget(rolePath, leaf)); err != nil {
						return nil, fmt.Errorf("task %s: %w", canon, err)
					}
				}
				res.Overlaid = append(res.Overlaid, canon+" <= patch/templates/"+v)
				continue
			}
			// YAML mutation via index-path re-descent.
			_, node, finalFile, err := locateTaskNode(rolePath, leaf.Path, leaf, opts.Force, cache)
			if err != nil {
				return nil, fmt.Errorf("task %s: %w", canon, err)
			}
			if opts.DryRun {
				res.Patched = append(res.Patched, canon)
				changed[finalFile] = true
				continue
			}
			summary, err := applyTaskMutation(node, pt)
			if err != nil {
				return nil, fmt.Errorf("task %s: %w", canon, err)
			}
			if summary == "" {
				summary = "patched"
			}
			res.Patched = append(res.Patched, canon+": "+summary)
			changed[finalFile] = true
			if pf, ok := cache[finalFile]; ok {
				pf.dirty = true
			}
		}
	}
	if !opts.DryRun {
		var files []string
		for rel, pf := range cache {
			if !pf.dirty {
				continue
			}
			data, err := marshalTaskDoc(pf.doc)
			if err != nil {
				return nil, fmt.Errorf("failed to encode %s: %w", rel, err)
			}
			if err := os.WriteFile(filepath.Join(rolePath, filepath.FromSlash(rel)), data, 0o644); err != nil {
				return nil, fmt.Errorf("failed to write %s: %w", rel, err)
			}
			files = append(files, rel)
		}
		sort.Strings(files)
		// Overlay-only runs touch no YAML; still report overlay targets.
		for rel := range changed {
			found := false
			for _, f := range files {
				if f == rel {
					found = true
					break
				}
			}
			if !found {
				if pf, ok := cache[rel]; ok && pf.dirty {
					files = append(files, rel)
				}
			}
		}
		sort.Strings(files)
		res.Files = files
	} else {
		for rel := range changed {
			res.Files = append(res.Files, rel)
		}
		sort.Strings(res.Files)
	}
	return res, nil
}

// applyOverlay copies overlay content over the in-role target, creating
// parent dirs. It preserves the overlay file's mode.
func applyOverlay(overlaySrc, target string) error {
	data, err := os.ReadFile(overlaySrc)
	if err != nil {
		return fmt.Errorf("cannot read overlay %s: %w", overlaySrc, err)
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(overlaySrc); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("cannot create overlay parent dir: %w", err)
	}
	if err := os.WriteFile(target, data, mode); err != nil {
		return fmt.Errorf("cannot write overlay target %s: %w", target, err)
	}
	return nil
}
