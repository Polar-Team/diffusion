package patch

// patch.go will host the main patching module: CreatePatchSkeleton,
// ApplyPatches, DiffPatches, CheckPatches and RevertPatches operating
// ephemerally on installed external roles. Implemented in a later step;
// the analyzer (patch_analyzer.go) and config (patch_config.go) already
// share dotted task IDs and FindLeafByID so inspection and patching
// resolve identically.
