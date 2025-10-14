package serialize

import (
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/twinmind/newo-tool/internal/platform"
	"github.com/twinmind/newo-tool/internal/state"
)

type enumString string

func (e enumString) MarshalYAML() (interface{}, error) {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!enum", Value: string(e), Style: yaml.DoubleQuotedStyle}
	return node, nil
}

type flowsDocument struct {
	Flows []agentEntry `yaml:"flows"`
}

type agentEntry struct {
	AgentIDN         string      `yaml:"agent_idn"`
	AgentDescription interface{} `yaml:"agent_description"`
	AgentFlows       []flowEntry `yaml:"agent_flows"`
}

type flowEntry struct {
	IDN                string            `yaml:"idn"`
	Title              string            `yaml:"title"`
	Description        string            `yaml:"description"`
	DefaultRunnerType  enumString        `yaml:"default_runner_type"`
	DefaultProviderIDN interface{}       `yaml:"default_provider_idn"`
	DefaultModelIDN    interface{}       `yaml:"default_model_idn"`
	Skills             []skillEntry      `yaml:"skills"`
	Events             []eventEntry      `yaml:"events"`
	StateFields        []stateFieldEntry `yaml:"state_fields"`
}

type skillEntry struct {
	IDN          string            `yaml:"idn"`
	Title        string            `yaml:"title"`
	PromptScript string            `yaml:"prompt_script"`
	RunnerType   enumString        `yaml:"runner_type"`
	Model        map[string]string `yaml:"model"`
	Parameters   []parameterEntry  `yaml:"parameters"`
}

type parameterEntry struct {
	Name         string      `yaml:"name"`
	DefaultValue interface{} `yaml:"default_value"`
}

type eventEntry struct {
	Title          interface{} `yaml:"title"`
	IDN            string      `yaml:"idn"`
	SkillSelector  enumString  `yaml:"skill_selector"`
	SkillIDN       string      `yaml:"skill_idn"`
	StateIDN       interface{} `yaml:"state_idn"`
	IntegrationIDN interface{} `yaml:"integration_idn"`
	ConnectorIDN   interface{} `yaml:"connector_idn"`
	InterruptMode  enumString  `yaml:"interrupt_mode"`
}

type stateFieldEntry struct {
	IDN          string      `yaml:"idn"`
	Title        string      `yaml:"title"`
	DefaultValue interface{} `yaml:"default_value"`
	Scope        enumString  `yaml:"scope"`
}

func GenerateFlowsYAML(project platform.Project, data state.ProjectData) ([]byte, error) {
	agentIDs := sortedKeys(data.Agents)
	doc := flowsDocument{}
	for _, agentID := range agentIDs {
		agentData := data.Agents[agentID]
		agent := agentEntry{AgentIDN: agentID}
		if agentData.Description != "" {
			agent.AgentDescription = agentData.Description
		} else {
			agent.AgentDescription = nil
		}

		flowIDs := sortedFlowKeys(agentData.Flows)
		for _, flowID := range flowIDs {
			flowData := agentData.Flows[flowID]
			fe := flowEntry{
				IDN:                flowID,
				Title:              flowData.Title,
				Description:        flowData.Description,
				DefaultRunnerType:  enumWithPrefix("RunnerType", flowData.RunnerType),
				DefaultProviderIDN: valueOrNil(flowData.Model["provider_idn"]),
				DefaultModelIDN:    valueOrNil(flowData.Model["model_idn"]),
			}

			skillIDs := sortedSkillKeys(flowData.Skills)
			for _, skillID := range skillIDs {
				skill := flowData.Skills[skillID]
				se := skillEntry{
					IDN:          skill.IDN,
					Title:        skill.Title,
					PromptScript: skill.Path,
					RunnerType:   enumWithPrefix("RunnerType", skill.RunnerType),
					Model:        skill.Model,
					Parameters:   convertParameters(skill.Parameters),
				}
				fe.Skills = append(fe.Skills, se)
			}

			for _, ev := range flowData.Events {
				fe.Events = append(fe.Events, eventEntry{
					Title:          nilIfEmpty(ev.Title),
					IDN:            ev.IDN,
					SkillSelector:  enumWithPrefix("SkillSelector", ev.SkillSelector),
					SkillIDN:       ev.SkillIDN,
					StateIDN:       nilIfEmpty(ev.StateIDN),
					IntegrationIDN: nilIfEmpty(ev.IntegrationIDN),
					ConnectorIDN:   nilIfEmpty(ev.ConnectorIDN),
					InterruptMode:  enumWithPrefix("InterruptMode", ev.InterruptMode),
				})
			}

			for _, st := range flowData.StateFields {
				fe.StateFields = append(fe.StateFields, stateFieldEntry{
					IDN:          st.IDN,
					Title:        st.Title,
					DefaultValue: nilIfEmpty(st.DefaultValue),
					Scope:        enumWithPrefix("StateFieldScope", st.Scope),
				})
			}

			agent.AgentFlows = append(agent.AgentFlows, fe)
		}

		doc.Flows = append(doc.Flows, agent)
	}

	return yaml.Marshal(doc)
}

func sortedKeys(m map[string]state.AgentData) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedFlowKeys(m map[string]state.FlowData) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedSkillKeys(m map[string]state.SkillMetadataInfo) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func convertParameters(params []map[string]any) []parameterEntry {
	if len(params) == 0 {
		return nil
	}
	result := make([]parameterEntry, 0, len(params))
	for _, p := range params {
		name, _ := p["name"].(string)
		value := p["default_value"]
		result = append(result, parameterEntry{Name: name, DefaultValue: value})
	}
	return result
}

func enumWithPrefix(prefix, value string) enumString {
	v := strings.TrimSpace(value)
	if v == "" {
		return enumString(prefix + ".none")
	}
	lowerPrefix := strings.ToLower(prefix)
	lowerValue := strings.ToLower(v)
	if strings.HasPrefix(lowerValue, lowerPrefix+".") {
		return enumString(v)
	}
	v = strings.ReplaceAll(v, " ", "_")
	v = strings.ReplaceAll(v, "-", "_")
	v = strings.ToLower(v)
	return enumString(prefix + "." + v)
}

func valueOrNil(value string) interface{} {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nilIfEmpty(value string) interface{} {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
