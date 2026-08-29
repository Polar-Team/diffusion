---
description: Scan all Diffusion ecosystem repositories and their dev-new-features branches for new or changed features, commands, configurations, or behaviors that should be reflected in the docs HTML website at diffusion/docs/index.html and diffusion/docs/changelog.md.
agent: team-lead
---
Scan all Diffusion ecosystem repositories for documentation updates needed on the docs HTML website.

Search these locations for new or changed information:
1.  diffusion\ — CLI source (Go files in internal/, cmd/), Makefile, README.md, dev-new-features/
2.  diffusion-molecule-container\ — Dockerfile, pyproject.toml, docs/, dev-new-features/
3.  diffusion-ansible-tests-role\ — tasks/, defaults/, README.md, CHANGELOG.md, dev-new-features/
4.  terraform-provider-diffusion\ — provider source, dev-new-features/

Look for:
- New CLI commands or flags
- Changed default values or configuration options
- New features or capabilities added
- Updated dependencies or version requirements
- New environment variables or container changes
- Terraform provider resource/data source changes
- Ansible role variable additions or behavior changes
- Workflow or CI/CD changes that affect usage

Then update the docs website at docs\index.html and docs\changelog.md to reflect any findings. Maintain the existing HTML structure, styling, and section organization. Add new sections if needed for entirely new features.
