package tfprovider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure DiffusionProvider satisfies the provider.Provider interface.
var _ provider.Provider = &DiffusionProvider{}

// DiffusionProvider is the concrete provider implementation.
type DiffusionProvider struct {
	version string
}

// DiffusionProviderModel maps provider HCL configuration to Go values.
type DiffusionProviderModel struct {
	// DiffusionBinary is the path to the diffusion CLI binary.
	// Defaults to "diffusion" (resolved from $PATH).
	DiffusionBinary types.String `tfsdk:"diffusion_binary"`

	// Container registry settings (mirrors diffusion.toml [container_registry]).
	RegistryServer    types.String `tfsdk:"registry_server"`
	RegistryProvider  types.String `tfsdk:"registry_provider"`
	ContainerName     types.String `tfsdk:"container_name"`
	ContainerTag      types.String `tfsdk:"container_tag"`

	// HashiCorp Vault settings.
	VaultAddr  types.String `tfsdk:"vault_addr"`
	VaultToken types.String `tfsdk:"vault_token"`

	// Artifact sources for private Galaxy repos / git repos.
	ArtifactSources []ArtifactSourceModel `tfsdk:"artifact_source"`

	// Host reachability wait defaults (can be overridden per resource).
	HostWaitInitialDelay types.String `tfsdk:"host_wait_initial_delay"`
	HostWaitInterval     types.String `tfsdk:"host_wait_interval"`
	HostWaitTimeout      types.String `tfsdk:"host_wait_timeout"`
}

// ArtifactSourceModel maps a single artifact_source block.
type ArtifactSourceModel struct {
	Name               types.String `tfsdk:"name"`
	URL                types.String `tfsdk:"url"`
	Type               types.String `tfsdk:"type"`
	VaultPath          types.String `tfsdk:"vault_path"`
	VaultSecretName    types.String `tfsdk:"vault_secret_name"`
	VaultUsernameField types.String `tfsdk:"vault_username_field"`
	VaultTokenField    types.String `tfsdk:"vault_token_field"`
	UseVault           types.Bool   `tfsdk:"use_vault"`
}

// New returns a provider.Provider factory function.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &DiffusionProvider{version: version}
	}
}

// Metadata sets the provider type name and version.
func (p *DiffusionProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "diffusion"
	resp.Version = p.version
}

// Schema defines all provider-level configuration attributes.
func (p *DiffusionProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `The **diffusion** provider deploys Ansible roles to remote hosts using the
diffusion molecule container. It fetches ` + "`diffusion.lock`" + ` from remote role
repositories, merges dependency constraints, downloads collections and roles,
and executes ansible-playbook inside the diffusion container.

The ` + "`diffusion`" + ` CLI binary must be installed on the machine running Terraform.`,

		Attributes: map[string]schema.Attribute{
			"diffusion_binary": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Path to the `diffusion` CLI binary. Defaults to `diffusion` (resolved from `$PATH`).",
			},
			"registry_server": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Container registry server (e.g. `ghcr.io`). Defaults to `ghcr.io`.",
			},
			"registry_provider": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Registry provider: `Public`, `YC`, `AWS`, or `GCP`. Defaults to `Public`.",
			},
			"container_name": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Molecule container image name. Defaults to `polar-team/diffusion-molecule-container`.",
			},
			"container_tag": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Molecule container image tag. Defaults to `latest`.",
			},
			"vault_addr": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "HashiCorp Vault address (e.g. `https://vault.example.com`). Falls back to the `VAULT_ADDR` environment variable.",
			},
			"vault_token": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				MarkdownDescription: "HashiCorp Vault token. Falls back to the `VAULT_TOKEN` environment variable.",
			},
			"host_wait_initial_delay": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Default initial delay before the first host reachability probe (Go duration, e.g. `\"10s\"`). Default: `\"10s\"`.",
			},
			"host_wait_interval": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Default interval between host reachability probes (Go duration, e.g. `\"15s\"`). Default: `\"15s\"`.",
			},
			"host_wait_timeout": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Default hard deadline for host reachability (Go duration, e.g. `\"10m\"`). Default: `\"10m\"`.",
			},
		},

		Blocks: map[string]schema.Block{
			"artifact_source": schema.ListNestedBlock{
				MarkdownDescription: "Private artifact source for Galaxy credentials or git repo authentication. Mirrors `diffusion.toml` `[[artifact_sources]]`.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Unique name for this artifact source.",
						},
						"url": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "URL of the artifact source (Galaxy server or git remote).",
						},
						"type": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Source type: `galaxy` or `git`. Defaults to `galaxy`.",
						},
						"vault_path": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Vault KV v2 mount path (e.g. `secret`).",
						},
						"vault_secret_name": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Vault KV v2 secret name.",
						},
						"vault_username_field": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Field name in the Vault secret that holds the username.",
						},
						"vault_token_field": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Field name in the Vault secret that holds the token or password.",
						},
						"use_vault": schema.BoolAttribute{
							Optional:            true,
							MarkdownDescription: "When `true`, credentials are fetched from Vault. Otherwise local encrypted storage is used.",
						},
					},
				},
			},
		},
	}
}

// Configure validates provider config and stores it in the provider data passed
// to resources and data sources.
func (p *DiffusionProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data DiffusionProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Resolve diffusion binary path.
	binary := "diffusion"
	if !data.DiffusionBinary.IsNull() && !data.DiffusionBinary.IsUnknown() {
		binary = data.DiffusionBinary.ValueString()
	}

	// Resolve Vault credentials (provider config overrides env vars).
	vaultAddr := os.Getenv("VAULT_ADDR")
	if !data.VaultAddr.IsNull() && !data.VaultAddr.IsUnknown() && data.VaultAddr.ValueString() != "" {
		vaultAddr = data.VaultAddr.ValueString()
	}
	vaultToken := os.Getenv("VAULT_TOKEN")
	if !data.VaultToken.IsNull() && !data.VaultToken.IsUnknown() && data.VaultToken.ValueString() != "" {
		vaultToken = data.VaultToken.ValueString()
	}

	// Build provider-level runner config and pass to resources/data sources.
	providerCfg := &ProviderConfig{
		DiffusionBinary:      binary,
		RegistryServer:       stringOrDefault(data.RegistryServer, "ghcr.io"),
		RegistryProvider:     stringOrDefault(data.RegistryProvider, "Public"),
		ContainerName:        stringOrDefault(data.ContainerName, "polar-team/diffusion-molecule-container"),
		ContainerTag:         stringOrDefault(data.ContainerTag, "latest"),
		VaultAddr:            vaultAddr,
		VaultToken:           vaultToken,
		ArtifactSources:      data.ArtifactSources,
		HostWaitInitialDelay: stringOrDefault(data.HostWaitInitialDelay, "10s"),
		HostWaitInterval:     stringOrDefault(data.HostWaitInterval, "15s"),
		HostWaitTimeout:      stringOrDefault(data.HostWaitTimeout, "10m"),
	}

	resp.DataSourceData = providerCfg
	resp.ResourceData = providerCfg
}

// Resources lists all resources provided by this provider.
func (p *DiffusionProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewDeployResource,
	}
}

// DataSources lists all data sources provided by this provider.
func (p *DiffusionProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewInventoryDataSource,
	}
}

// ProviderConfig is the internal configuration passed from the provider to
// resources and data sources via Configure().
type ProviderConfig struct {
	DiffusionBinary      string
	RegistryServer       string
	RegistryProvider     string
	ContainerName        string
	ContainerTag         string
	VaultAddr            string
	VaultToken           string
	ArtifactSources      []ArtifactSourceModel
	HostWaitInitialDelay string
	HostWaitInterval     string
	HostWaitTimeout      string
}

func stringOrDefault(v types.String, def string) string {
	if v.IsNull() || v.IsUnknown() || v.ValueString() == "" {
		return def
	}
	return v.ValueString()
}
