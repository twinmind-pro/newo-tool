package fsutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultCustomersDir = "newo_customers"
	StateDirName        = ".newo"
	lockDirName         = "locks"
	lockStaleAfter      = 15 * time.Minute

	// Directory and file permissions used across the workspace.
	DirPerm  = 0o755
	FilePerm = 0o644

	// Common directory and file names.
	ProjectsDir      = "projects"
	FlowsDir         = "flows"
	ProjectJSON      = "project.json"
	AttributesYAML   = "attributes.yaml"
	FlowsYAML        = "flows.yaml"
	MapJSON          = "map.json"
	HashesJSON       = "hashes.json"
	APIKeysJSON      = "api-keys.json"
	MetadataYAML     = "metadata.yaml"
	SkillMetaFileExt = ".meta.yaml"
)

// ErrLocked indicates the workspace is already locked by another process.
var ErrLocked = errors.New("workspace is locked")

// ExportProjectRoot returns the root directory for exported project assets.
func ExportProjectRoot(root, projectSlug string) string {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	if strings.TrimSpace(projectSlug) == "" {
		projectSlug = "project"
	}
	return filepath.Join(root, projectSlug)
}

// ExportProjectDir returns the root directory for exported project assets, including customer type and customer IDN.
func ExportProjectDir(root, customerType, customerIDN, projectSlug string) string {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	if strings.TrimSpace(customerIDN) == "" {
		customerIDN = "default_customer" // Fallback if customerIDN is empty
	}
	if strings.TrimSpace(projectSlug) == "" {
		projectSlug = "project"
	}

	switch strings.ToLower(strings.TrimSpace(customerType)) {
	case "integration":
		return filepath.Join(root, projectSlug)
	case "e2e":
		return filepath.Join(root+"_e2e", customerIDN, projectSlug)
	default:
		return filepath.Join(root, customerIDN, projectSlug)
	}
}



// ExportProjectJSONPath returns the project.json path.
func ExportProjectJSONPath(root, customerType, customerIDN, projectSlug string) string {
	return filepath.Join(ExportProjectDir(root, customerType, customerIDN, projectSlug), ProjectJSON)
}

// ExportAttributesPath returns the attributes.yaml path.
func ExportAttributesPath(root, customerType, customerIDN, projectSlug string) string {
	return filepath.Join(ExportProjectDir(root, customerType, customerIDN, projectSlug), AttributesYAML)
}

// ExportFlowsYAMLPath returns the flows.yaml path.
func ExportFlowsYAMLPath(root, customerType, customerIDN, projectSlug string) string {
	return filepath.Join(ExportProjectDir(root, customerType, customerIDN, projectSlug), FlowsYAML)
}

// ExportFlowDir returns the directory for a flow's assets.
func ExportFlowDir(root, customerType, customerIDN, projectSlug, agentIDN, flowIDN string) string {
	baseDir := ExportProjectDir(root, customerType, customerIDN, projectSlug)
	customerType = strings.ToLower(strings.TrimSpace(customerType))
	if customerType == "integration" || customerType == "e2e" {
		return filepath.Join(baseDir, FlowsDir, flowIDN)
	}
	return filepath.Join(baseDir, agentIDN, FlowsDir, flowIDN)
}

// ExportFlowMetadataPath returns the path for a flow's metadata YAML file.
func ExportFlowMetadataPath(root, customerType, customerIDN, projectSlug, agentIDN, flowIDN string) string {
	return filepath.Join(ExportFlowDir(root, customerType, customerIDN, projectSlug, agentIDN, flowIDN), MetadataYAML)
}

// ExportSkillScriptPath returns the path for a skill script under the exported structure.
func ExportSkillScriptPath(root, customerType, customerIDN, projectSlug, agentIDN, flowIDN, fileName string) string {
	return filepath.Join(ExportFlowDir(root, customerType, customerIDN, projectSlug, agentIDN, flowIDN), fileName)
}

// ExportSkillMetadataPath returns the path for a skill's metadata YAML file.
func ExportSkillMetadataPath(root, customerType, customerIDN, projectSlug, agentIDN, flowIDN, skillIDN string) string {
	return filepath.Join(ExportFlowDir(root, customerType, customerIDN, projectSlug, agentIDN, flowIDN), fmt.Sprintf("%s%s", skillIDN, SkillMetaFileExt))
}

// CustomerRoot returns the base directory for customer data.
func CustomerRoot(customerIDN string) string {
	return filepath.Join(DefaultCustomersDir, customerIDN)
}

// CustomersRoot returns the root directory containing all customers.
func CustomersRoot() string {
	return DefaultCustomersDir
}

// CustomerStateDir returns the directory storing state data for the given customer.
func CustomerStateDir(customerIDN string) string {
	return filepath.Join(StateDirName, strings.ToLower(customerIDN))
}

func EnsureDir(path string) error {
	return os.MkdirAll(path, DirPerm)
}

// EnsureWorkspace lays out the required directory structure for a customer.
func EnsureWorkspace(customerIDN string) error {
	return EnsureDir(CustomerStateDir(customerIDN))
}

// EnsureParentDir makes sure the parent directory for a file exists.
func EnsureParentDir(filePath string) error {
	dir := filepath.Dir(filePath)
	return EnsureDir(dir)
}

func lockDirectory() string {
	return filepath.Join(StateDirName, lockDirName)
}

// AcquireLock creates a lock file preventing concurrent destructive operations.
func AcquireLock(operation string) (func() error, error) {
	if err := EnsureDir(lockDirectory()); err != nil {
		return nil, fmt.Errorf("ensure lock directory: %w", err)
	}
	lockPath := filepath.Join(lockDirectory(), fmt.Sprintf("%s.lock", operation))

	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, FilePerm)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			info, statErr := os.Stat(lockPath)
			if statErr == nil {
				if time.Since(info.ModTime()) > lockStaleAfter {
					_ = os.Remove(lockPath)
					// retry once after removing stale lock
					return AcquireLock(operation)
				}
			}
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("create lock file: %w", err)
	}

	release := func() error {
		_ = file.Close()
		if err := os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove lock file: %w", err)
		}
		return nil
	}
	return release, nil
}

func projectDir(customerIDN, projectIDN string) string {
	return filepath.Join(CustomerRoot(customerIDN), ProjectsDir, projectIDN)
}

func agentDir(customerIDN, projectIDN, agentIDN string) string {
	return filepath.Join(projectDir(customerIDN, projectIDN), agentIDN)
}

func flowDir(customerIDN, projectIDN, agentIDN, flowIDN string) string {
	return filepath.Join(agentDir(customerIDN, projectIDN, agentIDN), flowIDN)
}

func skillDir(customerIDN, projectIDN, agentIDN, flowIDN, skillIDN string) string {
	return filepath.Join(flowDir(customerIDN, projectIDN, agentIDN, flowIDN), skillIDN)
}

// ProjectMetadataPath returns path to project metadata YAML.
func ProjectMetadataPath(customerIDN, projectIDN string) string {
	return filepath.Join(projectDir(customerIDN, projectIDN), MetadataYAML)
}

// ProjectDir exposes the project directory path.
func ProjectDir(customerIDN, projectIDN string) string {
	return projectDir(customerIDN, projectIDN)
}

// AgentMetadataPath returns path to agent metadata YAML.
func AgentMetadataPath(customerIDN, projectIDN, agentIDN string) string {
	return filepath.Join(agentDir(customerIDN, projectIDN, agentIDN), MetadataYAML)
}

// AgentDir exposes the agent directory path.
func AgentDir(customerIDN, projectIDN, agentIDN string) string {
	return agentDir(customerIDN, projectIDN, agentIDN)
}

// FlowMetadataPath returns path to flow metadata YAML.
func FlowMetadataPath(customerIDN, projectIDN, agentIDN, flowIDN string) string {
	return filepath.Join(flowDir(customerIDN, projectIDN, agentIDN, flowIDN), MetadataYAML)
}

// FlowDir exposes the flow directory path.
func FlowDir(customerIDN, projectIDN, agentIDN, flowIDN string) string {
	return flowDir(customerIDN, projectIDN, agentIDN, flowIDN)
}

// SkillMetadataPath returns path to skill metadata YAML.
func SkillMetadataPath(customerIDN, projectIDN, agentIDN, flowIDN, skillIDN string) string {
	return filepath.Join(skillDir(customerIDN, projectIDN, agentIDN, flowIDN, skillIDN), MetadataYAML)
}

// SkillDir exposes the skill directory path.
func SkillDir(customerIDN, projectIDN, agentIDN, flowIDN, skillIDN string) string {
	return skillDir(customerIDN, projectIDN, agentIDN, flowIDN, skillIDN)
}

// SkillScriptPath returns path for the skill script file using IDN-based naming.
func SkillScriptPath(customerIDN, projectIDN, agentIDN, flowIDN, skillIDN, extension string) string {
	return filepath.Join(skillDir(customerIDN, projectIDN, agentIDN, flowIDN, skillIDN), fmt.Sprintf("%s.%s", skillIDN, extension))
}

// MapPath returns the map.json path.
func MapPath(customerIDN string) string {
	return filepath.Join(CustomerStateDir(customerIDN), MapJSON)
}

// HashesPath returns hashes.json path.
func HashesPath(customerIDN string) string {
	return filepath.Join(CustomerStateDir(customerIDN), HashesJSON)
}

// AttributesPath returns attributes.yaml path.
func AttributesPath(customerIDN string) string {
	return filepath.Join(CustomerRoot(customerIDN), AttributesYAML)
}

// FlowsYAMLPath returns flows.yaml path.
func FlowsYAMLPath(customerIDN string) string {
	return filepath.Join(CustomerRoot(customerIDN), ProjectsDir, FlowsYAML)
}

// APIKeyRegistryPath returns the path to the API key registry file.
func APIKeyRegistryPath() string {
	return filepath.Join(StateDirName, APIKeysJSON)
}
