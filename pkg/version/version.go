package version

import "runtime/debug"

// Default version if not overriden by build parameters.
const DevVersion = "(dev build)"

// Version is overridden by main
var Version = DevVersion

func Get() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return Version
}
