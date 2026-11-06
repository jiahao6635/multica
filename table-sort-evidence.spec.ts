import { expect, test } from "@playwright/test";

// Production shared UI with synthetic HTTP/WS boundaries; no backend or account.
const wsId = "11111111-1111-4111-8111-111111111111";
const userId = "22222222-2222-4222-8222-222222222222";
const agentId = "33333333-3333-4333-8333-333333333333";
const sessionId = "44444444-4444-4444-8444-444444444444";
const taskId = "77777777-7777-4777-8777-777777777777";
const timestamp = "2026-10-20T00:00:00Z";
const session = {
  id: sessionId, workspace_id: wsId, agent_id: agentId, creator_id: userId,
  title: "Chat lifetime", status: "active", has_unread: false, unread_count: 0,
  last_message: null, pinned: false, created_at: timestamp, updated_at: timestamp,
};
const agent = {
  id: agentId, workspace_id: wsId, runtime_id: "55555555-5555-4555-8555-555555555555",
  name: "Fixture Agent", description: "", instructions: "", avatar_url: null,
  runtime_mode: "local", runtime_config: {}, custom_args: [], visibility: "workspace",
  permission_mode: "public_to", invocation_targets: [{ target_type: "workspace", target_id: null }],
  status: "idle", max_concurrent_tasks: 1, model: "", owner_id: userId, skills: [],
  runtime_availability: "online", created_at: timestamp, updated_at: timestamp,
  archived_at: null, archived_by: null,
};

test.use({ viewport: { width: 1280, height: 800 }, locale: "en-US", serviceWorkers: "block", launchOptions: { executablePath: "/usr/bin/chromium", args: ["--no-sandbox"] } });

test("long-integer sorting in the running application", async ({ page, context, baseURL }) => {
  const origin = new URL(baseURL ?? "http://localhost:3000").origin;
  const unexpected: string[] = [];
  const errors: string[] = [];
  page.on("pageerror", error => { errors.push(error.message); console.log("pageerror", error.message); });
  page.on("console", message => {
    if (message.text().includes("API response failed schema validation")) errors.push(message.text());
  });
  await page.emulateMedia({ reducedMotion: "no-preference" });
  await context.addCookies([
    { name: "multica_logged_in", value: "1", url: origin },
    { name: "last_workspace_slug", value: "perf", url: origin },
  ]);
  await context.addInitScript(({ sessionId, agentId }) => {
    localStorage.setItem("multica:chat:isOpen", "true");
    localStorage.setItem("multica:chat:floatingChatEnabled", "true");
    localStorage.setItem("multica:chat:activeSessionId:perf", sessionId);
    localStorage.setItem("multica:chat:selectedAgentId:perf", agentId);
  }, { sessionId, agentId });
  let latestContent = "| Value |\n| --- |\n| 9007199254740993 |\n| 9007199254740992 |\n| -9007199254740992 |\n| -9007199254740993 |";
  const taskMessages = [{ task_id: taskId, seq: 0, type: "text", content: "Streaming fixture. " }];
  const responseFor = (path: string): unknown => {
    switch (path) {
      case "/api/config": return { feature_flags: {}, allow_signup: true };
      case "/api/me": return {
        id: userId, name: "Fixture User", email: "fixture@example.test", avatar_url: null,
        language: "en", onboarded_at: timestamp, onboarding_questionnaire: { source: "other" },
        created_at: timestamp, updated_at: timestamp,
      };
      case "/api/workspaces": return [{
        id: wsId, name: "Perf", slug: "perf", description: null, context: null,
        settings: { github_enabled: false }, repos: [], issue_prefix: "PERF", avatar_url: null,
        created_at: timestamp, updated_at: timestamp,
      }];
      case "/api/agents": return [agent];
      case "/api/chat/sessions": return [session];
      case `/api/chat/sessions/${sessionId}`: return session;
      case `/api/chat/sessions/${sessionId}/messages/page`: return {
        messages: Array.from({ length: 1 }, (_, i) => ({
          id: `message-${i}`, chat_session_id: sessionId, role: "user", task_id: null,
          content: i === 0 ? latestContent : `Fixture message ${i}. `.repeat(1 + i % 5),
          created_at: timestamp,
        })), limit: 50, has_more: false,
      };
      case `/api/chat/sessions/${sessionId}/pending-task`: return {
        task_id: taskId, status: "running", created_at: timestamp, supports_queue: true, queued_tasks: [],
      };
      case `/api/tasks/${taskId}/messages`: return taskMessages;
      case `/api/chat/sessions/${sessionId}/read`: return {};
      case `/api/chat/sessions/${sessionId}/draft-restores`: return { restores: [] };
      case "/api/chat/pending-tasks/has-any": return { has_pending: true };
      case "/api/projects": return { projects: [], total: 0 };
      case "/api/issues": return { issues: [], total: 0 };
      case "/api/issue-statuses": return { statuses: [] };
      case "/api/properties": return { properties: [], total: 0 };
      case "/api/quick-actions": return { quick_actions: [] };
      case "/api/client-usage": return {};
      case "/api/issue-view-preferences": return { scope_type: "workspace", scope_id: null,
        ordered_view_ids: [], hidden_view_ids: [], default_view_id: null };
      case "/api/issue-workflows": return { workflows: [] };
      case "/api/search-index/manifest": return {
        cursor: "0", issue_count: 0, comment_count: 0, project_count: 0, text_bytes: 0,
      };
      case "/api/search-index/snapshot": return {
        issues: [], comments: [], projects: [], next_after_number: 0, done: true,
      };
      case "/api/search-index/changes": return {
        issues: [], comments: [], projects: [], deleted: { issues: [], comments: [], projects: [] },
        cursor: "0", has_more: false,
      };
      case `/api/workspaces/${wsId}/members`: return [{
        id: "member-1", user_id: userId, workspace_id: wsId, role: "owner",
        name: "Fixture User", email: "fixture@example.test", avatar_url: null,
      }];
      case "/api/autopilots": return { autopilots: [], total: 0 };
      case "/api/skills":
      case "/api/runtimes": case "/api/squads": case "/api/agent-task-snapshot":
      case "/api/invitations": case "/api/inbox": case "/api/inbox/unread-summary":
      case "/api/pins": case "/api/chat/pending-tasks": case "/api/assignee-frequency":
      case "/api/issue-views": case "/api/issue-wakeup-summaries":
      case "/api/agent-run-counts": case "/api/agent-activity-30d":
        return [];
      default: return undefined;
    }
  };
  await context.route("**/*", async route => {
    const url = new URL(route.request().url());
    if (!url.pathname.startsWith("/api/")) return url.origin === origin ? route.continue() : route.abort();
    if (route.request().method() === "OPTIONS") return route.fulfill({ status: 204, headers: { "access-control-allow-origin": origin, "access-control-allow-credentials": "true", "access-control-allow-headers": "*" } });
    console.log("fixture", url.pathname);
    const body = responseFor(url.pathname);
    if (body === undefined) unexpected.push(url.pathname);
    await route.fulfill({ status: body === undefined ? 404 : 200,
      headers: { "access-control-allow-origin": origin, "access-control-allow-credentials": "true" }, contentType: "application/json", body: JSON.stringify(body ?? {}) });
  });
  await context.routeWebSocket(url => url.pathname === "/ws", socket => {
    socket.onMessage(() => {});
  });

  await page.goto(`${origin}/perf/agents`, { waitUntil: "domcontentloaded" });

  await page.getByRole("button", { name: "Expand table", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  const values = () => dialog.locator("tbody tr").evaluateAll(rows => rows.map(row => row.querySelectorAll("td")[1]?.textContent));
  await dialog.getByRole("button", { name: "Value", exact: true }).click();
  const ascending = await values();
  await page.screenshot({path: `/workspace/table-sort-evidence/${process.env.SORT_STAGE}-ascending.png`});
  await dialog.getByRole("button", { name: "Value", exact: true }).click();
  const descending = await values();
  await page.screenshot({path: `/workspace/table-sort-evidence/${process.env.SORT_STAGE}-descending.png`});
  console.log(JSON.stringify({ascending, descending, errors, unexpected}));
  expect(errors).toEqual([]);
  expect(unexpected).toEqual([]);
  if (process.env.SORT_STAGE === "after") {
    expect(ascending).toEqual(["-9007199254740993", "-9007199254740992", "9007199254740992", "9007199254740993"]);
    expect(descending).toEqual(["9007199254740993", "9007199254740992", "-9007199254740992", "-9007199254740993"]);
  } else {
    expect(ascending).toEqual(["-9007199254740992", "-9007199254740993", "9007199254740993", "9007199254740992"]);
  }
});
