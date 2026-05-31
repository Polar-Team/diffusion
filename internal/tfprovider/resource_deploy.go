package tfprovider

import (
	"context"
	"fmt"
	"strings"

	"diffusion/internal/deploy"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure DeployResource satisfies the resource.Resource interface.
var _ resource.Resource = &DeployResource{}

// DeployResource implements the diffusion_deploy resource.
type DeployResource struct {
	providerCfg *ProviderConfig
}

// NewDeployResource is the factory function registered with the provider.
func NewDeployResource() resource.Resource {
	return &DeployResource{}
}

// RoleSourceModel maps a single entry in the role_sources list.
type RoleSourceModel struct {
	SCM     types.String `tfsdk:"scm"`
	Version types.String `tfsdk:"version"`
	URL     types.String `tfsdk:"url"`
	Galaxy  types.String `tfsdk:"galaxy"`
	// Name overrides the role name used in the auto-generated playbook.
	Name types.String `tfsdk:"name"`
	// ApplyTo is the Ansible hosts pattern for the auto-generated play (default: "all").
	ApplyTo types.String `tfsdk:"apply_to"`
}

// DeployResourceModel maps the full diffusion_deploy HCL schema to Go values.
type DeployResourceModel struct {
	// Required
	RoleSources []RoleSourceModel `tfsdk:"role_sources"`

	// Optional — when omitted a playbook is auto-generated from role_sources.
	Playbook types.String `tfsdk:"playbook"`

	// Inventory
	Hosts     types.Map `tfsdk:"hosts"`
	Groups    types.Map `tfsdk:"groups"`
	Variables types.Map `tfsdk:"variables"`
	ExtraVars types.Map `tfsdk:"extra_vars"`

	// Idempotence
	SkipIfSucceededWithin types.String `tfsdk:"skip_if_succeeded_within"`

	// Host wait overrides (empty = use provider defaults)
	HostWaitInitialDelay types.String `tfsdk:"host_wait_initial_delay"`
	HostWaitInterval     types.String `tfsdk:"host_wait_interval"`
	HostWaitTimeout      types.String `tfsdk:"host_wait_timeout"`

	// Computed
	RunID             types.String `tfsdk:"run_id"`
	LastDeployed      types.String `tfsdk:"last_deployed"`
	MergedLockHash    types.String `tfsdk:"merged_lock_hash"`
	InventoryRendered types.String `tfsdk:"inventory_rendered"`
}

func (r *DeployResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_deploy"
}

func (r *DeployResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `**diffusion_deploy** deploys Ansible roles to remote hosts using the diffusion
molecule container.

It fetches ` + "`diffusion.lock`" + ` from each ` + "`role_sources`" + ` entry, merges dependency
constraints, and runs ` + "`ansible-playbook`" + ` inside the container. Roles and
collections are installed **inside the container** — nothing is downloaded to
the machine running Terraform.

**Playbook**: when ` + "`playbook`" + ` is omitted, a playbook is auto-generated that
applies each role to its ` + "`apply_to`" + ` hosts pattern (default: ` + "`\"all\"`" + `).

**State tracking**: ` + "`~/.diffusion/state`" + ` is written on every remote host after
each run. Use ` + "`skip_if_succeeded_within`" + ` to skip re-deployment when nothing
has changed.`,

		Attributes: map[string]schema.Attribute{
			// --- Role sources ---
			"role_sources": schema.ListNestedAttribute{
				Required:            true,
				MarkdownDescription: "List of remote role repositories to fetch `diffusion.lock` from.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"scm": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Source control type: `git` or `galaxy`.",
						},
						"version": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Version constraint or git ref (e.g. `>=2.0.0`, `main`, `v1.3.0`).",
						},
						"url": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Git repository URL. Required when `scm = \"git\"`.",
						},
						"galaxy": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Ansible Galaxy role name (`namespace.role_name`). Required when `scm = \"galaxy\"`.",
						},
						"name": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Role name override used in the auto-generated playbook. Defaults to the Galaxy name or git repo basename.",
						},
						"apply_to": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "Ansible hosts pattern for the auto-generated play that applies this role. Defaults to `\"all\"`.",
						},
					},
				},
			},

			// --- Playbook ---
			"playbook": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Path to an Ansible playbook (relative to the Terraform working directory or absolute). When omitted, a playbook is auto-generated from `role_sources`.",
			},

			// --- Inventory ---
			"hosts": schema.MapNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Map of hostname → host variables.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"vars": schema.MapAttribute{
							Optional:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Ansible connection variables (e.g. `ansible_host`, `ansible_user`).",
						},
					},
				},
			},
			"groups": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.ListType{ElemType: types.StringType},
				MarkdownDescription: "Map of group name → list of host names.",
			},
			"variables": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Global inventory variables applied to the `all` group.",
			},
			"extra_vars": schema.MapAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Extra variables passed to `ansible-playbook --extra-vars`.",
			},

			// --- Idempotence ---
			"skip_if_succeeded_within": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Skip re-deploy if the last run succeeded within this period and inputs are identical. Go duration string (e.g. `\"24h\"`). Empty = always deploy.",
			},

			// --- Host wait overrides ---
			"host_wait_initial_delay": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Override provider default initial delay before first host probe (Go duration).",
			},
			"host_wait_interval": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Override provider default interval between host probes (Go duration).",
			},
			"host_wait_timeout": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Override provider default hard deadline for host reachability (Go duration).",
			},

			// --- Computed ---
			"run_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SHA-256 hash of all deploy inputs. Stable for identical plans.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"last_deployed": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "RFC3339 timestamp of the last successful deploy.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"merged_lock_hash": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Hash of the merged `diffusion.lock` across all role sources.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"inventory_rendered": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The rendered Ansible YAML inventory (for inspection / debugging).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *DeployResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	cfg, ok := req.ProviderData.(*ProviderConfig)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data type",
			fmt.Sprintf("Expected *ProviderConfig, got %T", req.ProviderData))
		return
	}
	r.providerCfg = cfg
}

func (r *DeployResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data DeployResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.runDeploy(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DeployResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data DeployResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DeployResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data DeployResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.runDeploy(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete is a no-op — deployments are one-way operations.
func (r *DeployResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {}

// runDeploy assembles a DiffusionRunConfig and invokes the diffusion deploy CLI.
func (r *DeployResource) runDeploy(ctx context.Context, data *DeployResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	rc, d := r.buildRunConfig(ctx, data)
	diags.Append(d...)
	if diags.HasError() {
		return diags
	}

	result, err := RunDiffusionDeploy(ctx, rc)
	if err != nil {
		diags.AddError("Deploy failed", err.Error())
		return diags
	}

	data.RunID = types.StringValue(result.RunID)
	data.LastDeployed = types.StringValue(result.LastDeployed)
	data.MergedLockHash = types.StringValue(result.MergedLockHash)
	data.InventoryRendered = types.StringValue(result.InventoryRendered)
	return diags
}

// buildRunConfig converts the resource model into a DiffusionRunConfig.
func (r *DeployResource) buildRunConfig(ctx context.Context, data *DeployResourceModel) (DiffusionRunConfig, diag.Diagnostics) {
	var diags diag.Diagnostics
	cfg := DiffusionRunConfig{}

	if r.providerCfg != nil {
		cfg.DiffusionBinary = r.providerCfg.DiffusionBinary
		cfg.RegistryServer = r.providerCfg.RegistryServer
		cfg.RegistryProvider = r.providerCfg.RegistryProvider
		cfg.ContainerName = r.providerCfg.ContainerName
		cfg.ContainerTag = r.providerCfg.ContainerTag
		cfg.VaultAddr = r.providerCfg.VaultAddr
		cfg.VaultToken = r.providerCfg.VaultToken
		cfg.ArtifactSources = r.providerCfg.ArtifactSources
		cfg.HostWaitInitialDelay = r.providerCfg.HostWaitInitialDelay
		cfg.HostWaitInterval = r.providerCfg.HostWaitInterval
		cfg.HostWaitTimeout = r.providerCfg.HostWaitTimeout
	}

	// Per-resource wait overrides
	if !data.HostWaitInitialDelay.IsNull() && !data.HostWaitInitialDelay.IsUnknown() {
		cfg.HostWaitInitialDelay = data.HostWaitInitialDelay.ValueString()
	}
	if !data.HostWaitInterval.IsNull() && !data.HostWaitInterval.IsUnknown() {
		cfg.HostWaitInterval = data.HostWaitInterval.ValueString()
	}
	if !data.HostWaitTimeout.IsNull() && !data.HostWaitTimeout.IsUnknown() {
		cfg.HostWaitTimeout = data.HostWaitTimeout.ValueString()
	}

	cfg.Playbook = data.Playbook.ValueString()
	cfg.SkipIfSucceededWithin = data.SkipIfSucceededWithin.ValueString()

	// Role sources — validate and convert
	for i, rs := range data.RoleSources {
		scm := rs.SCM.ValueString()
		version := rs.Version.ValueString()
		url := rs.URL.ValueString()
		galaxy := rs.Galaxy.ValueString()
		name := rs.Name.ValueString()
		applyTo := rs.ApplyTo.ValueString()

		if scm == "" || version == "" {
			diags.AddError("Invalid role_sources entry",
				fmt.Sprintf("role_sources[%d]: 'scm' and 'version' are required", i))
			continue
		}
		switch strings.ToLower(scm) {
		case "git":
			if url == "" {
				diags.AddError("Invalid role_sources entry",
					fmt.Sprintf("role_sources[%d]: 'url' is required when scm=git", i))
			}
		case "galaxy":
			if galaxy == "" {
				diags.AddError("Invalid role_sources entry",
					fmt.Sprintf("role_sources[%d]: 'galaxy' is required when scm=galaxy", i))
			}
		default:
			diags.AddError("Invalid role_sources entry",
				fmt.Sprintf("role_sources[%d]: unsupported scm %q", i, scm))
		}

		cfg.RoleSources = append(cfg.RoleSources, RoleSourceEntry{
			SCM:     scm,
			Version: version,
			URL:     url,
			Galaxy:  galaxy,
			Name:    name,
			ApplyTo: applyTo,
		})
	}
	if diags.HasError() {
		return cfg, diags
	}

	// Inventory
	hosts, d := extractHosts(ctx, data.Hosts)
	diags.Append(d...)
	cfg.Hosts = hosts

	groups, d := extractGroups(ctx, data.Groups)
	diags.Append(d...)
	cfg.Groups = groups

	globalVars, d := extractStringMap(ctx, data.Variables)
	diags.Append(d...)
	cfg.GlobalVars = globalVars

	extraVars, d := extractStringMap(ctx, data.ExtraVars)
	diags.Append(d...)
	cfg.ExtraVars = extraVars

	// Pre-render inventory for the computed attribute
	if !diags.HasError() {
		rendered, err := buildInventoryRendered(cfg.Hosts, cfg.Groups, cfg.GlobalVars)
		if err != nil {
			diags.AddError("Inventory generation failed", err.Error())
		} else {
			cfg.InventoryRendered = rendered
		}
	}

	return cfg, diags
}

// diagError is a helper to return a single-item diag.Diagnostics.
func diagError(summary, detail string) diag.Diagnostics {
	return diag.Diagnostics{diag.NewErrorDiagnostic(summary, detail)}
}

// buildInventoryRendered renders the inventory for the computed attribute.
func buildInventoryRendered(hosts []deploy.InventoryHost, groups []deploy.InventoryGroup, vars map[string]string) (string, error) {
	b, err := deploy.BuildInventory(hosts, groups, vars)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
