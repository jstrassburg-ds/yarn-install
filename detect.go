package yarninstall

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/paketo-buildpacks/libnodejs"
	"github.com/paketo-buildpacks/packit/v2"
	"github.com/paketo-buildpacks/packit/v2/fs"
)

type BuildPlanMetadata struct {
	Version       string `toml:"version"`
	VersionSource string `toml:"version-source"`
	Build         bool   `toml:"build"`
}

func Detect() packit.DetectFunc {
	return func(context packit.DetectContext) (packit.DetectResult, error) {
		projectPath, err := libnodejs.FindProjectPath(context.WorkingDir)
		if err != nil {
			return packit.DetectResult{}, err
		}

		// Parse .yarnrc.yml if it exists
		yarnrcConfig, err := ParseYarnrcYml(projectPath)
		if err != nil {
			return packit.DetectResult{}, err
		}

		// Check for .yarnrc.yml OR yarn.lock
		hasYarnrcYml := yarnrcConfig != nil

		hasYarnLock, err := fs.Exists(filepath.Join(projectPath, YarnLock))
		if err != nil {
			return packit.DetectResult{}, err
		}

		if !hasYarnrcYml && !hasYarnLock {
			return packit.DetectResult{}, packit.Fail.WithMessage("no '%s' or '%s' file found in the project path %s", YarnrcYml, YarnLock, projectPath)
		}

		// Determine what to provide based on configuration
		provisionType := DetermineProvisionType(projectPath, yarnrcConfig)

		pkg, err := libnodejs.ParsePackageJSON(projectPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return packit.DetectResult{}, packit.Fail.WithMessage("no 'package.json' found in project path %s", filepath.Join(projectPath))
			}

			return packit.DetectResult{}, err
		}
		nodeVersion := pkg.GetVersion()

		nodeRequirement := packit.BuildPlanRequirement{
			Name: PlanDependencyNode,
			Metadata: BuildPlanMetadata{
				Build: true,
			},
		}

		if nodeVersion != "" {
			nodeRequirement.Metadata = BuildPlanMetadata{
				Version:       nodeVersion,
				VersionSource: "package.json",
				Build:         true,
			}
		}

		return packit.DetectResult{
			Plan: packit.BuildPlan{
				Provides: []packit.BuildPlanProvision{
					{Name: provisionType},
				},
				Requires: []packit.BuildPlanRequirement{
					nodeRequirement,
					{
						Name: PlanDependencyYarn,
						Metadata: BuildPlanMetadata{
							Build: true,
						},
					},
				},
			},
		}, nil
	}
}
