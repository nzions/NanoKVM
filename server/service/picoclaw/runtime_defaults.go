package picoclaw

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"reflect"
	"slices"
	"strings"

	serverConfig "NanoKVM-Server/config"
)

const (
	currentPicoclawConfigVersion = 3
	defaultPicoclawPingSec       = 30
	defaultPicoclawReadSec       = 60
	defaultPicoclawWriteSec      = 10
	defaultPicoclawMaxConns      = 100
)

type picoclawConfigDefault struct {
	path  []string
	value any
}

// picoclawDisabledTools are the picoclaw built-in tools switched off
// (tools.<name>.enabled=false) on every start: shell, file read/write, web,
// scheduler-with-commands, sub-agents and skill installers. picoclaw is locked
// down to driving the remote host's HID through the NanoKVM MCP server and
// nothing else (F6). The key names follow picoclaw's config schema, which
// lives in the closed binary and is not verifiable from this repo, so this is
// defence in depth: the real control is the server refusing every non-HID
// request that carries the internal token (middleware.LoopbackInternalTokenPermits).
var picoclawDisabledTools = []string{
	"exec", "cron",
	"read_file", "write_file", "edit_file", "append_file", "list_dir", "send_file",
	"web", "web_fetch",
	"spawn", "subagent",
	"install_skill", "find_skills",
	"i2c", "spi",
}

// picoclawAllowedMCPServers are the only MCP servers picoclaw may keep.
var picoclawAllowedMCPServers = []string{"nanokvm"}

var picoclawNanoKVMDefaults = append([]picoclawConfigDefault{
	{path: []string{"agents", "defaults", "restrict_to_workspace"}, value: true},
	{path: []string{"agents", "defaults", "allow_read_outside_workspace"}, value: false},
	{path: []string{"agents", "defaults", "tool_feedback", "enabled"}, value: true},
	{path: []string{"gateway", "host"}, value: defaultPicoclawGatewayHost},
	{path: []string{"gateway", "port"}, value: defaultPicoclawGatewayPort},
	{path: []string{"gateway", "hot_reload"}, value: false},
	{path: []string{"tools", "cron", "allow_command"}, value: false},
	{path: []string{"tools", "exec", "allow_remote"}, value: false},
	{path: []string{"tools", "exec", "enable_deny_patterns"}, value: true},
	{path: []string{"channel_list", "pico", "type"}, value: "pico"},
	{path: []string{"channel_list", "pico", "settings", "allow_token_query"}, value: false},
	{path: []string{"channel_list", "pico", "settings", "ping_interval"}, value: defaultPicoclawPingSec},
	{path: []string{"channel_list", "pico", "settings", "read_timeout"}, value: defaultPicoclawReadSec},
	{path: []string{"channel_list", "pico", "settings", "write_timeout"}, value: defaultPicoclawWriteSec},
	{path: []string{"channel_list", "pico", "settings", "max_connections"}, value: defaultPicoclawMaxConns},
	{path: []string{"tools", "mcp", "enabled"}, value: true},
}, disabledToolDefaults()...)

func disabledToolDefaults() []picoclawConfigDefault {
	defaults := make([]picoclawConfigDefault, 0, len(picoclawDisabledTools))
	for _, tool := range picoclawDisabledTools {
		defaults = append(defaults, picoclawConfigDefault{
			path:  []string{"tools", tool, "enabled"},
			value: false,
		})
	}
	return defaults
}

func ensurePicoclawStartupDefaults() error {
	doc, err := loadPicoclawConfigDocument()
	if err != nil {
		return err
	}

	editor := &picoclawConfigEditor{raw: doc.raw}
	forceEnablePicoclawPicoChannel(doc, editor)
	if err := applyPicoclawStartupDefaults(editor); err != nil {
		return err
	}

	tokenChanged, err := ensurePicoclawPicoToken(doc)
	if err != nil {
		return err
	}

	if tokenChanged {
		if err := doc.saveSecurity(); err != nil {
			return err
		}
	}
	if editor.changed {
		if err := doc.saveConfig(); err != nil {
			return err
		}
	}

	return nil
}

func ensurePicoclawPicoChannelEnabled(doc *picoclawConfigDocument) error {
	if doc == nil {
		return nil
	}
	if pico, ok := doc.config.Channels["pico"]; ok && pico.Enabled {
		return nil
	}

	editor := &picoclawConfigEditor{raw: doc.raw}
	forceEnablePicoclawPicoChannel(doc, editor)
	if !editor.changed {
		return nil
	}

	return doc.saveConfig()
}

func applyPicoclawStartupDefaults(editor *picoclawConfigEditor) error {
	editor.setValue(currentPicoclawConfigVersion, "version")
	for _, entry := range picoclawNanoKVMDefaults {
		editor.setValue(entry.value, entry.path...)
	}
	server, err := defaultPicoclawMCPServer()
	if err != nil {
		return err
	}
	editor.setMCPServer("nanokvm", server)
	editor.pruneMCPServers(picoclawAllowedMCPServers)
	return nil
}

func forceEnablePicoclawPicoChannel(doc *picoclawConfigDocument, editor *picoclawConfigEditor) {
	if doc == nil || editor == nil {
		return
	}
	if pico, ok := doc.config.Channels["pico"]; ok && pico.Enabled {
		return
	}

	editor.setValue(true, "channel_list", "pico", "enabled")
	if doc.config.Channels == nil {
		doc.config.Channels = make(map[string]picoclawChannelEntry)
	}
	pico := doc.config.Channels["pico"]
	pico.Enabled = true
	doc.config.Channels["pico"] = pico
}

func defaultPicoclawMCPServer() (map[string]any, error) {
	internalToken, err := serverConfig.GetPicoclawInternalToken()
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"enabled": true,
		"type":    "http",
		"url":     nanoKVMMCPURL(),
		"headers": map[string]any{
			serverConfig.PicoclawInternalTokenHeader: internalToken,
		},
	}, nil
}

func nanoKVMMCPURL() string {
	port := serverConfig.GetInstance().Port.Http
	if port <= 0 {
		port = 80
	}

	return fmt.Sprintf("http://%s:%d/api/picoclaw/mcp", defaultPicoclawGatewayHost, port)
}

type picoclawConfigEditor struct {
	raw     map[string]any
	changed bool
}

func (e *picoclawConfigEditor) setValue(value any, path ...string) {
	if len(path) == 0 {
		return
	}

	target := e.ensureObject(path[:len(path)-1]...)
	key := path[len(path)-1]
	if current, exists := target[key]; exists && reflect.DeepEqual(current, value) {
		return
	}

	target[key] = value
	e.changed = true
}

func (e *picoclawConfigEditor) ensureObject(path ...string) map[string]any {
	current := e.raw
	for _, key := range path {
		next, ok := current[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[key] = next
			e.changed = true
		}
		current = next
	}
	return current
}

func (e *picoclawConfigEditor) setMCPServer(name string, fields map[string]any) {
	servers := e.ensureObject("tools", "mcp", "servers")
	entry, ok := servers[name].(map[string]any)
	if !ok {
		entry = map[string]any{}
		servers[name] = entry
		e.changed = true
	}

	for key, value := range fields {
		if current, exists := entry[key]; exists && reflect.DeepEqual(current, value) {
			continue
		}
		entry[key] = value
		e.changed = true
	}
}

// pruneMCPServers drops every MCP server not in keep: an operator-added MCP
// server could hand the agent a shell or a filesystem.
func (e *picoclawConfigEditor) pruneMCPServers(keep []string) {
	servers := e.ensureObject("tools", "mcp", "servers")
	for name := range servers {
		if !slices.Contains(keep, name) {
			delete(servers, name)
			e.changed = true
		}
	}
}

func generatePicoclawToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate picoclaw pico token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func ensurePicoclawPicoToken(doc *picoclawConfigDocument) (bool, error) {
	token, err := picoSecurityToken(doc)
	if err != nil {
		return false, err
	}

	if doc.security.ChannelList == nil {
		doc.security.ChannelList = make(map[string]picoclawChannelSecurityEntry)
	}
	entry := doc.security.ChannelList["pico"]
	if entry.Settings == nil {
		entry.Settings = &picoclawChannelSecuritySettings{}
	}
	if strings.TrimSpace(entry.Settings.Token) == token {
		return false, nil
	}

	entry.Settings.Token = token
	doc.security.ChannelList["pico"] = entry
	return true, nil
}

func picoSecurityToken(doc *picoclawConfigDocument) (string, error) {
	if doc == nil {
		return "", nil
	}

	// Check security channel_list.pico.settings.token
	if entry, ok := doc.security.ChannelList["pico"]; ok {
		if entry.Settings != nil {
			if token := strings.TrimSpace(entry.Settings.Token); token != "" {
				return token, nil
			}
		}
		if token := strings.TrimSpace(entry.Token); token != "" {
			return token, nil
		}
	}

	// Fall back to config channel_list.pico.settings.token
	if pico, ok := doc.config.Channels["pico"]; ok {
		if token := strings.TrimSpace(pico.Settings.Token); token != "" {
			return token, nil
		}
	}

	return generatePicoclawToken()
}
