package management

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"gopkg.in/yaml.v3"
)

func normalizeConfigYAML(body []byte) ([]byte, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 || root.Content[0] == nil {
		return nil, nil
	}
	top := root.Content[0]
	if top.Kind != yaml.MappingNode {
		return nil, nil
	}
	idx := findMapKeyIndex(top, "api-keys")
	if idx < 0 {
		return nil, nil
	}
	apiKeysNode := top.Content[idx+1]
	if apiKeysNode == nil || apiKeysNode.Kind != yaml.SequenceNode {
		return nil, nil
	}

	var (
		keys         []string
		keySet       = make(map[string]struct{})
		newEntries   = make(map[string]config.APIKeyEntry)
		changed      bool
		entriesError error
	)

	for _, item := range apiKeysNode.Content {
		if item == nil {
			continue
		}
		switch item.Kind {
		case yaml.ScalarNode:
			key := strings.TrimSpace(item.Value)
			if key == "" {
				continue
			}
			if _, exists := keySet[key]; !exists {
				keySet[key] = struct{}{}
				keys = append(keys, key)
			}
		case yaml.MappingNode:
			entry, ok, err := parseAPIKeyEntry(item)
			if err != nil {
				entriesError = err
				break
			}
			if !ok || entry.APIKey == "" {
				continue
			}
			changed = true
			if _, exists := keySet[entry.APIKey]; !exists {
				keySet[entry.APIKey] = struct{}{}
				keys = append(keys, entry.APIKey)
			}
			newEntries[entry.APIKey] = entry
		}
		if entriesError != nil {
			break
		}
	}
	if entriesError != nil {
		return nil, entriesError
	}
	if !changed {
		return nil, nil
	}

	apiKeysNode.Content = buildScalarSequence(keys)

	existingEntries, existingOrder := readAPIKeyEntries(top)
	finalEntries := mergeAPIKeyEntries(existingEntries, existingOrder, newEntries, keys)
	setAPIKeyEntries(top, finalEntries)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return nil, err
	}
	_ = enc.Close()
	return buf.Bytes(), nil
}

func findMapKeyIndex(mapNode *yaml.Node, key string) int {
	if mapNode == nil || mapNode.Kind != yaml.MappingNode {
		return -1
	}
	for i := 0; i < len(mapNode.Content)-1; i += 2 {
		k := mapNode.Content[i]
		if k != nil && k.Kind == yaml.ScalarNode && k.Value == key {
			return i
		}
	}
	return -1
}

func parseAPIKeyEntry(node *yaml.Node) (config.APIKeyEntry, bool, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return config.APIKeyEntry{}, false, nil
	}
	key := mappingScalarValue(node, "api-key")
	if key == "" {
		key = mappingScalarValue(node, "key")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return config.APIKeyEntry{}, false, nil
	}
	entry := config.APIKeyEntry{APIKey: key}

	dailyLimit := mappingScalarValue(node, "daily-token-limit")
	if dailyLimit == "" {
		dailyLimit = mappingScalarValue(node, "daily-limit")
	}
	dailyLimit = strings.TrimSpace(dailyLimit)
	if dailyLimit != "" {
		value, err := strconv.ParseInt(dailyLimit, 10, 64)
		if err != nil {
			return config.APIKeyEntry{}, false, fmt.Errorf("invalid daily-token-limit for api key %s: %v", key, err)
		}
		entry.DailyTokenLimit = value
	}

	expiresAt := strings.TrimSpace(mappingScalarValue(node, "expires-at"))
	if expiresAt != "" {
		entry.ExpiresAt = expiresAt
	}

	return entry, true, nil
}

func mappingScalarValue(node *yaml.Node, key string) string {
	if node == nil || node.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i < len(node.Content)-1; i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if k == nil || v == nil {
			continue
		}
		if k.Kind == yaml.ScalarNode && k.Value == key && v.Kind == yaml.ScalarNode {
			return v.Value
		}
	}
	return ""
}

func buildScalarSequence(values []string) []*yaml.Node {
	if len(values) == 0 {
		return []*yaml.Node{}
	}
	out := make([]*yaml.Node, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
	}
	return out
}

func readAPIKeyEntries(root *yaml.Node) (map[string]config.APIKeyEntry, []string) {
	result := make(map[string]config.APIKeyEntry)
	var order []string
	idx := findMapKeyIndex(root, "api-key-entries")
	if idx < 0 {
		return result, order
	}
	entriesNode := root.Content[idx+1]
	if entriesNode == nil || entriesNode.Kind != yaml.SequenceNode {
		return result, order
	}
	for _, item := range entriesNode.Content {
		entry, ok, err := parseAPIKeyEntry(item)
		if err != nil || !ok || entry.APIKey == "" {
			continue
		}
		result[entry.APIKey] = entry
		order = append(order, entry.APIKey)
	}
	return result, order
}

func mergeAPIKeyEntries(existing map[string]config.APIKeyEntry, existingOrder []string, incoming map[string]config.APIKeyEntry, incomingOrder []string) []config.APIKeyEntry {
	final := make([]config.APIKeyEntry, 0, len(existing)+len(incoming))
	used := make(map[string]struct{}, len(existing)+len(incoming))

	for _, key := range existingOrder {
		entry, ok := existing[key]
		if !ok {
			continue
		}
		if incomingEntry, exists := incoming[key]; exists {
			entry = incomingEntry
		}
		final = append(final, entry)
		used[key] = struct{}{}
	}

	for _, key := range incomingOrder {
		if _, exists := used[key]; exists {
			continue
		}
		entry, ok := incoming[key]
		if !ok {
			continue
		}
		final = append(final, entry)
		used[key] = struct{}{}
	}

	for key, entry := range incoming {
		if _, exists := used[key]; exists {
			continue
		}
		final = append(final, entry)
	}

	return final
}

func setAPIKeyEntries(root *yaml.Node, entries []config.APIKeyEntry) {
	idx := findMapKeyIndex(root, "api-key-entries")
	var seqNode *yaml.Node
	if idx < 0 {
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "api-key-entries"})
		seqNode = &yaml.Node{Kind: yaml.SequenceNode}
		root.Content = append(root.Content, seqNode)
	} else {
		seqNode = root.Content[idx+1]
		if seqNode == nil {
			seqNode = &yaml.Node{Kind: yaml.SequenceNode}
			root.Content[idx+1] = seqNode
		}
		seqNode.Kind = yaml.SequenceNode
		seqNode.Content = nil
	}

	for _, entry := range entries {
		if strings.TrimSpace(entry.APIKey) == "" {
			continue
		}
		item := &yaml.Node{Kind: yaml.MappingNode}
		item.Content = append(item.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "api-key"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: entry.APIKey},
		)
		if entry.DailyTokenLimit > 0 {
			item.Content = append(item.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "daily-token-limit"},
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(entry.DailyTokenLimit, 10)},
			)
		}
		if strings.TrimSpace(entry.ExpiresAt) != "" {
			item.Content = append(item.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "expires-at"},
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: strings.TrimSpace(entry.ExpiresAt)},
			)
		}
		seqNode.Content = append(seqNode.Content, item)
	}
}
