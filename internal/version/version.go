package version

import (
	"fmt"
	"runtime"
)

// Build information, injected at compile time via ldflags:
//
//	go build -ldflags "-X github.com/user/tgbot/internal/version.GitCommit=abc123
//	                   -X github.com/user/tgbot/internal/version.GitDate=2026-02-04
//	                   -X github.com/user/tgbot/internal/version.GitBranch=main"
var (
	// GitCommit is the git commit hash
	GitCommit = "unknown"

	// GitDate is the commit date (ISO format)
	GitDate = "unknown"

	// GitBranch is the git branch name
	GitBranch = "unknown"

	// BuildDate is when the binary was built
	BuildDate = "unknown"
)

// Info holds version information
type Info struct {
	GitCommit string
	GitDate   string
	GitBranch string
	BuildDate string
	GoVersion string
	Platform  string
}

// Get returns current version information
func Get() Info {
	return Info{
		GitCommit: GitCommit,
		GitDate:   GitDate,
		GitBranch: GitBranch,
		BuildDate: BuildDate,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// String returns a formatted version string
func (i Info) String() string {
	return fmt.Sprintf("commit=%s date=%s branch=%s built=%s go=%s platform=%s",
		i.GitCommit, i.GitDate, i.GitBranch, i.BuildDate, i.GoVersion, i.Platform)
}

// Short returns abbreviated version info
func (i Info) Short() string {
	commit := i.GitCommit
	if len(commit) > 7 {
		commit = commit[:7]
	}
	return fmt.Sprintf("%s (%s)", commit, i.GitDate)
}

// LogFields returns version info as key-value pairs for structured logging
func (i Info) LogFields() []any {
	return []any{
		"git_commit", i.GitCommit,
		"git_date", i.GitDate,
		"git_branch", i.GitBranch,
		"build_date", i.BuildDate,
		"go_version", i.GoVersion,
		"platform", i.Platform,
	}
}
