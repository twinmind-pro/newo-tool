package status

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/twinmind/newo-tool/internal/fsutil"
	"github.com/twinmind/newo-tool/internal/state"
	"github.com/twinmind/newo-tool/internal/util"
)

var scriptExtensions = map[string]bool{
	".nsl":      true,
	".jinja":    true,
	".guidance": true,
}

// Run performs a status scan for a single customer and reports changes.
func Run(customerIDN string, outputRoot string, verbose bool, stdout io.Writer, _ io.Writer) (int, error) {
	mapPath := fsutil.MapPath(customerIDN)
	if _, err := os.Stat(mapPath); err != nil {
		if os.IsNotExist(err) {
			_, _ = fmt.Fprintf(stdout, "No map for customer %s. Run `newo pull --customer %s` first.\n", customerIDN, customerIDN)
			return 0, nil
		}
		return 0, fmt.Errorf("stat map file: %w", err)
	}

	projectMap, err := state.LoadProjectMap(customerIDN)
	if err != nil {
		return 0, err
	}
	hashes, err := state.LoadHashes(customerIDN)
	if err != nil {
		return 0, err
	}

	if projectMap.Projects == nil {
		projectMap.Projects = map[string]state.ProjectData{}
	}

	if len(projectMap.Projects) == 0 {
		if len(hashes) == 0 {
			_, _ = fmt.Fprintf(stdout, "No tracked files for %s. Run `newo pull --customer %s` first.\n", customerIDN, customerIDN)
			return 0, nil
		}
		for path := range hashes {
			_, _ = fmt.Fprintf(stdout, "D  %s (no project mapping)\n", toSlash(path))
		}
		_, _ = fmt.Fprintf(stdout, "%d changed file(s).\n", len(hashes))
		return len(hashes), nil
	}

	if len(hashes) == 0 {
		_, _ = fmt.Fprintf(stdout, "No hash snapshot for %s. Run `newo pull --customer %s` to initialise tracking.\n", customerIDN, customerIDN)
		return 0, nil
	}

	dirty := 0
	hashKeys := make([]string, 0, len(hashes))
	for path := range hashes {
		hashKeys = append(hashKeys, path)
	}
	sort.Strings(hashKeys)

	for _, relPath := range hashKeys {
		oldHash := hashes[relPath]
		absPath := filepath.Clean(relPath)
		data, err := os.ReadFile(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				if verbose {
					_, _ = fmt.Fprintf(stdout, "• %s\n", toSlash(relPath))
					_, _ = fmt.Fprintf(stdout, "  old: %s\n", emptyIf(oldHash, "none"))
					_, _ = fmt.Fprintf(stdout, "  new: missing\n")
				}
				_, _ = fmt.Fprintf(stdout, "D  %s\n", toSlash(relPath))
				dirty++
				continue
			}
			return dirty, fmt.Errorf("read %s: %w", relPath, err)
		}
		newHash := util.SHA256Bytes(data)
		if verbose {
			_, _ = fmt.Fprintf(stdout, "• %s\n", toSlash(relPath))
			_, _ = fmt.Fprintf(stdout, "  old: %s\n", emptyIf(oldHash, "none"))
			_, _ = fmt.Fprintf(stdout, "  new: %s\n", newHash)
		}
		if newHash != oldHash {
			_, _ = fmt.Fprintf(stdout, "M  %s\n", toSlash(relPath))
			dirty++
		}
	}

	tracked := make(map[string]struct{}, len(hashes))
	for path := range hashes {
		tracked[toSlash(path)] = struct{}{}
	}

	for projectIDN, projectData := range projectMap.Projects {
		projectDir := resolveProjectDir(outputRoot, customerIDN, projectIDN, projectData)
		flowBase := filepath.Join(projectDir, fsutil.FlowsDir)
		flowEntries, err := os.ReadDir(flowBase)
		if err != nil {
			if errorsIs(err, fs.ErrNotExist) {
				continue
			}
			return dirty, fmt.Errorf("read flows directory %s: %w", flowBase, err)
		}

		expectedFlows := expectedFlowMap(projectData)

		for _, entry := range flowEntries {
			if !entry.IsDir() {
				continue
			}
			flowIDN := entry.Name()
			flowPath := filepath.Join(flowBase, flowIDN)
			relFlow := toSlash(flowPath)
			info, knownFlow := expectedFlows[flowIDN]
			if !knownFlow {
				_, _ = fmt.Fprintf(stdout, "A  %s (new flow)\n", relFlow)
				dirty++
				continue
			}

			expectedSkills := expectedSkillSet(info)
			files, err := os.ReadDir(flowPath)
			if err != nil {
				return dirty, fmt.Errorf("scan flow %s: %w", flowPath, err)
			}

			for _, file := range files {
				if file.IsDir() {
					continue
				}
				ext := strings.ToLower(filepath.Ext(file.Name()))
				if !scriptExtensions[ext] {
					continue
				}
				rel := toSlash(filepath.Join(flowPath, file.Name()))
				if _, tracked := tracked[rel]; tracked {
					continue
				}
				if _, expected := expectedSkills[strings.ToLower(file.Name())]; expected {
					continue
				}
				_, _ = fmt.Fprintf(stdout, "A  %s (new skill)\n", rel)
				dirty++
			}
		}
	}

	if dirty == 0 {
		_, _ = fmt.Fprintln(stdout, "No changes detected.")
	} else {
		_, _ = fmt.Fprintf(stdout, "%d changed file(s).\n", dirty)
	}

	return dirty, nil
}

type flowDetails struct {
	Skills map[string]state.SkillMetadataInfo
}

func expectedFlowMap(project state.ProjectData) map[string]flowDetails {
	flows := make(map[string]flowDetails)
	for _, agentData := range project.Agents {
		for flowIDN, flowData := range agentData.Flows {
			flows[flowIDN] = flowDetails{Skills: flowData.Skills}
		}
	}
	return flows
}

func expectedSkillSet(details flowDetails) map[string]struct{} {
	set := make(map[string]struct{})
	for _, skill := range details.Skills {
		name := strings.ToLower(skill.Path)
		if name == "" {
			name = strings.ToLower(skill.IDN)
		}
		if filepath.Ext(name) == "" {
			name += ".nsl"
		}
		set[name] = struct{}{}
	}
	return set
}

func resolveProjectDir(outputRoot, customerIDN, projectIDN string, project state.ProjectData) string {
	candidates := []string{}
	if project.Path != "" {
		candidates = append(candidates, filepath.Join(outputRoot, project.Path))
	}
	if outputRoot != "" {
		candidates = append(candidates,
			filepath.Join(outputRoot, projectIDN),
			filepath.Join(outputRoot, strings.ToLower(projectIDN)))
	}
	legacy := filepath.Join(fsutil.DefaultCustomersDir, strings.ToLower(customerIDN), fsutil.ProjectsDir, projectIDN)
	legacyAlt := filepath.Join(fsutil.DefaultCustomersDir, strings.ToLower(customerIDN), projectIDN)
	candidates = append(candidates, legacy, legacyAlt)

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if stat, err := os.Stat(candidate); err == nil && stat.IsDir() {
			return candidate
		}
	}

	if len(candidates) > 0 {
		return candidates[0]
	}
	return filepath.Join(outputRoot, projectIDN)
}

func toSlash(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func errorsIs(err error, target error) bool {
	return err != nil && errors.Is(err, target)
}

func emptyIf(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
