package platform

import "strings"

// ScriptExtension returns the file extension associated with a runner type.
func ScriptExtension(runnerType string) string {
	switch strings.ToLower(runnerType) {
	case "nsl":
		return "nsl"
	case "guidance":
		return "guidance"
	default:
		return "txt"
	}
}
