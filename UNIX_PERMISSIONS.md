# Unix Permissions Compatibility

## Problem

Docker containers typically run as the `root` user (UID 0), which causes permission issues on Unix systems (Linux, macOS):

1. **Permission Issues**: Files created by the container are owned by root, making them inaccessible to the host user
2. **Docker-in-Docker (DinD) Requirement**: Molecule's docker driver requires root privileges to run Docker inside Docker

## Solution

Diffusion uses a hybrid approach that balances DinD requirements with Unix permissions:

### Main Molecule Container
- **Runs as ROOT** (required for Docker-in-Docker functionality)
- After operations complete, ownership is fixed inside the container using `chown -R UID:GID`
- Files are automatically chowned to the host user's UID:GID

### AnsibleGalaxyInit Container
- **Runs with user mapping** (--user UID:GID)
- Creates role structure with correct ownership from the start
- No permission fixes needed

### Windows
- Containers run as `root` user (default behavior)
- No permission fixes needed due to Windows file system permissions model

## Implementation

### Permission Fixing Inside Container

Instead of trying to chown from the host (which fails with "operation not permitted"), we run chown commands **inside the container** where we have root privileges:

```go
// Fix permissions inside container after operations
if runtime.GOOS != "windows" {
    uid := os.Getuid()
    gid := os.Getgid()
    chownCmd := fmt.Sprintf("chown -R %d:%d /opt/molecule", uid, gid)
    dockerExecInteractiveHide(RoleFlag, "/bin/sh", "-c", chownCmd)
}
```

This is called:
1. After ansible-galaxy role init (line ~1750 in main.go)
2. After molecule converge operations (line ~1480 in main.go)
3. After all molecule operations complete (line ~1795 in main.go)

### User Mapping for Galaxy Init

The `GetUserMappingArgs()` function provides user mapping for ansible-galaxy:

```go
func GetUserMappingArgs() []string {
    if runtime.GOOS == "windows" {
        return []string{}
    }
    
    uid := os.Getuid()
    gid := os.Getgid()
    return []string{"--user", fmt.Sprintf("%d:%d", uid, gid)}
}
```

## Usage

This feature is automatically applied when running `diffusion` commands. No configuration or manual intervention required.

### Example

On Linux/macOS:
```bash
diffusion molecule --role myrole --org myorg
```

The container runs as root for DinD, but after operations complete, all files in the molecule directory are automatically chowned to your user.

## Benefits

1. **Docker-in-Docker Works**: Container runs as root, enabling Molecule's docker driver
2. **No Permission Issues**: Files are automatically fixed to be owned by host user
3. **Ansible Galaxy Works**: Role initialization uses user mapping for correct ownership
4. **Cross-Platform**: Works seamlessly on Windows, Linux, and macOS
5. **Automatic**: No manual configuration needed

## Technical Details

### Why Root for Main Container?

Molecule's docker driver requires Docker-in-Docker (DinD), which needs:
- Root privileges to run Docker daemon
- Access to Docker socket with elevated permissions
- Ability to create and manage containers

Running as non-root would break Molecule's core functionality.

### Why Fix Permissions Inside Container?

Since the container runs as root, any files it creates in mounted volumes are owned by root. Trying to chown from the host fails with "operation not permitted" because the host user doesn't have permission to change ownership of root-owned files.

By running `chown -R UID:GID /opt/molecule` **inside the container** (where we have root privileges), we can successfully change ownership of all files to match the host user's UID:GID.

This happens automatically after:
- Ansible Galaxy role initialization
- Molecule converge operations
- Any operation that modifies files in the molecule directory

### Why User Mapping for Galaxy Init?

The ansible-galaxy init command only creates files and doesn't need Docker-in-Docker. Running it with user mapping ensures the role structure is created with correct ownership from the start, avoiding the need for permission fixes.

## Affected Operations

### Operations with Permission Fixes
- Molecule converge, verify, test, lint, idempotence
- Molecule destroy (during wipe)
- Any operation that modifies files in the molecule directory

### Operations with User Mapping
- Ansible Galaxy role initialization (--init flag)

## Troubleshooting

### Issue: Molecule folder owned by root

**Cause**: Permission fix didn't run or failed inside container.

**Solution**: Check logs for warnings about permission fixes. The chown command should run automatically inside the container after operations.

### Issue: "operation not permitted" when fixing permissions

**Cause**: Trying to chown from host instead of inside container.

**Solution**: The fix now runs inside the container where we have root privileges. This error should no longer occur.

### Issue: Cannot access Docker inside container

**Cause**: Container not running as root.

**Solution**: Ensure the main molecule container runs as root (no --user flag). This is required for Docker-in-Docker.

### Issue: Ansible galaxy creates root-owned files

**Cause**: User mapping not applied to galaxy init container.

**Solution**: Verify `GetUserMappingArgs()` is used when creating the ansible-galaxy container.

## Testing

The feature includes comprehensive tests in `helpers_test.go`:

```bash
# Test user mapping
go test -v -run TestGetUserMappingArgs

# Test permission fixing
go test -v -run TestFixMoleculePermissions

# Run all helper tests
go test -v ./... -run "helpers_test"
```

## Summary

Diffusion provides **Unix compatibility** with a hybrid approach:

✅ **Main container runs as root** - Docker-in-Docker works  
✅ **Automatic permission fixes** - Files owned by host user after operations  
✅ **Galaxy init uses user mapping** - Correct ownership from start  
✅ **Cross-platform** - Windows, Linux, macOS  
✅ **Zero configuration** - Works out of the box  
✅ **Well tested** - Comprehensive test coverage
