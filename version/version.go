package version

import (
	"fmt"
)

var (
	Version        = "dev"
	Branch         = "n/a"
	CommitHash     = "n/a"
	BuildTimestamp = "n/a"
	BuiltBy        = "n/a"
)

func BuildVersion() string {
	return fmt.Sprintf("%s %s:%s (%s, %s)", Version, Branch, CommitHash, BuildTimestamp, BuiltBy)
}
