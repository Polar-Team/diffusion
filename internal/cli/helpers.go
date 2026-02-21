package cli

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"strings"
)

// PromptInput prompts the user for input and returns the trimmed response
func PromptInput(prompt string) string {
	fmt.Print(prompt)
	r := bufio.NewReader(os.Stdin)
	val, _ := r.ReadString('\n')
	return strings.TrimSpace(val)
}

// maskToken masks a token for display, showing only first and last few characters
func maskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

// exists checks if a path exists
func exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// copyIfExists copies file/directory if it exists (recursively when directory)
// Performance optimization: cache os.Stat result to avoid duplicate calls
func copyIfExists(src, dst string) {
	fi, err := os.Stat(src)
	if os.IsNotExist(err) {
		log.Printf("\033[38;2;127;255;212mnote: %s does not exist, skipping\033[0m", src)
		return
	}
	if err != nil {
		log.Printf("copy stat error: %v", err)
		return
	}
	if fi.IsDir() {
		if err := copyDir(src, dst); err != nil {
			log.Printf("copy dir error %s -> %s: %v", src, dst, err)
		}
	} else {
		// file
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			log.Printf("mkdir for file: %v", err)
		}
		if err := copyFile(src, dst); err != nil {
			log.Printf("copy file error %v", err)
		}
	}
}

// copyFile copies a single file with buffered I/O
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := in.Close(); cerr != nil {
			log.Printf("Failed to close source file: %v", cerr)
		}
	}()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); cerr != nil {
			log.Printf("Failed to close destination file: %v", cerr)
		}
	}()

	// Use buffered I/O for better performance
	bufIn := bufio.NewReaderSize(in, 32*1024) // 32KB buffer
	bufOut := bufio.NewWriterSize(out, 32*1024)

	if _, err := io.Copy(bufOut, bufIn); err != nil {
		return err
	}

	if err := bufOut.Flush(); err != nil {
		return err
	}

	return out.Sync()
}

// copyDir recursively copies a directory
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		// file
		return copyFile(path, target)
	})
}

// copyRoleData copies tasks, handlers, templates, files, vars, defaults, meta, scenarios, .ansible-lint, .yamllint
func copyRoleData(basePath, roleMoleculePath string, ciMode bool) error {
	// Validate that scenarios/default directory exists
	scenariosPath := filepath.Join(basePath, "scenarios", "default")
	if _, err := os.Stat(scenariosPath); os.IsNotExist(err) {
		return fmt.Errorf("\033[31mscenarios/default directory not found in %s\n\nTo fix this:\n1. Initialize a new role: diffusion role --init\n2. Or create the directory structure manually:\n   mkdir -p scenarios/default\n   # Add molecule.yml, converge.yml, verify.yml to scenarios/default/\033[0m", basePath)
	}

	// Validate that molecule.yml exists
	moleculeYml := filepath.Join(scenariosPath, "molecule.yml")
	if _, err := os.Stat(moleculeYml); os.IsNotExist(err) {
		return fmt.Errorf("\033[31mscenarios/default/molecule.yml not found in %s\n\nThis file is required for Molecule testing.\nTo fix this:\n1. Initialize a new role: diffusion role --init\n2. Or create molecule.yml manually in scenarios/default/\033[0m", basePath)
	}

	if !ciMode {
		log.Printf("\033[38;2;127;255;212mCopying role data from %s to %s\033[0m", basePath, roleMoleculePath)
	}

	// create role dir base
	if err := os.MkdirAll(roleMoleculePath, 0o755); err != nil {
		return err
	}
	// helper copy pairs
	pairs := []struct{ src, dst string }{
		{"tasks", "tasks"},
		{"handlers", "handlers"},
		{"templates", "templates"},
		{"files", "files"},
		{"vars", "vars"},
		{"defaults", "defaults"},
		{"meta", "meta"},
		{"scenarios", "molecule"}, // copy scenarios into molecule/<role>/molecule/
	}
	for _, p := range pairs {
		src := filepath.Join(basePath, p.src)
		dst := filepath.Join(roleMoleculePath, p.dst)
		if p.src == "scenarios" {
			dst = filepath.Join(roleMoleculePath, "molecule")
		}
		if ciMode {
			log.Printf("Copying %s -> %s", src, dst)
		}
		copyIfExists(src, dst)
	}

	// Verify that molecule.yml was copied successfully
	copiedMoleculeYml := filepath.Join(roleMoleculePath, "molecule", "default", "molecule.yml")
	if ciMode {
		log.Printf("Checking if molecule.yml exists at: %s", copiedMoleculeYml)
	}
	if _, err := os.Stat(copiedMoleculeYml); os.IsNotExist(err) {
		// List what's actually in the molecule directory for debugging
		moleculeDir := filepath.Join(roleMoleculePath, "molecule")
		if entries, err := os.ReadDir(moleculeDir); err == nil {
			log.Printf("\033[33mContents of %s:\033[0m", moleculeDir)
			for _, entry := range entries {
				log.Printf("  - %s (isDir: %v)", entry.Name(), entry.IsDir())
			}
		}
		return fmt.Errorf("\033[31mFailed to copy molecule.yml to container.\nSource: %s\nDestination: %s\n\nThis may be a permission or file system issue in CI/CD.\nTry running with --ci flag: diffusion molecule --ci --converge\033[0m", moleculeYml, copiedMoleculeYml)
	}

	return nil
}
