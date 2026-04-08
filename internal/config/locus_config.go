package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/oculus/port"
	"gopkg.in/yaml.v3"
)

// ErrUnsafeConfigValue signals that a config string contains shell metacharacters.
var ErrUnsafeConfigValue = errors.New("unsafe config value")

// configFileName is the expected config file name in the repo root.
const configFileName = ".locus.yaml"

// shellMetaChars are characters that must not appear in config string values.
// This prevents command injection via layer names, boundary patterns, etc.
var shellMetaChars = []string{"|", ";", "&&", "$", "`"}

// LoadLocusConfig reads a .locus.yaml from the repo root and returns the
// parsed DesiredState. If the file does not exist, it returns (nil, nil) —
// the config is optional. Returned values are validated for shell safety.
func LoadLocusConfig(repoPath string) (*port.DesiredState, error) {
	configPath := filepath.Join(repoPath, configFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", configFileName, err)
	}

	var ds port.DesiredState
	if err := yaml.Unmarshal(data, &ds); err != nil {
		return nil, fmt.Errorf("parse %s: %w", configFileName, err)
	}

	if err := validateDesiredState(&ds); err != nil {
		return nil, fmt.Errorf("validate %s: %w", configFileName, err)
	}

	return &ds, nil
}

// validateDesiredState checks all user-supplied strings for shell metacharacters.
func validateDesiredState(ds *port.DesiredState) error {
	for i, layer := range ds.Layers {
		if err := checkSafe(layer); err != nil {
			return fmt.Errorf("layers[%d] %q: %w", i, layer, err)
		}
	}
	for i, b := range ds.Boundaries {
		if err := checkSafe(b.FromPattern); err != nil {
			return fmt.Errorf("boundaries[%d].from_pattern %q: %w", i, b.FromPattern, err)
		}
		if err := checkSafe(b.ToPattern); err != nil {
			return fmt.Errorf("boundaries[%d].to_pattern %q: %w", i, b.ToPattern, err)
		}
	}
	for i, c := range ds.Constraints {
		if err := checkSafe(c.Component); err != nil {
			return fmt.Errorf("constraints[%d].component %q: %w", i, c.Component, err)
		}
	}
	for name := range ds.Roles {
		if err := checkSafe(name); err != nil {
			return fmt.Errorf("roles key %q: %w", name, err)
		}
	}
	for i, a := range ds.Accepted {
		if err := checkSafe(a.Component); err != nil {
			return fmt.Errorf("accepted[%d].component %q: %w", i, a.Component, err)
		}
		if err := checkSafe(a.Principle); err != nil {
			return fmt.Errorf("accepted[%d].principle %q: %w", i, a.Principle, err)
		}
	}
	return nil
}

// checkSafe rejects strings containing shell metacharacters.
func checkSafe(s string) error {
	for _, meta := range shellMetaChars {
		if strings.Contains(s, meta) {
			return fmt.Errorf("%w: contains %q", ErrUnsafeConfigValue, meta)
		}
	}
	return nil
}
