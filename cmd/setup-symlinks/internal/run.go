package internal

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// YarnrcConfig represents the configuration from .yarnrc.yml
type YarnrcConfig struct {
	NodeLinker string `yaml:"nodeLinker"`
}

func Run(executablePath, appDir string) error {
	// Check if this is a Yarn Berry PnP project
	isPnP, err := isYarnBerryPnP(appDir)
	if err != nil {
		return fmt.Errorf("failed to check for Yarn Berry PnP: %w", err)
	}

	// Skip symlink setup for PnP projects
	if isPnP {
		return nil
	}

	fname := strings.Split(executablePath, "/")
	layerPath := filepath.Join(fname[:len(fname)-2]...)
	if filepath.IsAbs(executablePath) {
		layerPath = fmt.Sprintf("/%s", layerPath)
	}

	linkPath, err := os.Readlink(filepath.Join(appDir, "node_modules"))
	if err != nil {
		return err
	}

	linkPath, err = filepath.Abs(linkPath)
	if err != nil {
		return err
	}

	fileInfo, err := os.Stat(linkPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	if fileInfo != nil && fileInfo.IsDir() {
		return nil
	}

	return createSymlink(filepath.Join(layerPath, "node_modules"), linkPath)
}

// isYarnBerryPnP checks if this is a Yarn Berry project using PnP
func isYarnBerryPnP(appDir string) (bool, error) {
	// Check for .yarnrc.yml file (Berry indicator)
	yarnrcPath := filepath.Join(appDir, ".yarnrc.yml")
	if _, err := os.Stat(yarnrcPath); err == nil {
		// Parse .yarnrc.yml to check nodeLinker setting
		content, err := os.ReadFile(yarnrcPath)
		if err != nil {
			return false, err
		}

		var config YarnrcConfig
		if err := yaml.Unmarshal(content, &config); err != nil {
			// If we can't parse it, assume it's Berry with default PnP
			return true, nil
		}

		// If nodeLinker is not set or is "pnp", it's PnP mode
		return config.NodeLinker == "" || config.NodeLinker == "pnp", nil
	}

	// Check for .pnp.cjs file (PnP runtime)
	pnpPath := filepath.Join(appDir, ".pnp.cjs")
	if _, err := os.Stat(pnpPath); err == nil {
		return true, nil
	}

	// No Berry PnP indicators found
	return false, nil
}

func createSymlink(target, source string) error {
	err := os.RemoveAll(source)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(source), os.ModePerm)
	if err != nil {
		return err
	}

	return os.Symlink(target, source)
}
