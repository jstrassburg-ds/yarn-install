package yarninstall_test

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"

	yarninstall "github.com/paketo-buildpacks/yarn-install"
)

func testYarnrcParser(t *testing.T, context spec.G, it spec.S) {
	var (
		Expect = NewWithT(t).Expect
		tmpDir string
	)

	it.Before(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "yarn-install-test")
		Expect(err).NotTo(HaveOccurred())
	})

	it.After(func() {
		err := os.RemoveAll(tmpDir)
		Expect(err).NotTo(HaveOccurred())
	})

	context("ParseYarnrcYml", func() {
		context("when .yarnrc.yml exists", func() {
			it("parses the configuration correctly", func() {
				yarnrcContent := `nodeLinker: pnp
cacheFolder: .yarn/cache
enableImmutableInstalls: false
packageExtensions:
  "react@*":
    dependencies:
      "prop-types": "*"
`
				yarnrcPath := filepath.Join(tmpDir, ".yarnrc.yml")
				err := os.WriteFile(yarnrcPath, []byte(yarnrcContent), 0644)
				Expect(err).NotTo(HaveOccurred())

				config, err := yarninstall.ParseYarnrcYml(tmpDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(config).NotTo(BeNil())
				Expect(config.NodeLinker).To(Equal("pnp"))
				Expect(config.CacheFolder).To(Equal(".yarn/cache"))
				Expect(*config.EnableImmutableInstalls).To(BeFalse())
			})
		})

		context("when .yarnrc.yml does not exist", func() {
			it("returns nil without error", func() {
				config, err := yarninstall.ParseYarnrcYml(tmpDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(config).To(BeNil())
			})
		})
	})

	context("DetermineYarnVersion", func() {
		context("when .yarnrc.yml exists", func() {
			it("returns Berry", func() {
				yarnrcPath := filepath.Join(tmpDir, ".yarnrc.yml")
				err := os.WriteFile(yarnrcPath, []byte("nodeLinker: pnp"), 0644)
				Expect(err).NotTo(HaveOccurred())

				version, err := yarninstall.DetermineYarnVersion(tmpDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(version).To(Equal(yarninstall.YarnBerry))
			})
		})

		context("when only yarn.lock exists", func() {
			it("returns Classic", func() {
				yarnLockPath := filepath.Join(tmpDir, "yarn.lock")
				err := os.WriteFile(yarnLockPath, []byte("# yarn lockfile v1"), 0644)
				Expect(err).NotTo(HaveOccurred())

				version, err := yarninstall.DetermineYarnVersion(tmpDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(version).To(Equal(yarninstall.YarnClassic))
			})
		})

		context("when neither file exists", func() {
			it("returns empty string", func() {
				version, err := yarninstall.DetermineYarnVersion(tmpDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(version).To(Equal(""))
			})
		})
	})

	context("DetermineProvisionType", func() {
		context("when config is nil (Classic)", func() {
			it("returns node_modules", func() {
				provisionType := yarninstall.DetermineProvisionType(tmpDir, nil)
				Expect(provisionType).To(Equal(yarninstall.PlanDependencyNodeModules))
			})
		})

		context("when nodeLinker is node-modules", func() {
			it("returns node_modules", func() {
				config := &yarninstall.YarnrcConfig{
					NodeLinker: "node-modules",
				}
				provisionType := yarninstall.DetermineProvisionType(tmpDir, config)
				Expect(provisionType).To(Equal(yarninstall.PlanDependencyNodeModules))
			})
		})

		context("when nodeLinker is pnpm", func() {
			it("returns node_modules", func() {
				config := &yarninstall.YarnrcConfig{
					NodeLinker: "pnpm",
				}
				provisionType := yarninstall.DetermineProvisionType(tmpDir, config)
				Expect(provisionType).To(Equal(yarninstall.PlanDependencyNodeModules))
			})
		})

		context("when nodeLinker is pnp", func() {
			it("returns yarn_pkgs", func() {
				config := &yarninstall.YarnrcConfig{
					NodeLinker: "pnp",
				}
				provisionType := yarninstall.DetermineProvisionType(tmpDir, config)
				Expect(provisionType).To(Equal(yarninstall.PlanDependencyYarnPkgs))
			})
		})

		context("when nodeLinker is not specified (defaults to pnp for Berry)", func() {
			it("returns yarn_pkgs", func() {
				config := &yarninstall.YarnrcConfig{}
				provisionType := yarninstall.DetermineProvisionType(tmpDir, config)
				Expect(provisionType).To(Equal(yarninstall.PlanDependencyYarnPkgs))
			})
		})
	})

	context("HasPnpFiles", func() {
		context("when .pnp.cjs exists", func() {
			it("returns true", func() {
				pnpPath := filepath.Join(tmpDir, ".pnp.cjs")
				err := os.WriteFile(pnpPath, []byte("// PnP file"), 0644)
				Expect(err).NotTo(HaveOccurred())

				hasPnp, err := yarninstall.HasPnpFiles(tmpDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(hasPnp).To(BeTrue())
			})
		})

		context("when .pnp.cjs does not exist", func() {
			it("returns false", func() {
				hasPnp, err := yarninstall.HasPnpFiles(tmpDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(hasPnp).To(BeFalse())
			})
		})
	})

	context("HasYarnCache", func() {
		context("when .yarn/cache exists", func() {
			it("returns true", func() {
				cacheDir := filepath.Join(tmpDir, ".yarn", "cache")
				err := os.MkdirAll(cacheDir, 0755)
				Expect(err).NotTo(HaveOccurred())

				hasCache, err := yarninstall.HasYarnCache(tmpDir, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(hasCache).To(BeTrue())
			})
		})

		context("when custom cache folder is configured", func() {
			it("uses the custom cache folder", func() {
				customCacheDir := filepath.Join(tmpDir, "custom-cache")
				err := os.MkdirAll(customCacheDir, 0755)
				Expect(err).NotTo(HaveOccurred())

				config := &yarninstall.YarnrcConfig{
					CacheFolder: "custom-cache",
				}

				hasCache, err := yarninstall.HasYarnCache(tmpDir, config)
				Expect(err).NotTo(HaveOccurred())
				Expect(hasCache).To(BeTrue())
			})
		})

		context("when cache does not exist", func() {
			it("returns false", func() {
				hasCache, err := yarninstall.HasYarnCache(tmpDir, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(hasCache).To(BeFalse())
			})
		})
	})
}
