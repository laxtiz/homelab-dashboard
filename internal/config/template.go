package config

import (
	"os"
	"path/filepath"

	tpl "dashboard/config"
)

// generateSample writes the embedded sample template to path. It never
// overwrites an existing file.
func generateSample(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, tpl.Sample, 0o644)
}
