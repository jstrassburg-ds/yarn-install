package yarninstall_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/paketo-buildpacks/packit/v2/scribe"
	yarninstall "github.com/paketo-buildpacks/yarn-install"
	"github.com/paketo-buildpacks/yarn-install/fakes"
	"github.com/sclevine/spec"

	. "github.com/onsi/gomega"
)

// testBerryInstallProcess covers the Berry-specific ShouldRun behavior. The
// classic YarnInstallProcess is covered by install_process_test.go; these
// tests are focused on the .yarnrc.yml / .pnp.cjs / .yarn/cache/ interactions
// that Berry adds.
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
}
