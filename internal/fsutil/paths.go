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
	defaultCustomersDir = "newo_customers"
	stateDirName        = ".newo"
	lockDirName         = "locks"
	lockStaleAfter      = 15 * time.Minute

	// Directory and file permissions used across the workspace.
	DirPerm  = 0o755
	FilePerm = 0o644
)

// ErrLocked indicates the workspace is already locked by another process.
var ErrLocked = errors.New("workspace is locked")

// ExportProjectDir returns the root directory for exported project assets.
func ExportProjectDir(root, projectSlug string) string {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	if strings.TrimSpace(projectSlug) == "" {
		projectSlug = "project"
	}
	return filepath.Join(root, projectSlug)
}

// ExportProjectJSONPath returns the project.json path.
func ExportProjectJSONPath(root, projectSlug string) string {
	return filepath.Join(ExportProjectDir(root, projectSlug), "project.json")
}

// ExportAttributesPath returns the attributes.yaml path.
func ExportAttributesPath(root, projectSlug string) string {
	return filepath.Join(ExportProjectDir(root, projectSlug), "attributes.yaml")
}

// ExportFlowsYAMLPath returns the flows.yaml path.
func ExportFlowsYAMLPath(root, projectSlug string) string {
	return filepath.Join(ExportProjectDir(root, projectSlug), "flows.yaml")
}

// ExportFlowDir returns the directory for flow scripts.
func ExportFlowDir(root, projectSlug, flowIDN string) string {
	return filepath.Join(ExportProjectDir(root, projectSlug), "flows", flowIDN)
}

// ExportSkillScriptPath returns the path for a skill script under the exported structure.
func ExportSkillScriptPath(root, projectSlug, flowIDN, fileName string) string {
	return filepath.Join(ExportFlowDir(root, projectSlug, flowIDN), fileName)
}

// CustomerRoot returns the base directory for customer data.
func CustomerRoot(customerIDN string) string {
	return filepath.Join(defaultCustomersDir, customerIDN)
}

// CustomersRoot returns the root directory containing all customers.
func CustomersRoot() string {
	return defaultCustomersDir
}

// CustomerStateDir returns the directory storing state data for the given customer.
func CustomerStateDir(customerIDN string) string {
	return filepath.Join(stateDirName, strings.ToLower(customerIDN))
}

func ensureDir(path string) error {
	return os.MkdirAll(path, DirPerm)
}

// EnsureWorkspace lays out the required directory structure for a customer.
func EnsureWorkspace(customerIDN string) error {
	return ensureDir(CustomerStateDir(customerIDN))
}

// EnsureParentDir makes sure the parent directory for a file exists.
func EnsureParentDir(filePath string) error {
	dir := filepath.Dir(filePath)
	return ensureDir(dir)
}

func lockDirectory() string {
	return filepath.Join(stateDirName, lockDirName)
}

// AcquireLock creates a lock file preventing concurrent destructive operations.
func AcquireLock(operation string) (func() error, error) {
	if err := ensureDir(lockDirectory()); err != nil {
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
	return filepath.Join(CustomerRoot(customerIDN), "projects", projectIDN)
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
	return filepath.Join(projectDir(customerIDN, projectIDN), "metadata.yaml")
}

// ProjectDir exposes the project directory path.
func ProjectDir(customerIDN, projectIDN string) string {
	return projectDir(customerIDN, projectIDN)
}

// AgentMetadataPath returns path to agent metadata YAML.
func AgentMetadataPath(customerIDN, projectIDN, agentIDN string) string {
	return filepath.Join(agentDir(customerIDN, projectIDN, agentIDN), "metadata.yaml")
}

// AgentDir exposes the agent directory path.
func AgentDir(customerIDN, projectIDN, agentIDN string) string {
	return agentDir(customerIDN, projectIDN, agentIDN)
}

// FlowMetadataPath returns path to flow metadata YAML.
func FlowMetadataPath(customerIDN, projectIDN, agentIDN, flowIDN string) string {
	return filepath.Join(flowDir(customerIDN, projectIDN, agentIDN, flowIDN), "metadata.yaml")
}

// FlowDir exposes the flow directory path.
func FlowDir(customerIDN, projectIDN, agentIDN, flowIDN string) string {
	return flowDir(customerIDN, projectIDN, agentIDN, flowIDN)
}

// SkillMetadataPath returns path to skill metadata YAML.
func SkillMetadataPath(customerIDN, projectIDN, agentIDN, flowIDN, skillIDN string) string {
	return filepath.Join(skillDir(customerIDN, projectIDN, agentIDN, flowIDN, skillIDN), "metadata.yaml")
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
	return filepath.Join(CustomerStateDir(customerIDN), "map.json")
}

// HashesPath returns hashes.json path.
func HashesPath(customerIDN string) string {
	return filepath.Join(CustomerStateDir(customerIDN), "hashes.json")
}

// AttributesPath returns attributes.yaml path.
func AttributesPath(customerIDN string) string {
	return filepath.Join(CustomerRoot(customerIDN), "attributes.yaml")
}

// FlowsYAMLPath returns flows.yaml path.
func FlowsYAMLPath(customerIDN string) string {
	return filepath.Join(CustomerRoot(customerIDN), "projects", "flows.yaml")
}

// APIKeyRegistryPath returns the path to the API key registry file.
func APIKeyRegistryPath() string {
	return filepath.Join(stateDirName, "api-keys.json")
}
