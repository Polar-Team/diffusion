---
description: Review all source changes and update the MCP server data, HTML documentation, troubleshooting cases, and helper utilities to stay in sync with the codebase.
agent: team-lead
---
Review all recent source code changes across the workspace (diffusion, diffusion-molecule-container, diffusion-ansible-tests-role, terraform-provider-diffusion). Then:

1. **MCP Server Data** — Update the MCP server tool definitions, descriptions, and input schemas to reflect any new or changed CLI commands, flags, configurations, or behaviors. It's already existing MCP server located under mcp/diffusion_mcp.

3. **Troubleshooting Cases** — Add new troubleshooting entries for any new error conditions, failure modes, or common issues introduced by the changes. It's should be under mcp/diffusion_mcp server onlyu.

4. **Helpers** — Update or add helper functions/utilities that support the new or changed functionality. It's should be under mcp/diffusion_mcp server onlyu.

All updates should be located under main workree inside mcp/diffusion_mcp.
