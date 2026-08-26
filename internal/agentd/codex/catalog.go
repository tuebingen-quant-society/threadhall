package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
)

const maxCatalogCapabilities = 512

func discoverProtocol(ctx context.Context, transport readWriter, cwd string) ([]agenttask.Capability, error) {
	if strings.TrimSpace(cwd) == "" {
		return nil, errors.New("Codex cwd is required")
	}
	scanner := bufio.NewScanner(transport)
	scanner.Buffer(make([]byte, 64<<10), maxRPCFrameBytes)
	rpc := &rpcConnection{encoder: json.NewEncoder(transport), scanner: scanner}
	if _, err := rpc.request(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{"name": "threadhall-agentd", "title": "Threadhall", "version": "0.1"},
	}); err != nil {
		return nil, fmt.Errorf("initialize Codex catalog: %w", err)
	}
	if err := rpc.encoder.Encode(map[string]any{"method": "initialized"}); err != nil {
		return nil, fmt.Errorf("notify Codex initialization: %w", err)
	}
	pluginsRaw, err := rpc.request(ctx, "plugin/list", map[string]any{
		"cwds": []string{cwd}, "forceRefetch": false, "marketplaceKinds": []string{"local"},
	})
	if err != nil {
		return nil, fmt.Errorf("list Codex plugins: %w", err)
	}
	items, err := decodePlugins(pluginsRaw)
	if err != nil {
		return nil, err
	}
	skillsRaw, err := rpc.request(ctx, "skills/list", map[string]any{"cwds": []string{cwd}, "forceReload": false})
	if err != nil {
		return nil, fmt.Errorf("list Codex skills: %w", err)
	}
	return decodeSkills(skillsRaw, items)
}

func decodePlugins(raw json.RawMessage) ([]agenttask.Capability, error) {
	var response struct {
		Marketplaces []struct {
			Plugins []struct {
				ID, Name          string
				Installed, Enabled bool
				Interface          *struct {
					DisplayName, ShortDescription string
				} `json:"interface"`
			} `json:"plugins"`
		} `json:"marketplaces"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, errors.New("Codex plugin/list returned invalid data")
	}
	items := make([]agenttask.Capability, 0)
	seen := make(map[string]struct{})
	for _, marketplace := range response.Marketplaces {
		for _, plugin := range marketplace.Plugins {
			if !plugin.Installed || !plugin.Enabled || plugin.ID == "" {
				continue
			}
			name, description := plugin.Name, ""
			if plugin.Interface != nil {
				if plugin.Interface.DisplayName != "" { name = plugin.Interface.DisplayName }
				description = plugin.Interface.ShortDescription
			}
			key := "plugin\x00" + plugin.ID
			if _, exists := seen[key]; exists { continue }
			seen[key] = struct{}{}
			items = append(items, agenttask.Capability{Kind: "plugin", ID: plugin.ID, Name: name, Description: description})
			if len(items) == maxCatalogCapabilities { return items, nil }
		}
	}
	return items, nil
}

func decodeSkills(raw json.RawMessage, items []agenttask.Capability) ([]agenttask.Capability, error) {
	var response struct {
		Data []struct {
			Skills []struct {
				Name, Description string
				Enabled           bool
			} `json:"skills"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, errors.New("Codex skills/list returned invalid data")
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items { seen[item.Kind+"\x00"+item.ID] = struct{}{} }
	for _, entry := range response.Data {
		for _, skill := range entry.Skills {
			if !skill.Enabled || skill.Name == "" { continue }
			key := "skill\x00" + skill.Name
			if _, exists := seen[key]; exists { continue }
			seen[key] = struct{}{}
			items = append(items, agenttask.Capability{Kind: "skill", ID: skill.Name, Name: skill.Name, Description: skill.Description})
			if len(items) == maxCatalogCapabilities { return items, nil }
		}
	}
	return items, nil
}
