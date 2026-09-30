package yarninstall_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/paketo-buildpacks/packit/v2/pexec"
	"github.com/paketo-buildpacks/packit/v2/scribe"
	yarninstall "github.com/paketo-buildpacks/yarn-install"
	"github.com/paketo-buildpacks/yarn-install/fakes"
	"github.com/sclevine/spec"

	. "github.com/onsi/gomega"
)

// testBerryInstallProcess covers the Berry-specific ShouldRun and Execute
// behavior. The classic YarnInstallProcess is covered by
// install_process_test.go; these tests are focused on the .yarnrc.yml /
// .pnp.cjs / .yarn/cache/ interactions that Berry adds, and on the
// devDependency pruning that replaces Yarn 1's --production flag.
func testBerryInstallProcess(t *testing.T, context spec.G, it spec.S) {
	var Expect = NewWithT(t).Expect

	context("ShouldRun for PnP projects", func() {
		var (
			workingDir     string
			installProcess yarninstall.BerryInstallProcess
			buffer         *bytes.Buffer
		)

		it.Before(func() {
			var err error
			workingDir, err = os.MkdirTemp("", "berry-should-run")
			Expect(err).NotTo(HaveOccurred())

			// PnP-complete on-disk setup: yarn.lock + .yarnrc.yml + .pnp.cjs
			// + a non-empty .yarn/cache/ directory. This mirrors the state a
			// Yarn Berry Zero-Installs project ships in git.
			Expect(os.WriteFile(filepath.Join(workingDir, "yarn.lock"), []byte("# yarn.lock"), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(workingDir, ".yarnrc.yml"), []byte("nodeLinker: pnp\n"), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(workingDir, ".pnp.cjs"), []byte("// pnp loader"), 0644)).To(Succeed())
			Expect(os.MkdirAll(filepath.Join(workingDir, ".yarn", "cache"), 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(workingDir, ".yarn", "cache", "example.zip"), []byte("zip"), 0644)).To(Succeed())

			buffer = bytes.NewBuffer(nil)
			installProcess = yarninstall.NewBerryInstallProcess(&fakes.Executable{}, &fakes.Summer{}, scribe.NewEmitter(buffer))
		})

		it.After(func() {
			Expect(os.RemoveAll(workingDir)).To(Succeed())
		})

		// Regression: on a first-time build (no prior cache_sha in layer
		// metadata), the buildpack previously returned run=false whenever the
		// on-disk PnP setup was complete. That told the caller to skip install
		// AND request a launch-layer reuse from a non-existent previous image,
		// which the CNB lifecycle then rejects with
		//   cannot reuse 'paketo-buildpacks/yarn-install:launch-modules',
		//   previous image has no metadata for layer 'paketo-buildpacks/yarn-install:launch-modules'
		// The fix is to only take the skip-install shortcut once a prior build
		// has actually written a launch layer we can reuse.
		it("runs install when no prior cache_sha is present in metadata", func() {
			run, _, err := installProcess.ShouldRun(workingDir, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(run).To(BeTrue())
			Expect(buffer.String()).To(ContainSubstring(".pnp.cjs -> true"))
			Expect(buffer.String()).To(ContainSubstring("Yarn cache -> true"))
			Expect(buffer.String()).To(ContainSubstring("Prior build metadata -> false"))
			Expect(buffer.String()).NotTo(ContainSubstring("PnP setup complete, skipping install"))
		})

		it("runs install when metadata is an empty map (still no cache_sha)", func() {
			run, _, err := installProcess.ShouldRun(workingDir, map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(run).To(BeTrue())
			Expect(buffer.String()).To(ContainSubstring("Prior build metadata -> false"))
		})

		it("runs install when metadata['cache_sha'] is present but not a string", func() {
			// Defensive against non-string types stored under the same key by
			// a different buildpack revision.
			run, _, err := installProcess.ShouldRun(workingDir, map[string]interface{}{"cache_sha": 42})
			Expect(err).NotTo(HaveOccurred())
			Expect(run).To(BeTrue())
			Expect(buffer.String()).To(ContainSubstring("Prior build metadata -> false"))
		})

		it("skips install and reuses the layer when a prior cache_sha is present", func() {
			run, sha, err := installProcess.ShouldRun(workingDir, map[string]interface{}{"cache_sha": "abc123"})
			Expect(err).NotTo(HaveOccurred())
			Expect(run).To(BeFalse())
			Expect(sha).To(BeEmpty())
			Expect(buffer.String()).To(ContainSubstring("Prior build metadata -> true"))
			Expect(buffer.String()).To(ContainSubstring("PnP setup complete, skipping install"))
		})

		it("skips install and reuses the layer even for an empty prior cache_sha", func() {
			// An empty-string cache_sha means a prior build wrote the layer
			// (the classic PnP path always stored ""); we must treat it as a
			// real prior build, not a first-time build, so downstream reuse
			// keeps working after the fix.
			run, sha, err := installProcess.ShouldRun(workingDir, map[string]interface{}{"cache_sha": ""})
			Expect(err).NotTo(HaveOccurred())
			Expect(run).To(BeFalse())
			Expect(sha).To(BeEmpty())
			Expect(buffer.String()).To(ContainSubstring("Prior build metadata -> true"))
			Expect(buffer.String()).To(ContainSubstring("PnP setup complete, skipping install"))
		})

		it("runs install when the yarn cache is absent, regardless of prior metadata", func() {
			// Cache missing -> unconditionally rebuild, prior metadata is moot.
			Expect(os.RemoveAll(filepath.Join(workingDir, ".yarn", "cache"))).To(Succeed())
			run, _, err := installProcess.ShouldRun(workingDir, map[string]interface{}{"cache_sha": "abc123"})
			Expect(err).NotTo(HaveOccurred())
			Expect(run).To(BeTrue())
			Expect(buffer.String()).To(ContainSubstring("Yarn cache -> false"))
		})

		it("runs install when .pnp.cjs is absent, regardless of prior metadata", func() {
			Expect(os.Remove(filepath.Join(workingDir, ".pnp.cjs"))).To(Succeed())
			run, _, err := installProcess.ShouldRun(workingDir, map[string]interface{}{"cache_sha": "abc123"})
			Expect(err).NotTo(HaveOccurred())
			Expect(run).To(BeTrue())
			Expect(buffer.String()).To(ContainSubstring(".pnp.cjs -> false"))
		})
	})

	context("Execute", func() {
		var (
			workingDir       string
			modulesLayerPath string
			executable       *fakes.Executable
			executions       []pexec.Execution
			installProcess   yarninstall.BerryInstallProcess
			buffer           *bytes.Buffer
		)

		// executedArgs flattens the recorded yarn invocations so a test can
		// assert on both the commands and the order they ran in.
		executedArgs := func() [][]string {
			var args [][]string
			for _, execution := range executions {
				args = append(args, execution.Args)
			}
			return args
		}

		it.Before(func() {
			var err error
			workingDir, err = os.MkdirTemp("", "berry-execute")
			Expect(err).NotTo(HaveOccurred())

			modulesLayerPath, err = os.MkdirTemp("", "berry-execute-layer")
			Expect(err).NotTo(HaveOccurred())

			Expect(os.WriteFile(filepath.Join(workingDir, "yarn.lock"), []byte("# yarn.lock"), 0644)).To(Succeed())

			executions = nil
			executable = &fakes.Executable{}
			executable.ExecuteCall.Stub = func(execution pexec.Execution) error {
				executions = append(executions, execution)
				return nil
			}

			buffer = bytes.NewBuffer(nil)
			installProcess = yarninstall.NewBerryInstallProcess(executable, &fakes.Summer{}, scribe.NewEmitter(buffer))
		})

		it.After(func() {
			Expect(os.RemoveAll(workingDir)).To(Succeed())
			Expect(os.RemoveAll(modulesLayerPath)).To(Succeed())
		})

		context("node_modules projects", func() {
			it.Before(func() {
				Expect(os.WriteFile(filepath.Join(workingDir, ".yarnrc.yml"), []byte("nodeLinker: node-modules\n"), 0644)).To(Succeed())
			})

			it("prunes devDependencies from the launch layer", func() {
				Expect(installProcess.Execute(workingDir, modulesLayerPath, true)).To(Succeed())

				Expect(executedArgs()).To(Equal([][]string{
					{"install", "--immutable"},
					{"workspaces", "focus", "--all", "--production"},
				}))
				Expect(executions[1].Dir).To(Equal(workingDir))
				Expect(buffer.String()).To(ContainSubstring("Running 'yarn workspaces focus --all --production'"))
			})

			// The build layer runs 'yarn build', which needs vite and tsc, so
			// it has to keep its devDependencies.
			it("leaves devDependencies in place for the build layer", func() {
				Expect(installProcess.Execute(workingDir, modulesLayerPath, false)).To(Succeed())

				Expect(executedArgs()).To(Equal([][]string{
					{"install", "--immutable"},
				}))
				Expect(buffer.String()).NotTo(ContainSubstring("workspaces focus"))
			})
		})

		context("PnP projects", func() {
			it.Before(func() {
				Expect(os.WriteFile(filepath.Join(workingDir, ".yarnrc.yml"), []byte("nodeLinker: pnp\n"), 0644)).To(Succeed())
			})

			it("prunes devDependencies from the launch layer using the layer cache", func() {
				Expect(installProcess.Execute(workingDir, modulesLayerPath, true)).To(Succeed())

				Expect(executedArgs()).To(Equal([][]string{
					{"install", "--immutable"},
					{"workspaces", "focus", "--all", "--production"},
				}))

				// The prune has to resolve against the same layer-local cache
				// the install just populated.
				Expect(executions[1].Env).To(ContainElement(fmt.Sprintf("YARN_CACHE_FOLDER=%s", filepath.Join(modulesLayerPath, "cache"))))
			})

			it("leaves devDependencies in place for the build layer", func() {
				Expect(installProcess.Execute(workingDir, modulesLayerPath, false)).To(Succeed())

				Expect(executedArgs()).To(Equal([][]string{
					{"install", "--immutable"},
				}))
			})
		})

		context("when BP_NODE_RUN_SCRIPTS is set", func() {
			it.Before(func() {
				Expect(os.WriteFile(filepath.Join(workingDir, ".yarnrc.yml"), []byte("nodeLinker: node-modules\n"), 0644)).To(Succeed())
				Expect(os.Setenv("BP_NODE_RUN_SCRIPTS", "build")).To(Succeed())
			})

			it.After(func() {
				Expect(os.Unsetenv("BP_NODE_RUN_SCRIPTS")).To(Succeed())
			})

			// Ordering matters: the build scripts themselves live in
			// devDependencies, so pruning ahead of them would break the build.
			it("runs the build scripts before pruning", func() {
				Expect(installProcess.Execute(workingDir, modulesLayerPath, true)).To(Succeed())

				Expect(executedArgs()).To(Equal([][]string{
					{"install", "--immutable"},
					{"run", "build"},
					{"workspaces", "focus", "--all", "--production"},
				}))
			})

			it("does not prune when a build script fails", func() {
				executable.ExecuteCall.Stub = func(execution pexec.Execution) error {
					executions = append(executions, execution)
					if execution.Args[0] == "run" {
						return errors.New("script failed")
					}
					return nil
				}

				err := installProcess.Execute(workingDir, modulesLayerPath, true)
				Expect(err).To(MatchError(ContainSubstring("failed to execute yarn run build")))
				Expect(executedArgs()).To(Equal([][]string{
					{"install", "--immutable"},
					{"run", "build"},
				}))
			})
		})

		context("when the prune fails", func() {
			it.Before(func() {
				Expect(os.WriteFile(filepath.Join(workingDir, ".yarnrc.yml"), []byte("nodeLinker: node-modules\n"), 0644)).To(Succeed())

				executable.ExecuteCall.Stub = func(execution pexec.Execution) error {
					executions = append(executions, execution)
					if execution.Args[0] == "workspaces" {
						return errors.New("Unsupported option name")
					}
					return nil
				}
			})

			// Yarn 2 and 3 only have 'workspaces focus' once the
			// workspace-tools plugin is imported. A missing command must cost
			// image size, not the whole build.
			it("warns and lets the build succeed", func() {
				Expect(installProcess.Execute(workingDir, modulesLayerPath, true)).To(Succeed())

				Expect(buffer.String()).To(ContainSubstring("Warning: failed to prune devDependencies"))
				Expect(buffer.String()).To(ContainSubstring("Unsupported option name"))
			})
		})

		context("when the install fails", func() {
			it.Before(func() {
				Expect(os.WriteFile(filepath.Join(workingDir, ".yarnrc.yml"), []byte("nodeLinker: node-modules\n"), 0644)).To(Succeed())

				executable.ExecuteCall.Stub = func(execution pexec.Execution) error {
					executions = append(executions, execution)
					return errors.New("install failed")
				}
			})

			it("returns the error without attempting a prune", func() {
				err := installProcess.Execute(workingDir, modulesLayerPath, true)
				Expect(err).To(MatchError(ContainSubstring("failed to execute yarn install")))
				Expect(executedArgs()).To(Equal([][]string{
					{"install", "--immutable"},
				}))
			})
		})
	})
}
