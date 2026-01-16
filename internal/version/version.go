package version

import (
	"fmt"
	"runtime"
)

var (
	version      = "dev"
	commit       = "unknown"
	buildDate    = "unknown"
	gitTreeState = "unknown"
)

// Info contains version information.
type Info struct {
	Version      string `json:"version"`
	Commit       string `json:"commit"`
	BuildDate    string `json:"buildDate"`
	GitTreeState string `json:"gitTreeState"`
	GoVersion    string `json:"goVersion"`
	Compiler     string `json:"compiler"`
	Platform     string `json:"platform"`
}

// Get returns version information.
func Get() Info {
	return Info{
		Version:      version,
		Commit:       commit,
		BuildDate:    buildDate,
		GitTreeState: gitTreeState,
		GoVersion:    runtime.Version(),
		Compiler:     runtime.Compiler,
		Platform:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns a short version string.
func (i Info) String() string {
	return fmt.Sprintf("%s (%s)", i.Version, i.Commit)
}
