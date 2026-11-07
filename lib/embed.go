package libsonnet

import "embed"

//go:embed *.*sonnet knit/*.libsonnet knit/schema
var Assets embed.FS
