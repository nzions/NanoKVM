package picoclaw

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// F6: picoclaw may only drive HID. The tool set it is offered through the
// NanoKVM MCP server is exactly the screenshot and the HID actions.
func TestMCPToolsAreHIDOnly(t *testing.T) {
	var names []string
	for _, tool := range mcpToolDefinitions {
		name, _ := tool["name"].(string)
		names = append(names, name)
	}
	slices.Sort(names)
	if want := []string{"kvm_actions", "kvm_screenshot"}; !slices.Equal(names, want) {
		t.Fatalf("mcp tools = %v, want %v", names, want)
	}
}

func TestStartupDefaultsLockDownTools(t *testing.T) {
	editor := &picoclawConfigEditor{raw: map[string]any{
		"tools": map[string]any{
			"exec": map[string]any{"enabled": true, "allow_remote": true},
			"cron": map[string]any{"allow_command": true},
		},
		"agents": map[string]any{"defaults": map[string]any{"restrict_to_workspace": false}},
	}}
	for _, entry := range picoclawNanoKVMDefaults {
		editor.setValue(entry.value, entry.path...)
	}

	tools, _ := editor.raw["tools"].(map[string]any)
	for _, tool := range []string{"exec", "read_file", "write_file", "edit_file", "append_file", "list_dir", "web", "web_fetch", "cron", "spawn", "subagent"} {
		entry, _ := tools[tool].(map[string]any)
		if enabled, ok := entry["enabled"].(bool); !ok || enabled {
			t.Errorf("tools.%s.enabled = %v, want false", tool, entry["enabled"])
		}
	}
	if exec, _ := tools["exec"].(map[string]any); exec["allow_remote"] != false {
		t.Errorf("tools.exec.allow_remote = %v, want false", exec["allow_remote"])
	}
	if cron, _ := tools["cron"].(map[string]any); cron["allow_command"] != false {
		t.Errorf("tools.cron.allow_command = %v, want false", cron["allow_command"])
	}
	agents, _ := editor.raw["agents"].(map[string]any)
	defaults, _ := agents["defaults"].(map[string]any)
	if defaults["restrict_to_workspace"] != true || defaults["allow_read_outside_workspace"] != false {
		t.Errorf("workspace restriction not enforced: %v", defaults)
	}
	if mcp, _ := tools["mcp"].(map[string]any); mcp["enabled"] != true {
		t.Errorf("tools.mcp.enabled = %v, want true (the HID path)", mcp["enabled"])
	}
}

func TestPruneMCPServersKeepsOnlyNanoKVM(t *testing.T) {
	editor := &picoclawConfigEditor{raw: map[string]any{
		"tools": map[string]any{"mcp": map[string]any{"servers": map[string]any{
			"nanokvm":    map[string]any{"enabled": true},
			"filesystem": map[string]any{"enabled": true},
			"shell":      map[string]any{"enabled": true},
		}}},
	}}
	editor.pruneMCPServers(picoclawAllowedMCPServers)

	servers := editor.ensureObject("tools", "mcp", "servers")
	if len(servers) != 1 {
		t.Fatalf("servers = %v, want only nanokvm", servers)
	}
	if _, ok := servers["nanokvm"]; !ok {
		t.Fatalf("nanokvm server was pruned: %v", servers)
	}
	if !editor.changed {
		t.Fatal("editor did not record the prune")
	}
}

func TestMigrateLegacyPicoclawHome(t *testing.T) {
	legacy := t.TempDir()
	home := filepath.Join(t.TempDir(), "picoclaw")
	if err := os.MkdirAll(filepath.Join(legacy, "workspace"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"config.json":        `{"a":1}`,
		".security.yml":      "k: v",
		"workspace/AGENT.md": "x",
	} {
		if err := os.WriteFile(filepath.Join(legacy, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	migrateLegacyPicoclawHome(legacy, home)

	for _, name := range []string{"config.json", ".security.yml", "workspace/AGENT.md"} {
		info, err := os.Stat(filepath.Join(home, name))
		if err != nil {
			t.Fatalf("%s not migrated: %v", name, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %v, want 0600", name, info.Mode().Perm())
		}
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("legacy home still present: %v", err)
	}

	// A second run must not overwrite the migrated config.
	if err := os.WriteFile(filepath.Join(home, "config.json"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	migrateLegacyPicoclawHome(legacy, home)
	if data, _ := os.ReadFile(filepath.Join(home, "config.json")); string(data) != "new" {
		t.Errorf("config.json overwritten: %s", data)
	}
}
