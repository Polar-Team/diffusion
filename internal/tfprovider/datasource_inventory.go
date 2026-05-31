package tfprovider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"diffusion/internal/deploy"
)

// Ensure InventoryDataSource satisfies the datasource.DataSource interface.
var _ datasource.DataSource = &InventoryDataSource{}

// InventoryDataSource implements the diffusion_inventory data source.
// It renders an Ansible YAML inventory from hosts/groups/variables without
// triggering any deployment. Useful for debugging or piping to other tools.
type InventoryDataSource struct {
	providerCfg *ProviderConfig
}

// NewInventoryDataSource is the factory function registered with the provider.
func NewInventoryDataSource() datasource.DataSource {
	return &InventoryDataSource{}
}

// InventoryDataSourceModel maps the data source HCL schema to Go values.
type InventoryDataSourceModel struct {
	// Inputs
	Hosts     types.Map `tfsdk:"hosts"`
	Groups    types.Map `tfsdk:"groups"`
	Variables types.Map `tfsdk:"variables"`

	// Computed output
	Rendered types.String `tfsdk:"rendered"`
}

func (d *InventoryDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_inventory"
}

func (d *InventoryDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: `Renders an Ansible YAML inventory from the provided hosts, groups, and
variables without triggering any deployment. Useful for inspection, debugging,
or passing the rendered inventory to other Terraform resources.`,

		Attributes: map[string]schema.Attribute{
			"hosts": schema.MapNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Map of hostname → Ansible connection variables.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"vars": schema.MapAttribute{
							Optional:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "Ansible host variables (ansible_host, ansible_user, etc.).",
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
			"rendered": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The rendered Ansible YAML inventory.",
			},
		},
	}
}

func (d *InventoryDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	cfg, ok := req.ProviderData.(*ProviderConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *ProviderConfig, got %T", req.ProviderData),
		)
		return
	}
	d.providerCfg = cfg
}

func (d *InventoryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data InventoryDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hosts, diags := extractHosts(ctx, data.Hosts)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	groups, diags := extractGroups(ctx, data.Groups)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	globalVars, diags := extractStringMap(ctx, data.Variables)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	rendered, err := deploy.BuildInventory(hosts, groups, globalVars)
	if err != nil {
		resp.Diagnostics.AddError("Inventory generation failed", err.Error())
		return
	}

	data.Rendered = types.StringValue(string(rendered))
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// --- shared extraction helpers used by both data source and resource ---

// hostVarsModel mirrors the nested object under each host entry.
type hostVarsModel struct {
	Vars types.Map `tfsdk:"vars"`
}

func extractHosts(ctx context.Context, hostsMap types.Map) ([]deploy.InventoryHost, diag.Diagnostics) {
	if hostsMap.IsNull() || hostsMap.IsUnknown() {
		return nil, nil
	}

	elements := hostsMap.Elements()
	hosts := make([]deploy.InventoryHost, 0, len(elements))

	for name, v := range elements {
		obj, ok := v.(types.Object)
		if !ok {
			return nil, diagError("Invalid host entry", fmt.Sprintf("host %q is not an object", name))
		}

		var hm hostVarsModel
		d := obj.As(ctx, &hm, basetypes.ObjectAsOptions{})
		if d.HasError() {
			return nil, d
		}

		vars, d := extractStringMap(ctx, hm.Vars)
		if d.HasError() {
			return nil, d
		}

		hosts = append(hosts, deploy.InventoryHost{Name: name, Variables: vars})
	}

	return hosts, nil
}

func extractGroups(ctx context.Context, groupsMap types.Map) ([]deploy.InventoryGroup, diag.Diagnostics) {
	if groupsMap.IsNull() || groupsMap.IsUnknown() {
		return nil, nil
	}

	elements := groupsMap.Elements()
	groups := make([]deploy.InventoryGroup, 0, len(elements))

	for name, v := range elements {
		listVal, ok := v.(types.List)
		if !ok {
			return nil, diagError("Invalid group entry", fmt.Sprintf("group %q value is not a list", name))
		}
		var hostNames []string
		d := listVal.ElementsAs(ctx, &hostNames, false)
		if d.HasError() {
			return nil, d
		}
		groups = append(groups, deploy.InventoryGroup{Name: name, Hosts: hostNames})
	}

	return groups, nil
}

func extractStringMap(ctx context.Context, m types.Map) (map[string]string, diag.Diagnostics) {
	if m.IsNull() || m.IsUnknown() {
		return nil, nil
	}
	result := make(map[string]string)
	d := m.ElementsAs(ctx, &result, false)
	return result, d
}
