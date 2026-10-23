package testdb

import "strings"

func AgentUsername(id string) string { return "agent_" + strings.ReplaceAll(id, "-", "")[:20] }
