# Ansible Cache Feature

## Overview

The cache feature allows you to persist the `.ansible` directory from the Molecule container to your local filesystem. This significantly speeds up subsequent role executions by caching downloaded collections, roles, and other Ansible artifacts.

## Benefits

1. **Faster Execution**: Avoid re-downloading collections and roles on every run
2. **Bandwidth Savings**: Reduce network usage by caching artifacts locally
3. **Offline Support**: Work with cached artifacts when offline
4. **Per-Role Caching**: Each role gets its own isolated cache directory
5. **Easy Management**: Simple CLI commands to enable, disable, and clean cache

## How It Works

When cache is enabled:
1. A unique cache ID is generated for your role
2. A cache directory is created at `~/.diffusion/cache/role_<cache_id>/`
3. Two subdirectories are created: `roles/` and `collections/`
4. These directories are mounted to `/root/.ansible/roles` and `/root/.ansible/collections` in the Molecule container
5. Ansible stores downloaded collections and roles in these directories
6. On subsequent runs, these artifacts are reused instead of re-downloaded

## CLI Commands

### Enable Cache

Enable caching for the current role:

```bash
$ diffusion cache enable
Cache enabled for this role
Cache ID: a1b2c3d4e5f6g7h8
Cache Directory: /home/user/.diffusion/cache/role_a1b2c3d4e5f6g7h8
```

This command:
- Generates a unique cache ID
- Creates the cache directory
- Updates `diffusion.toml` with cache settings

### Disable Cache

Disable caching (preserves cache directory):

```bash
$ diffusion cache disable
Cache disabled for this role
Note: Cache directory is preserved. Use 'diffusion cache clean' to remove it.
```

### Check Cache Status

View current cache configuration:

```bash
$ diffusion cache status
[Cache Status]
  Enabled:     true
  Cache ID:    a1b2c3d4e5f6g7h8
  Cache Path:  /home/user/.diffusion/cache/role_a1b2c3d4e5f6g7h8
  Cache Size:  45.23 MB
```

### Clean Cache

Remove cached artifacts for the current role:

```bash
$ diffusion cache clean
Cache cleaned successfully
Freed: 45.23 MB
```

### List All Caches

View all cache directories across all roles:

```bash
$ diffusion cache list
[Cache Directories]
  ✓ role_a1b2c3d4e5f6g7h8 - 45.23 MB
  ✓ role_9f8e7d6c5b4a3210 - 12.50 MB
  ✓ role_1234567890abcdef - 0.00 MB (empty)
```

## Configuration

Cache settings are stored in `diffusion.toml`:

```toml
[cache]
  enabled = true
  cache_id = "a1b2c3d4e5f6g7h8"
```

### Configuration Fields

- **enabled** (bool): Whether caching is enabled for this role
- **cache_id** (string): Unique identifier for this role's cache
- **cache_path** (string, optional): Custom cache path (defaults to `~/.diffusion/cache/role_<cache_id>`)

## Usage Example

### First Run (Without Cache)

```bash
$ cd my-ansible-role
$ diffusion molecule
# Downloads all collections and roles (slow)
# Execution time: 5 minutes
```

### Enable Cache

```bash
$ diffusion cache enable
Cache enabled for this role
Cache ID: a1b2c3d4e5f6g7h8
```

### Subsequent Runs (With Cache)

```bash
$ diffusion molecule
# Reuses cached collections and roles (fast)
# Execution time: 30 seconds
```

## Directory Structure

```
~/.diffusion/
└── cache/
    ├── role_a1b2c3d4e5f6g7h8/    # Role 1 cache
    │   ├── collections/           # Ansible collections cache
    │   └── roles/                 # Ansible roles cache
    ├── role_9f8e7d6c5b4a3210/    # Role 2 cache
    │   ├── collections/
    │   └── roles/
    └── role_1234567890abcdef/    # Role 3 cache
        ├── collections/
        └── roles/
```

**Note**: Only `roles/` and `collections/` subdirectories are mounted to avoid conflicts with other Ansible temporary files.

## Docker Integration

When cache is enabled, the Molecule container is started with two additional volume mounts:

```bash
docker run ... \
  -v /path/to/role/molecule:/opt/molecule \
  -v ~/.diffusion/cache/role_<cache_id>/roles:/root/.ansible/roles \
  -v ~/.diffusion/cache/role_<cache_id>/collections:/root/.ansible/collections \
  ...
```

This mounts only the `roles/` and `collections/` subdirectories to their respective locations in the container, avoiding potential conflicts with other Ansible files.

## Best Practices

### 1. Enable Cache for Development

Enable cache when actively developing a role to speed up iterations:

```bash
$ diffusion cache enable
```

### 2. Clean Cache Periodically

Clean cache to free disk space and ensure fresh downloads:

```bash
$ diffusion cache clean
```

### 3. Disable Cache for CI/CD

Disable cache in CI/CD pipelines to ensure clean builds:

```bash
$ diffusion cache disable
```

### 4. Monitor Cache Size

Check cache size regularly:

```bash
$ diffusion cache status
$ diffusion cache list
```

### 5. Share Cache ID in Team

Share the cache ID in your team's documentation for consistent caching:

```toml
[cache]
  enabled = true
  cache_id = "team-shared-cache-id"
```

## Troubleshooting

### Cache Not Working

If cache doesn't seem to be working:

1. Check cache status:
   ```bash
   $ diffusion cache status
   ```

2. Verify cache is enabled in `diffusion.toml`:
   ```toml
   [cache]
     enabled = true
   ```

3. Check cache directory exists:
   ```bash
   $ ls -la ~/.diffusion/cache/
   ```

### Cache Too Large

If cache grows too large:

1. Clean current role's cache:
   ```bash
   $ diffusion cache clean
   ```

2. List all caches and clean unused ones:
   ```bash
   $ diffusion cache list
   # Manually remove unused cache directories
   $ rm -rf ~/.diffusion/cache/role_<old_cache_id>
   ```

### Permission Issues

If you encounter permission issues:

```bash
$ chmod -R 755 ~/.diffusion/cache/
```

## Performance Impact

### Without Cache

- First run: Downloads ~50MB of collections/roles
- Time: 3-5 minutes
- Network: 50MB download

### With Cache

- First run: Same as without cache (builds cache)
- Subsequent runs: Reuses cached artifacts
- Time: 30 seconds - 1 minute
- Network: Minimal (only updates)

**Speed Improvement**: 3-10x faster on subsequent runs

## Security Considerations

1. **Local Storage**: Cache is stored locally in your home directory
2. **No Sensitive Data**: Cache contains only public Ansible artifacts
3. **Permissions**: Cache directories have 755 permissions
4. **Isolation**: Each role has its own isolated cache

## Migration

### From No Cache to Cache

Simply enable cache - no migration needed:

```bash
$ diffusion cache enable
```

### From Cache to No Cache

Disable cache and optionally clean:

```bash
$ diffusion cache disable
$ diffusion cache clean  # Optional: remove cached data
```

## Technical Details

### Cache ID Generation

- Uses cryptographically secure random number generator
- Generates 8 bytes (16 hex characters)
- Format: `a1b2c3d4e5f6g7h8`

### Cache Directory Structure

- Base: `~/.diffusion/cache/`
- Per-role: `role_<cache_id>/`
- Subdirectories: `roles/` and `collections/`
- Mounted to: `/root/.ansible/roles` and `/root/.ansible/collections` in container

### Automatic Cleanup

Cache is NOT automatically cleaned. Use `diffusion cache clean` to manually clean when needed.

## Examples

### Enable Cache and Run Molecule

```bash
$ cd my-ansible-role
$ diffusion cache enable
$ diffusion molecule
# First run builds cache
$ diffusion molecule
# Second run uses cache (much faster!)
```

### Check Cache Size Before and After

```bash
$ diffusion cache status
Cache Size: 0 MB (empty)

$ diffusion molecule
# ... runs molecule ...

$ diffusion cache status
Cache Size: 45.23 MB
```

### Clean Cache to Free Space

```bash
$ diffusion cache list
[Cache Directories]
  ✓ role_a1b2c3d4e5f6g7h8 - 45.23 MB
  ✓ role_9f8e7d6c5b4a3210 - 120.50 MB  # Large cache

$ cd role-with-large-cache
$ diffusion cache clean
Cache cleaned successfully
Freed: 120.50 MB
```

## FAQ

**Q: Does cache work across different roles?**
A: No, each role has its own isolated cache directory.

**Q: Can I share cache between team members?**
A: Yes, by sharing the same `cache_id` in `diffusion.toml`.

**Q: What happens if I delete the cache directory manually?**
A: Ansible will simply re-download artifacts on the next run.

**Q: Does cache affect molecule test results?**
A: No, cache only affects download speed, not test behavior.

**Q: Can I customize the cache location?**
A: Yes, set `cache_path` in `diffusion.toml` (advanced usage).

## See Also

- [ARTIFACT_MANAGEMENT.md](ARTIFACT_MANAGEMENT.md) - Managing artifact sources
- [README.md](README.md) - Main documentation
- [TESTING_AND_PERFORMANCE.md](TESTING_AND_PERFORMANCE.md) - Performance optimization
