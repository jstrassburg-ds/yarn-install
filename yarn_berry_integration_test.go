package yarninstall_test

import (
	"os"
	"path/filepath"
	"testing"

	yarninstall "github.com/paketo-buildpacks/yarn-install"
	"github.com/sclevine/spec"

	. "github.com/onsi/gomega"
)

func testYarnBerryIntegration(t *testing.T, context spec.G, it spec.S) {
	var (
		Expect     = NewWithT(t).Expect
		workingDir string
	)

	it.Before(func() {
		var err error
		workingDir, err = os.MkdirTemp("", "yarn-berry-test")
		Expect(err).NotTo(HaveOccurred())
	})

	it.After(func() {
		Expect(os.RemoveAll(workingDir)).To(Succeed())
	})

	context("with .yarnrc.yml file", func() {
		it.Before(func() {
			err := os.WriteFile(filepath.Join(workingDir, ".yarnrc.yml"), []byte("nodeLinker: pnp\nyarnPath: .yarn/releases/yarn-4.0.0.cjs"), 0644)
			Expect(err).NotTo(HaveOccurred())
		})

		it("detects Yarn Berry", func() {
			version, err := yarninstall.DetermineYarnVersion(workingDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(version).To(Equal(yarninstall.YarnBerry))
		})

		it("determines PnP provision type", func() {
			config, err := yarninstall.ParseYarnrcYml(workingDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(config).NotTo(BeNil())

			provisionType := yarninstall.DetermineProvisionType(workingDir, config)
			Expect(provisionType).To(Equal(yarninstall.PlanDependencyYarnPkgs))
		})
	})

	context("with .yarnrc.yml file and node-modules linker", func() {
		it.Before(func() {
			err := os.WriteFile(filepath.Join(workingDir, ".yarnrc.yml"), []byte("nodeLinker: node-modules\nyarnPath: .yarn/releases/yarn-4.0.0.cjs"), 0644)
			Expect(err).NotTo(HaveOccurred())
		})

		it("determines node_modules provision type", func() {
			config, err := yarninstall.ParseYarnrcYml(workingDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(config).NotTo(BeNil())

			provisionType := yarninstall.DetermineProvisionType(workingDir, config)
			Expect(provisionType).To(Equal(yarninstall.PlanDependencyNodeModules))
		})
	})

	context("with only yarn.lock file", func() {
		it.Before(func() {
			err := os.WriteFile(filepath.Join(workingDir, "yarn.lock"), []byte("# Yarn classic lock file"), 0644)
			Expect(err).NotTo(HaveOccurred())
		})

		it("detects Yarn Classic", func() {
			version, err := yarninstall.DetermineYarnVersion(workingDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(version).To(Equal(yarninstall.YarnClassic))
		})

		it("determines node_modules provision type", func() {
			config, err := yarninstall.ParseYarnrcYml(workingDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(config).To(BeNil())

			provisionType := yarninstall.DetermineProvisionType(workingDir, config)
			Expect(provisionType).To(Equal(yarninstall.PlanDependencyNodeModules))
		})
	})
}
