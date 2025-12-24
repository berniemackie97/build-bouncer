// Package config contains build-bouncer configuration types and parsing helpers.
package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// StringList accepts either a single string or a list of strings in YAML.
type StringList []string

func (s *StringList) UnmarshalYAML(node *yaml.Node) error {
	if s == nil {
		return fmt.Errorf("StringList: UnmarshalYAML on nil receiver")
	}
	if node == nil {
		return nil
	}

	// Follow aliases to the real node.
	for node.Kind == yaml.AliasNode && node.Alias != nil {
		node = node.Alias
	}
	// Treat explicit YAML null as empty/nil.
	if node.Tag == "!!null" {
		*s = nil
		return nil
	}

	switch node.Kind {
	case yaml.ScalarNode:
		item := strings.TrimSpace(node.Value)
		if item == "" {
			*s = nil
			return nil
		}
		*s = []string{item}
		return nil

	case yaml.SequenceNode:
		out := make([]string, 0, len(node.Content))
		for _, child := range node.Content {
			if child == nil {
				continue
			}
			for child.Kind == yaml.AliasNode && child.Alias != nil {
				child = child.Alias
			}
			if child.Tag == "!!null" {
				continue
			}
			if child.Kind != yaml.ScalarNode {
				return fmt.Errorf("StringList: expected sequence of strings, got %s at line %d col %d", yamlKindName(child.Kind), child.Line, child.Column)
			}

			item := strings.TrimSpace(child.Value)
			if item == "" {
				continue
			}
			out = append(out, item)
		}
		if len(out) == 0 {
			*s = nil
			return nil
		}
		*s = out
		return nil

	default:
		return fmt.Errorf("StringList: expected string or list, got %s at line %d col %d", yamlKindName(node.Kind), node.Line, node.Column)
	}
}

func (s StringList) MarshalYAML() (any, error) {
	switch len(s) {
	case 0:
		return nil, nil
	case 1:
		return s[0], nil
	default:
		return []string(s), nil
	}
}

// IsZero helps omitempty-style behavior in serializers that consult IsZero.
func (s StringList) IsZero() bool { return len(s) == 0 }

func yamlKindName(kind yaml.Kind) string {
	switch kind {
	case yaml.DocumentNode:
		return "document"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.MappingNode:
		return "mapping"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	default:
		return fmt.Sprintf("kind(%d)", int(kind))
	}
}
