// Package version reports which build this is.
package version

// Version is the release this binary was built from. Overridden at link time
// by the release workflow, and kept in step with herdr-plugin.toml so
// `herdr plugin list` and `ekanban --version` cannot disagree.
var Version = "dev"
