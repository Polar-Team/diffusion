package tfprovider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func nullString() types.String  { return types.StringNull() }
func emptyString() types.String { return types.StringValue("") }
func setString(v string) types.String { return types.StringValue(v) }

// ---------------------------------------------------------------------------
// DiffusionProvider — Metadata + Schema
// ---------------------------------------------------------------------------

func TestProvider_Metadata(t *testing.T) {
	p := &DiffusionProvider{version: "0.1.0"}
	req := provider.MetadataRequest{}
	resp := &provider.MetadataResponse{}
	p.Metadata(context.Background(), req, resp)
	if resp.TypeName != "diffusion" {
		t.Errorf("expected TypeName='diffusion', got %q", resp.TypeName)
	}
	if resp.Version != "0.1.0" {
		t.Errorf("expected Version='0.1.0', got %q", resp.Version)
	}
}

func TestProvider_Schema_RequiredAttributes(t *testing.T) {
	p := &DiffusionProvider{}
	req := provider.SchemaRequest{}
	resp := &provider.SchemaResponse{}
	p.Schema(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() returned errors: %v", resp.Diagnostics)
	}

	expectedAttrs := []string{
		"diffusion_binary",
		"registry_server",
		"registry_provider",
		"container_name",
		"container_tag",
		"vault_addr",
		"vault_token",
		"host_wait_initial_delay",
		"host_wait_interval",
		"host_wait_timeout",
	}
	for _, name := range expectedAttrs {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("provider schema missing attribute %q", name)
		}
	}
}

func TestProvider_Schema_ArtifactSourceBlock(t *testing.T) {
	p := &DiffusionProvider{}
	req := provider.SchemaRequest{}
	resp := &provider.SchemaResponse{}
	p.Schema(context.Background(), req, resp)

	if _, ok := resp.Schema.Blocks["artifact_source"]; !ok {
		t.Fatal("provider schema missing 'artifact_source' block")
	}
}

func TestProvider_Resources(t *testing.T) {
	p := &DiffusionProvider{}
	factories := p.Resources(context.Background())
	if len(factories) == 0 {
		t.Fatal("expected at least one resource factory")
	}
	for i, f := range factories {
		if r := f(); r == nil {
			t.Errorf("resource factory[%d] returned nil", i)
		}
	}
}

func TestProvider_DataSources(t *testing.T) {
	p := &DiffusionProvider{}
	factories := p.DataSources(context.Background())
	if len(factories) == 0 {
		t.Fatal("expected at least one data source factory")
	}
	for i, f := range factories {
		if ds := f(); ds == nil {
			t.Errorf("data source factory[%d] returned nil", i)
		}
	}
}

func TestProvider_New(t *testing.T) {
	factory := New("1.2.3")
	p := factory()
	if p == nil {
		t.Fatal("New() returned nil provider")
	}
}

// ---------------------------------------------------------------------------
// stringOrDefault
// ---------------------------------------------------------------------------

func TestStringOrDefault_Null(t *testing.T) {
	got := stringOrDefault(nullString(), "fallback")
	if got != "fallback" {
		t.Errorf("expected 'fallback' for null string, got %q", got)
	}
}

func TestStringOrDefault_EmptyValue(t *testing.T) {
	got := stringOrDefault(emptyString(), "fallback")
	if got != "fallback" {
		t.Errorf("expected 'fallback' for empty string, got %q", got)
	}
}

func TestStringOrDefault_SetValue(t *testing.T) {
	got := stringOrDefault(setString("hello"), "fallback")
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestStringOrDefault_Unknown(t *testing.T) {
	got := stringOrDefault(types.StringUnknown(), "fallback")
	if got != "fallback" {
		t.Errorf("expected 'fallback' for unknown string, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// DeployResource — Schema
// ---------------------------------------------------------------------------

func TestDeployResource_Metadata(t *testing.T) {
	r := NewDeployResource()
	req := resource.MetadataRequest{ProviderTypeName: "diffusion"}
	resp := &resource.MetadataResponse{}
	r.Metadata(context.Background(), req, resp)
	if resp.TypeName != "diffusion_deploy" {
		t.Errorf("expected TypeName='diffusion_deploy', got %q", resp.TypeName)
	}
}

func TestDeployResource_Schema_TopLevelAttributes(t *testing.T) {
	dr := &DeployResource{}
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	dr.Schema(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() errors: %v", resp.Diagnostics)
	}

	expected := []string{
		"role_sources",
		"playbook",
		"hosts",
		"groups",
		"variables",
		"extra_vars",
		"skip_if_succeeded_within",
		"host_wait_initial_delay",
		"host_wait_interval",
		"host_wait_timeout",
		"run_id",
		"last_deployed",
		"merged_lock_hash",
		"inventory_rendered",
	}
	for _, name := range expected {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("deploy resource schema missing attribute %q", name)
		}
	}
}

func TestDeployResource_Schema_ComputedAttributeDescriptions(t *testing.T) {
	dr := &DeployResource{}
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	dr.Schema(context.Background(), req, resp)

	for _, name := range []string{"run_id", "last_deployed", "merged_lock_hash", "inventory_rendered"} {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Errorf("missing computed attribute %q", name)
			continue
		}
		if attr.GetMarkdownDescription() == "" {
			t.Errorf("computed attribute %q has empty description", name)
		}
	}
}

func TestDeployResource_Schema_RoleSourcesPresent(t *testing.T) {
	dr := &DeployResource{}
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	dr.Schema(context.Background(), req, resp)

	if _, ok := resp.Schema.Attributes["role_sources"]; !ok {
		t.Fatal("schema missing 'role_sources' attribute")
	}
}

func TestDeployResource_Schema_NoPathAttributes(t *testing.T) {
	// Ensure removed attributes (roles_path, collections_path) are absent.
	dr := &DeployResource{}
	req := resource.SchemaRequest{}
	resp := &resource.SchemaResponse{}
	dr.Schema(context.Background(), req, resp)

	for _, removed := range []string{"roles_path", "collections_path"} {
		if _, ok := resp.Schema.Attributes[removed]; ok {
			t.Errorf("unexpected removed attribute %q is still in schema", removed)
		}
	}
}

// ---------------------------------------------------------------------------
// InventoryDataSource — Schema
// ---------------------------------------------------------------------------

func TestInventoryDataSource_Metadata(t *testing.T) {
	ds := NewInventoryDataSource()
	req := datasource.MetadataRequest{ProviderTypeName: "diffusion"}
	resp := &datasource.MetadataResponse{}
	ds.Metadata(context.Background(), req, resp)
	if resp.TypeName != "diffusion_inventory" {
		t.Errorf("expected TypeName='diffusion_inventory', got %q", resp.TypeName)
	}
}

func TestInventoryDataSource_Schema(t *testing.T) {
	ids := &InventoryDataSource{}
	req := datasource.SchemaRequest{}
	resp := &datasource.SchemaResponse{}
	ids.Schema(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() errors: %v", resp.Diagnostics)
	}
	for _, name := range []string{"hosts", "groups", "variables", "rendered"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("inventory data source schema missing attribute %q", name)
		}
	}
}

// ---------------------------------------------------------------------------
// ProviderConfig defaults
// ---------------------------------------------------------------------------

func TestProviderConfig_DefaultValues(t *testing.T) {
	cfg := &ProviderConfig{
		DiffusionBinary:      stringOrDefault(nullString(), "diffusion"),
		RegistryServer:       stringOrDefault(nullString(), "ghcr.io"),
		RegistryProvider:     stringOrDefault(nullString(), "Public"),
		ContainerName:        stringOrDefault(nullString(), "polar-team/diffusion-molecule-container"),
		ContainerTag:         stringOrDefault(nullString(), "latest"),
		HostWaitInitialDelay: stringOrDefault(nullString(), "10s"),
		HostWaitInterval:     stringOrDefault(nullString(), "15s"),
		HostWaitTimeout:      stringOrDefault(nullString(), "10m"),
	}
	cases := map[string]string{
		"binary":   cfg.DiffusionBinary,
		"registry": cfg.RegistryServer,
		"provider": cfg.RegistryProvider,
		"delay":    cfg.HostWaitInitialDelay,
		"interval": cfg.HostWaitInterval,
		"timeout":  cfg.HostWaitTimeout,
	}
	want := map[string]string{
		"binary":   "diffusion",
		"registry": "ghcr.io",
		"provider": "Public",
		"delay":    "10s",
		"interval": "15s",
		"timeout":  "10m",
	}
	for k, v := range cases {
		if v != want[k] {
			t.Errorf("%s: got %q, want %q", k, v, want[k])
		}
	}
}
