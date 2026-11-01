package daemon

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHandleTaskShutdownTerminalReport(t *testing.T) {
	for _, tc := range []struct {
		name         string
		shutdown     bool
		result       TaskResult
		wantEndpoint string
		wantReason   string
		wantError    string
	}{
		{"shutdown cancellation", true, TaskResult{Status: "cancelled"}, "/fail", "runtime_offline", "daemon shut down while task was in flight"},
		{"ordinary cancellation", false, TaskResult{Status: "cancelled"}, "/fail", "cancelled", ""},
		{"completion wins", true, TaskResult{Status: "completed", Comment: "done"}, "/complete", "", ""},
		{"cleanup uncertainty wins", true, TaskResult{Status: "blocked", Comment: "cleanup uncertain", FailureReason: "cleanup_unconfirmed"}, "/fail", "cleanup_unconfirmed", "cleanup uncertain"},
		{"real failure wins", true, TaskResult{Status: "blocked", Comment: "access denied", FailureReason: "agent_error.provider_auth_or_access"}, "/fail", "agent_error.provider_auth_or_access", "access denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			type report struct {
				endpoint string
				payload  map[string]any
			}
			reports := make(chan report, 2)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/fail") || strings.HasSuffix(r.URL.Path, "/complete") {
					var payload map[string]any
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Error(err)
					}
					reports <- report{r.URL.Path, payload}
				} else if strings.HasSuffix(r.URL.Path, "/status") {
					_, _ = io.WriteString(w, `{"status":"running"}`)
				}
			}))
			defer srv.Close()
			d := &Daemon{
				client: NewClient(srv.URL), logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
				runtimeIndex:       map[string]Runtime{"rt": {ID: "rt", Provider: "claude"}},
				cancelPollInterval: time.Hour,
			}
			d.runner = taskRunnerFunc(func(runCtx context.Context, _ Task, _ string, _ int, _ *slog.Logger) (TaskResult, error) {
				if tc.shutdown {
					cancel()
					<-runCtx.Done()
				}
				result := tc.result
				result.SessionID, result.WorkDir, result.BranchName = "session", "/work/task", "work-branch"
				return result, nil
			})
			d.handleTask(ctx, Task{ID: "task", RuntimeID: "rt"}, 0)
			select {
			case got := <-reports:
				if !strings.HasSuffix(got.endpoint, tc.wantEndpoint) {
					t.Errorf("endpoint = %s, want %s", got.endpoint, tc.wantEndpoint)
				}
				for key, want := range map[string]string{"failure_reason": tc.wantReason, "error": tc.wantError} {
					value, _ := got.payload[key].(string)
					if value != want {
						t.Errorf("%s = %q, want %q", key, value, want)
					}
				}
				for key, want := range map[string]string{"session_id": "session", "work_dir": "/work/task", "branch_name": "work-branch"} {
					if got.payload[key] != want {
						t.Errorf("%s = %v, want %s", key, got.payload[key], want)
					}
				}
			default:
				t.Fatal("no terminal callback delivered")
			}
			if len(reports) != 0 {
				t.Fatal("duplicate terminal report")
			}
		})
	}
}

func TestHandleTaskServerCancellationWinsOverShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var terminalCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/status"):
			_, _ = io.WriteString(w, `{"status":"cancelled"}`)
		case strings.HasSuffix(r.URL.Path, "/fail"), strings.HasSuffix(r.URL.Path, "/complete"):
			terminalCalls.Add(1)
		}
	}))
	defer srv.Close()
	d := &Daemon{
		client: NewClient(srv.URL), logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		runtimeIndex:       map[string]Runtime{"rt": {ID: "rt", Provider: "claude"}},
		cancelPollInterval: time.Millisecond,
	}
	d.runner = taskRunnerFunc(func(runCtx context.Context, _ Task, _ string, _ int, _ *slog.Logger) (TaskResult, error) {
		<-runCtx.Done()
		if ctx.Err() != nil {
			t.Fatal("server cancellation did not interrupt the runner")
		}
		cancel()
		return TaskResult{Status: "cancelled"}, nil
	})
	d.handleTask(ctx, Task{ID: "task", RuntimeID: "rt"}, 0)
	if got := terminalCalls.Load(); got != 0 {
		t.Fatalf("server-cancelled task received %d terminal callbacks during shutdown", got)
	}
}
