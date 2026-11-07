package main

import (
	"bytes"
	"fmt"
	"os"
	"reflect"

	"github.com/berniemackie97/build-bouncer/internal/config"
	"github.com/berniemackie97/build-bouncer/internal/storage"
	"gopkg.in/yaml.v3"
)

func saveConfigured(path string, before, after *config.Config) error {
	original, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err := config.Parse(original); err != nil {
		return err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(original, &document); err != nil {
		return err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("configuration must contain a YAML mapping")
	}
	mapping := document.Content[0]
	if !reflect.DeepEqual(before.Checks, after.Checks) {
		sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		old := make(map[string]*yaml.Node, len(before.Checks))
		for i, node := range mapping.Content {
			if i%2 == 0 && node.Value == "checks" {
				if i+1 < len(mapping.Content) && mapping.Content[i+1].Kind == yaml.SequenceNode {
					for _, item := range mapping.Content[i+1].Content {
						var check config.Check
						if err := item.Decode(&check); err != nil {
							return err
						}
						old[check.Name] = item
					}
				}
				break
			}
		}
		previous := make(map[string]config.Check, len(before.Checks))
		for _, check := range before.Checks {
			previous[check.Name] = check
		}
		for _, check := range after.Checks {
			if existing, ok := previous[check.Name]; ok && reflect.DeepEqual(existing, check) {
				if node := old[check.Name]; node != nil {
					sequence.Content = append(sequence.Content, node)
					continue
				}
			}
			var node yaml.Node
			if err := node.Encode(check); err != nil {
				return err
			}
			sequence.Content = append(sequence.Content, &node)
		}
		replaceMappingValue(mapping, "checks", sequence)
	}
	if before.Runner.MaxParallel != after.Runner.MaxParallel || before.Runner.FailFast != after.Runner.FailFast {
		runner := mappingValue(mapping, "runner")
		if runner == nil {
			runner = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			replaceMappingValue(mapping, "runner", runner)
		}
		if runner.Kind != yaml.MappingNode {
			return fmt.Errorf("runner must be a YAML mapping")
		}
		if before.Runner.MaxParallel != after.Runner.MaxParallel {
			replaceMappingValue(runner, "maxParallel", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprint(after.Runner.MaxParallel)})
		}
		if before.Runner.FailFast != after.Runner.FailFast {
			replaceMappingValue(runner, "failFast", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprint(after.Runner.FailFast)})
		}
	}
	if before.Protection.Level != after.Protection.Level {
		protection := mappingValue(mapping, "protection")
		if protection == nil {
			protection = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			replaceMappingValue(mapping, "protection", protection)
		}
		if protection.Kind != yaml.MappingNode {
			return fmt.Errorf("protection must be a YAML mapping")
		}
		replaceMappingValue(protection, "level", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: after.Protection.Level})
	}
	if !reflect.DeepEqual(before.Meta, after.Meta) {
		var node yaml.Node
		if err := node.Encode(after.Meta); err != nil {
			return err
		}
		replaceMappingValue(mapping, "meta", &node)
	}
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	if _, err := config.Parse(buf.Bytes()); err != nil {
		return fmt.Errorf("configuration would be invalid: %w", err)
	}
	if current, err := os.ReadFile(path); err != nil {
		return err
	} else if !bytes.Equal(original, current) {
		return fmt.Errorf("configuration changed during editing; retry")
	}
	return storage.WriteFile(path, buf.Bytes(), 0o600)
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func replaceMappingValue(mapping *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}
