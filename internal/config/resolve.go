package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

const configFilename = "dashboard.yaml"

// Resolve determines the config file path by checking, in order:
//  1. explicit (from the -config flag) — must exist if provided
//  2. ./dashboard.yaml — current working directory
//  3. user config dir: os.UserConfigDir()/dashboard/dashboard.yaml
//     ($XDG_CONFIG_HOME 未设置时回退 ~/.config)
//
// If none exist, a sample config is generated in the current working directory.
func Resolve(explicit string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", fmt.Errorf("config file not found: %s", explicit)
		}
		log.Printf("config: using explicit path %s", explicit)
		return explicit, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	candidates := []struct {
		path string
		desc string
	}{
		{filepath.Join(cwd, configFilename), "current directory"},
		{filepath.Join(userConfigDir(), "dashboard", configFilename), "user config dir"},
	}
	for _, c := range candidates {
		if _, err := os.Stat(c.path); err == nil {
			log.Printf("config: using %s (%s)", c.path, c.desc)
			return c.path, nil
		}
	}

	path := filepath.Join(cwd, configFilename)
	if err := generateSample(path); err != nil {
		return "", fmt.Errorf("generate sample config: %w", err)
	}
	log.Printf("config: no config found, generated sample at %s", path)
	return path, nil
}

// userConfigDir returns the per-user config directory, with a sane fallback
// when os.UserConfigDir fails (e.g. no $HOME set).
func userConfigDir() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}
