package deploy

import "errors"

var (
	// ErrProjectNotFound indicates the requested project is absent from the state map.
	ErrProjectNotFound = errors.New("project not found in state map")
	// ErrProjectDirMissing indicates the expected project directory cannot be located.
	ErrProjectDirMissing = errors.New("project directory not found")
	// ErrProjectJSONMissing indicates project.json is missing from the project directory.
	ErrProjectJSONMissing = errors.New("project.json not found")
	// ErrSkillScriptMissing indicates the script file for a skill is missing.
	ErrSkillScriptMissing = errors.New("skill script not found")
)
