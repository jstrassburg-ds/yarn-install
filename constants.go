package yarninstall

const (
	PlanDependencyNodeModules = "node_modules"
	PlanDependencyYarnPkgs    = "yarn_pkgs"
	PlanDependencyNode        = "node"
	PlanDependencyYarn        = "yarn"

	// Yarn version constants
	YarnClassic = "classic"
	YarnBerry   = "berry"

	// Yarn configuration files
	YarnrcYml = ".yarnrc.yml"
	YarnrcJs  = ".yarnrc"
	YarnLock  = "yarn.lock"
	PnpCjs    = ".pnp.cjs"

	// Node linker options
	NodeLinkerPnP         = "pnp"
	NodeLinkerNodeModules = "node-modules"
	NodeLinkerPnpm        = "pnpm"
)
