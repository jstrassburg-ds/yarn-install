package yarninstall

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// YarnrcConfig represents the configuration from .yarnrc.yml
type YarnrcConfig struct {
	NodeLinker              string                 `yaml:"nodeLinker"`
	PnpIgnorePatterns       []string               `yaml:"pnpIgnorePatterns"`
	CacheFolder             string                 `yaml:"cacheFolder"`
	EnableImmutableInstalls *bool                  `yaml:"enableImmutableInstalls"`
	YarnPath                string                 `yaml:"yarnPath"`
	PackageExtensions       map[string]interface{} `yaml:"packageExtensions"`
}

// ParseYarnrcYml parses a .yarnrc.yml file and returns the configuration
func ParseYarnrcYml(projectPath string) (*YarnrcConfig, error) {
	yarnrcPath := filepath.Join(projectPath, YarnrcYml)

	data, err := os.ReadFile(yarnrcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var config YarnrcConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// DetermineYarnVersion determines if the project uses Yarn Classic or Berry
func DetermineYarnVersion(projectPath string) (string, error) {
	// Check for .yarnrc.yml (Berry)
	yarnrcYmlPath := filepath.Join(projectPath, YarnrcYml)
	if _, err := os.Stat(yarnrcYmlPath); err == nil {
		return YarnBerry, nil
	}

	// Check for yarn.lock (could be either, but default to Classic if no .yarnrc.yml)
	yarnLockPath := filepath.Join(projectPath, YarnLock)
	if _, err := os.Stat(yarnLockPath); err == nil {
		return YarnClassic, nil
	}

	return "", nil
}

// DetermineProvisionType determines whether to provide node_modules or yarn_pkgs
func DetermineProvisionType(projectPath string, config *YarnrcConfig) string {
	// If no .yarnrc.yml, default to node_modules (Classic behavior)
	if config == nil {
		return PlanDependencyNodeModules
	}

	// Check nodeLinker setting
	switch config.NodeLinker {
	case NodeLinkerNodeModules, NodeLinkerPnpm:
		return PlanDependencyNodeModules
	case NodeLinkerPnP, "":
		// PnP is the default for Berry when nodeLinker is not specified
		return PlanDependencyYarnPkgs
	default:
		// Unknown linker, default to node_modules for safety
		return PlanDependencyNodeModules
	}
}

// ShouldUseNodeModules determines if the project should use node_modules
func ShouldUseNodeModules(projectPath string, config *YarnrcConfig) bool {
	return DetermineProvisionType(projectPath, config) == PlanDependencyNodeModules
}

// HasPnpFiles checks if PnP files exist in the project
func HasPnpFiles(projectPath string) (bool, error) {
	pnpPath := filepath.Join(projectPath, PnpCjs)
	_, err := os.Stat(pnpPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// HasYarnCache checks if .yarn/cache directory exists
func HasYarnCache(projectPath string, config *YarnrcConfig) (bool, error) {
	cacheDir := ".yarn/cache"
	if config != nil && config.CacheFolder != "" {
		cacheDir = config.CacheFolder
	}

	cachePath := filepath.Join(projectPath, cacheDir)
	_, err := os.Stat(cachePath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
