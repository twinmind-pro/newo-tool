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
	doc := attributesDocument{}
	for _, attr := range attrs {
		entry := attributeEntry{
			IDN:            attr.IDN,
			Value:          attr.Value,
			Title:          attr.Title,
			Description:    attr.Description,
			Group:          attr.Group,
			IsHidden:       attr.IsHidden,
			PossibleValues: attr.PossibleValues,
			ValueType:      enumWithPrefix("AttributeValueTypes", attr.ValueType),
		}
		doc.Attributes = append(doc.Attributes, entry)
	}
	return yaml.Marshal(doc)
}
