package collector

import (
	"encoding/json"
	"os"

	"github.com/fkasasagi/ccfx/model"
)

type rawSettings struct {
	Permissions *rawPermissions        `json:"permissions"`
	Hooks       map[string]json.RawMessage `json:"hooks"`
}

type rawPermissions struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

type rawHookGroup struct {
	Matcher string    `json:"matcher"`
	Hooks   []rawHook `json:"hooks"`
}

type rawHook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

func parseSettings(path string) (*model.SettingsData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw rawSettings
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	result := &model.SettingsData{}

	if raw.Permissions != nil {
		result.AllowRules = raw.Permissions.Allow
		result.DenyRules = raw.Permissions.Deny
	}

	for event, hookRaw := range raw.Hooks {
		var groups []rawHookGroup
		if err := json.Unmarshal(hookRaw, &groups); err != nil {
			continue
		}
		for _, g := range groups {
			for _, h := range g.Hooks {
				result.Hooks = append(result.Hooks, model.HookEntry{
					Event:   event,
					Matcher: g.Matcher,
					Command: h.Command,
				})
			}
		}
	}

	return result, nil
}
