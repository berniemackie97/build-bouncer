package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// StringList accepts either a single string or a list of strings in YAML.
type StringList []string

func (s *StringList) UnmarshalYAML(value *yaml.Node) error {
	if value == nil {
		return nil
	}
	switch value.Kind {
	case yaml.ScalarNode:
		item := strings.TrimSpace(value.Value)
		if item == "" {
			*s = nil
			return nil
		}
		*s = []string{item}
		return nil
	case yaml.SequenceNode:
		out := make([]string, 0, len(value.Content))
		for _, n := range value.Content {
			if n.Kind != yaml.ScalarNode {
				return fmt.Errorf("expected string list, got %v", n.Kind)
			}
			item := strings.TrimSpace(n.Value)
			if item == "" {
				continue
			}
			out = append(out, item)
		}
		*s = out
		return nil
	default:
		return fmt.Errorf("expected string or list, got %v", value.Kind)
	}
}

func (s StringList) MarshalYAML() (interface{}, error) {
	if len(s) == 1 {
		return s[0], nil
	}
	return []string(s), nil
}
