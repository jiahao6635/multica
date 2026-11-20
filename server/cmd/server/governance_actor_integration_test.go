package main

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestGovernanceRoutesRejectTaskToken exercises the Auth -> router boundary
// for account and workspace governance. Auth must overwrite the forged actor
// source, and every representative human-only scope must reject before its
// handler can mutate state or consume secret input.
func TestGovernanceRoutesRejectTaskToken(t *testing.T) {
	if testPool == nil {
		t.Skip("no database connection")
	}

	var agentID string
	if err := testPool.QueryRow(context.Background(), `
		SELECT id::text FROM agent WHERE workspace_id = $1 ORDER BY created_at LIMIT 1
	`, testWorkspaceID).Scan(&agentID); err != nil {
		t.Fatalf("load integration-test agent: %v", err)
	}
	taskID := ensureAgentTask(t, agentID)
	matToken := mintAgentTaskToken(t, agentID, taskID, testUserID)
	workspaceBase := "/api/workspaces/" + testWorkspaceID

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "profile", method: http.MethodPatch, path: "/api/me"},
		{name: "onboarding", method: http.MethodPost, path: "/api/me/onboarding/complete"},
		{name: "workspace creation", method: http.MethodPost, path: "/api/workspaces"},
		{name: "member listing", method: http.MethodGet, path: workspaceBase + "/members"},
		{name: "workspace leave", method: http.MethodPost, path: workspaceBase + "/leave"},
		{name: "workspace update", method: http.MethodPatch, path: workspaceBase},
		{name: "member invitation", method: http.MethodPost, path: workspaceBase + "/members"},
		{name: "mcp definition", method: http.MethodPost, path: workspaceBase + "/mcp-servers"},
		{name: "share link", method: http.MethodPost, path: workspaceBase + "/share-links"},
		{name: "runtime profile", method: http.MethodPost, path: workspaceBase + "/runtime-profiles"},
		{name: "plugin install", method: http.MethodPost, path: workspaceBase + "/plugins"},
		{name: "plugin config", method: http.MethodPut, path: workspaceBase + "/plugins/00000000-0000-0000-0000-000000000000/config"},
		{name: "workspace deletion", method: http.MethodDelete, path: workspaceBase},
		{name: "github integration", method: http.MethodGet, path: workspaceBase + "/github/connect"},
		{name: "lark integration", method: http.MethodPost, path: workspaceBase + "/lark/install/begin"},
		{name: "slack integration", method: http.MethodPost, path: workspaceBase + "/slack/install/byo"},
		{name: "dingtalk integration", method: http.MethodPost, path: workspaceBase + "/dingtalk/install/byo"},
		{name: "telegram integration", method: http.MethodPost, path: workspaceBase + "/telegram/install"},
		{name: "account integration", method: http.MethodPost, path: "/api/integrations/composio/connect/init"},
		{name: "invitation acceptance", method: http.MethodPost, path: "/api/invitations/00000000-0000-0000-0000-000000000000/accept"},
		{name: "share link join", method: http.MethodPost, path: "/api/share-links/join"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, testServer.URL+tt.path, strings.NewReader(`{}`))
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
			req.Header.Set("Authorization", "Bearer "+matToken)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Actor-Source", "member")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("perform request: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", resp.StatusCode)
			}
		})
	}
}
