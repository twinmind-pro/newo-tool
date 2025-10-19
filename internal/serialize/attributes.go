package serialize

import (
	"gopkg.in/yaml.v3"

	"github.com/twinmind/newo-tool/internal/platform"
)

type attributesDocument struct {
	Attributes []attributeEntry `yaml:"attributes"`
}

type attributeEntry struct {
	IDN            string      `yaml:"idn"`
	Value          interface{} `yaml:"value"`
	Title          string      `yaml:"title"`
	Description    string      `yaml:"description"`
	Group          string      `yaml:"group"`
	IsHidden       bool        `yaml:"is_hidden"`
	PossibleValues []string    `yaml:"possible_values"`
	ValueType      enumString  `yaml:"value_type"`
}

func GenerateAttributesYAML(attrs []platform.CustomerAttribute) ([]byte, error) {
	doc := attributesDocument{
		Attributes: []attributeEntry{}, // Always generate an empty list
	}
	return yaml.Marshal(doc)
}
