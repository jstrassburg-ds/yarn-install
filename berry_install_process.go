package yarninstall

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/paketo-buildpacks/packit/v2/fs"
	"github.com/paketo-buildpacks/packit/v2/pexec"
	"github.com/paketo-buildpacks/packit/v2/scribe"
)

type BerryInstallProcess struct {
	executable Executable
	summer     Summer
	logger     scribe.Emitter
}

func NewBerryInstallProcess(executable Executable, summer Summer, logger scribe.Emitter) BerryInstallProcess {
	return BerryInstallProcess{
		executable: executable,
		summer:     summer,
		logger:     logger,
	}
}

// ShouldRun determines if yarn install should be executed for Berry projects
func (ip BerryInstallProcess) ShouldRun(workingDir string, metadata map[string]interface{}) (run bool, sha string, err error) {
	ip.logger.Subprocess("Process inputs (Berry):")

	// Check for yarn.lock
	yarnLockPath := filepath.Join(workingDir, YarnLock)
	_, err = os.Stat(yarnLockPath)
	if os.IsNotExist(err) {
		ip.logger.Action("yarn.lock -> Not found")
		ip.logger.Break()
		return true, "", nil
	} else if err != nil {
		return true, "", fmt.Errorf("unable to read yarn.lock file: %w", err)
	}

	ip.logger.Action("yarn.lock -> Found")

	// Parse .yarnrc.yml to understand the project configuration
	yarnrcConfig, err := ParseYarnrcYml(workingDir)
	if err != nil {
		return true, "", fmt.Errorf("failed to parse .yarnrc.yml: %w", err)
	}

	// Check if using node_modules
	usesNodeModules := ShouldUseNodeModules(workingDir, yarnrcConfig)
	ip.logger.Action("Uses node_modules -> %t", usesNodeModules)

	// If using node_modules, use similar logic to Classic
	if usesNodeModules {
		return ip.shouldRunForNodeModules(workingDir, metadata)
	}

	// For PnP projects, check different conditions
	return ip.shouldRunForPnP(workingDir, yarnrcConfig, metadata)
}

func (ip BerryInstallProcess) shouldRunForNodeModules(workingDir string, metadata map[string]interface{}) (bool, string, error) {
	// For node_modules, check if yarn.lock has changed (similar to Classic)
	buffer := bytes.NewBuffer(nil)

	err := ip.executable.Execute(pexec.Execution{
		Args:   []string{"info", "--all", "--json"},
		Stdout: buffer,
		Stderr: buffer,
		Dir:    workingDir,
	})
	if err != nil {
		ip.logger.Action("Failed to get yarn info, falling back to install")
		return true, "", nil
	}

	nodeEnv := os.Getenv("NODE_ENV")
	buffer.WriteString(nodeEnv)

	file, err := os.CreateTemp("", "berry-config-file")
	if err != nil {
		return true, "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer file.Close()

	_, err = file.Write(buffer.Bytes())
	if err != nil {
		return true, "", fmt.Errorf("failed to write temp file: %w", err)
	}

	sum, err := ip.summer.Sum(filepath.Join(workingDir, YarnLock), filepath.Join(workingDir, "package.json"), file.Name())
	if err != nil {
		return true, "", fmt.Errorf("unable to sum config files: %w", err)
	}

	prevSHA, ok := metadata["cache_sha"].(string)
	if (ok && sum != prevSHA) || !ok {
		return true, sum, nil
	}

	return false, "", nil
}

func (ip BerryInstallProcess) shouldRunForPnP(workingDir string, config *YarnrcConfig, metadata map[string]interface{}) (bool, string, error) {
	// Check for .yarnrc.yml
	hasYarnrcYml := config != nil
	ip.logger.Action(".yarnrc.yml -> %t", hasYarnrcYml)

	// Check for .pnp.cjs
	hasPnpFiles, err := HasPnpFiles(workingDir)
	if err != nil {
		return true, "", fmt.Errorf("failed to check for PnP files: %w", err)
	}
	ip.logger.Action(".pnp.cjs -> %t", hasPnpFiles)

	// Check for local cache
	hasCache, err := HasYarnCache(workingDir, config)
	if err != nil {
		return true, "", fmt.Errorf("failed to check for yarn cache: %w", err)
	}
	ip.logger.Action("Yarn cache -> %t", hasCache)

	ip.logger.Break()

	// According to RFC, should NOT run install when:
	// .yarnrc.yml, .pnp.cjs file and a local cache are present
	if hasYarnrcYml && hasPnpFiles && hasCache {
		ip.logger.Action("PnP setup complete, skipping install")
		return false, "", nil
	}

	// Should run install in other cases:
	// - No local cache
	// - No .pnp.cjs file
	return true, "", nil
}

func (ip BerryInstallProcess) SetupModules(workingDir, currentModulesLayerPath, nextModulesLayerPath string) (string, error) {
	// Parse configuration to determine approach
	yarnrcConfig, err := ParseYarnrcYml(workingDir)
	if err != nil {
		return "", fmt.Errorf("failed to parse .yarnrc.yml: %w", err)
	}

	usesNodeModules := ShouldUseNodeModules(workingDir, yarnrcConfig)

	if usesNodeModules {
		// Use Classic node_modules setup logic
		return ip.setupNodeModules(workingDir, currentModulesLayerPath, nextModulesLayerPath)
	}

	// For PnP, we don't need to setup node_modules in the same way
	// Just return the layer path for caching yarn cache
	return nextModulesLayerPath, nil
}

func (ip BerryInstallProcess) setupNodeModules(workingDir, currentModulesLayerPath, nextModulesLayerPath string) (string, error) {
	// This mirrors the Classic logic
	if currentModulesLayerPath != "" {
		err := fs.Copy(filepath.Join(currentModulesLayerPath, "node_modules"), filepath.Join(nextModulesLayerPath, "node_modules"))
		if err != nil {
			return "", fmt.Errorf("failed to copy node_modules directory: %w", err)
		}
	} else {
		file, err := os.Lstat(filepath.Join(workingDir, "node_modules"))
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return "", fmt.Errorf("failed to stat node_modules directory: %w", err)
			}
		}

		if file != nil && file.Mode()&os.ModeSymlink == os.ModeSymlink {
			err = os.RemoveAll(filepath.Join(workingDir, "node_modules"))
			if err != nil {
				return "", fmt.Errorf("failed to remove node_modules symlink: %w", err)
			}
		}

		err = os.MkdirAll(filepath.Join(workingDir, "node_modules"), os.ModePerm)
		if err != nil {
			return "", fmt.Errorf("failed to create node_modules directory: %w", err)
		}

		err = fs.Move(filepath.Join(workingDir, "node_modules"), filepath.Join(nextModulesLayerPath, "node_modules"))
		if err != nil {
			return "", fmt.Errorf("failed to move node_modules directory to layer: %w", err)
		}

		err = os.Symlink(filepath.Join(nextModulesLayerPath, "node_modules"), filepath.Join(workingDir, "node_modules"))
		if err != nil {
			return "", fmt.Errorf("failed to symlink node_modules into working directory: %w", err)
		}
	}

	return nextModulesLayerPath, nil
}

func (ip BerryInstallProcess) Execute(workingDir, modulesLayerPath string, launch bool) error {
	// Parse configuration to determine installation strategy
	yarnrcConfig, err := ParseYarnrcYml(workingDir)
	if err != nil {
		return fmt.Errorf("failed to parse .yarnrc.yml: %w", err)
	}

	usesNodeModules := ShouldUseNodeModules(workingDir, yarnrcConfig)

	if usesNodeModules {
		return ip.executeNodeModulesInstall(workingDir, modulesLayerPath, launch, yarnrcConfig)
	}

	return ip.executePnPInstall(workingDir, modulesLayerPath, launch, yarnrcConfig)
}

func (ip BerryInstallProcess) executeNodeModulesInstall(workingDir, modulesLayerPath string, launch bool, config *YarnrcConfig) error {
	environment := os.Environ()
	environment = append(environment, fmt.Sprintf("PATH=%s%c%s", os.Getenv("PATH"), os.PathListSeparator, filepath.Join("node_modules", ".bin")))

	// Use --immutable instead of --frozen-lockfile for Berry
	installArgs := []string{"install", "--ignore-engines"}

	// Check if immutable installs are disabled
	if config == nil || config.EnableImmutableInstalls == nil || *config.EnableImmutableInstalls {
		installArgs = append(installArgs, "--immutable")
	}

	if !launch {
		installArgs = append(installArgs, "--production", "false")
	}

	// For Berry node_modules, set modules folder
	installArgs = append(installArgs, "--modules-folder", filepath.Join(modulesLayerPath, "node_modules"))

	ip.logger.Subprocess("Running 'yarn %s'", strings.Join(installArgs, " "))

	err := ip.executable.Execute(pexec.Execution{
		Args:   installArgs,
		Env:    environment,
		Stdout: ip.logger.ActionWriter,
		Stderr: ip.logger.ActionWriter,
		Dir:    workingDir,
	})
	if err != nil {
		return fmt.Errorf("failed to execute yarn install: %w", err)
	}

	return nil
}

func (ip BerryInstallProcess) executePnPInstall(workingDir, modulesLayerPath string, launch bool, config *YarnrcConfig) error {
	environment := os.Environ()

	// Set up cache folder to point to layer
	cacheDir := filepath.Join(modulesLayerPath, "cache")
	environment = append(environment, fmt.Sprintf("YARN_CACHE_FOLDER=%s", cacheDir))

	// Ensure cache directory exists
	err := os.MkdirAll(cacheDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	installArgs := []string{"install", "--ignore-engines"}

	// Check if immutable installs are disabled
	if config == nil || config.EnableImmutableInstalls == nil || *config.EnableImmutableInstalls {
		installArgs = append(installArgs, "--immutable")
	}

	if !launch {
		// For Berry, we might not need --production for PnP since dependencies are resolved differently
		// But keeping for compatibility
		installArgs = append(installArgs, "--production", "false")
	}

	ip.logger.Subprocess("Running 'yarn %s' (PnP)", strings.Join(installArgs, " "))

	err = ip.executable.Execute(pexec.Execution{
		Args:   installArgs,
		Env:    environment,
		Stdout: ip.logger.ActionWriter,
		Stderr: ip.logger.ActionWriter,
		Dir:    workingDir,
	})
	if err != nil {
		return fmt.Errorf("failed to execute yarn install (PnP): %w", err)
	}

	return nil
}
