# Role Command Behavior Changes

## Summary

The `diffusion role` command behavior has been updated to be more explicit and predictable.

## Previous Behavior

```bash
$ diffusion role
# If no role config found:
# - Would prompt: "Would you like to initialize a new role? (y/n):"
# - User had to respond interactively
# - Could accidentally initialize when just checking config
```

## New Behavior

### Without `--init` flag

```bash
$ diffusion role
# If role exists:
# - Displays current role configuration
# - Shows role name, namespace, collections, and roles

# If no role exists:
# - Returns error: "role config not found. Use 'diffusion role --init' to initialize a new role"
# - No interactive prompt
# - Clear guidance on what to do next
```

### With `--init` flag

```bash
$ diffusion role --init
# If no role exists:
# - Initializes new role with ansible-galaxy
# - Prompts for role configuration
# - Creates meta/main.yml and scenarios/default/requirements.yml

# If role already exists:
# - Returns error: "role already exists in current directory (meta/main.yml found)"
# - Prevents accidental overwrite
```

## Migration Guide

### Before (Old Behavior)

```bash
# User runs command without knowing if role exists
$ diffusion role
Role config not found. Would you like to initialize a new role? (y/n):
y
# Role gets initialized
```

### After (New Behavior)

```bash
# Check if role exists
$ diffusion role
Error: role config not found. Use 'diffusion role --init' to initialize a new role

# Explicitly initialize new role
$ diffusion role --init
Enter role name: my-role
# ... configuration prompts ...
Role initialized successfully.

# View existing role
$ diffusion role
Current Role Name: my-role
Current Namespace: my-namespace
Current Collections:
  - community.general
Current Roles:
  - example.role
```

## Benefits

1. **Explicit Intent**: Users must explicitly use `--init` to create a new role
2. **No Accidental Initialization**: Prevents accidentally creating a role when just checking config
3. **Better Error Messages**: Clear guidance on what to do when role doesn't exist
4. **Prevents Overwrites**: Warns if trying to initialize when role already exists
5. **Scriptable**: Non-interactive by default, better for automation

## Examples

### Initialize a new role

```bash
$ cd ~/projects
$ diffusion role --init
Enter role name: webserver
What namespace of the role should be?: mycompany
# ... follow prompts ...
Role initialized successfully.
```

### View existing role configuration

```bash
$ cd ~/projects/webserver
$ diffusion role
Current Role Name: webserver
Current Namespace: mycompany
Current Collections:
  - community.general
  - ansible.posix
Current Roles:
  - geerlingguy.nginx
```

### Try to initialize when role exists

```bash
$ cd ~/projects/webserver
$ diffusion role --init
Error: role already exists in current directory (meta/main.yml found)
```

### Check role in empty directory

```bash
$ cd ~/projects/empty-dir
$ diffusion role
Error: role config not found. Use 'diffusion role --init' to initialize a new role: open meta/main.yml: The system cannot find the file specified.
```

## Testing

New tests have been added to verify the behavior:

- `TestRoleCommandWithoutInit` - Verifies error when no role exists
- `TestRoleCommandWithInitFlagExistingRole` - Verifies warning when role exists
- `TestRoleCommandDisplaysConfig` - Verifies config display
- `TestCheckRoleExists` - Verifies role existence check

Run tests with:
```bash
go test -v -run TestRoleCommand
```

## Breaking Change Notice

⚠️ **This is a breaking change** if you have scripts or workflows that rely on the interactive prompt behavior.

**Action Required:**
- Update any scripts that use `diffusion role` to use `diffusion role --init` when initializing new roles
- Update documentation or training materials that reference the old behavior

## Rollback

If you need the old behavior temporarily, you can:
1. Check out the previous commit before this change
2. Build from that version
3. File an issue if you have a use case that isn't covered by the new behavior
