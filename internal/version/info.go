package version

// Build-time variables. They can be overridden with -ldflags "-X".
var (
	Version = "dev"
	Commit  = "unknown"
)
