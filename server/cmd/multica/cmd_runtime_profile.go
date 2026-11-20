package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/pkg/agent"
)

// `multica runtime profile ...` — custom runtime profiles (MUL-3284)
//
// A runtime profile lets a workspace declare a custom agent runtime built on
// top of a supported protocol family (the routing backend) but launched via a
// site-specific command_name (e.g. a wrapper that injects credentials). The
// profile lives server-side and is workspace-scoped; each daemon owner must
// approve the exact executable and fixed arguments before it can run locally.
//
// `set-path` / `unset-path` manage that per-machine approval. The selected path
// and current fixed_args digest stay in local CLI config and never leave the
// machine.

var runtimeProfileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage custom runtime profiles",
}

var runtimeProfileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List custom runtime profiles in the workspace",
	RunE:  runRuntimeProfileList,
}

var runtimeProfileCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a custom runtime profile",
	RunE:  runRuntimeProfileCreate,
}

var runtimeProfileUpdateCmd = &cobra.Command{
	Use:   "update <profile-id>",
	Short: "Update a custom runtime profile (protocol family is immutable)",
	Args:  exactArgs(1),
	RunE:  runRuntimeProfileUpdate,
}

var runtimeProfileDeleteCmd = &cobra.Command{
	Use:   "delete <profile-id>",
	Short: "Delete a custom runtime profile",
	Args:  exactArgs(1),
	RunE:  runRuntimeProfileDelete,
}

var runtimeProfileSetPathCmd = &cobra.Command{
	Use:   "set-path <profile-id>",
	Short: "Approve an executable and the current profile arguments on this machine",
	Args:  exactArgs(1),
	RunE:  runRuntimeProfileSetPath,
}

var runtimeProfileUnsetPathCmd = &cobra.Command{
	Use:   "unset-path <profile-id>",
	Short: "Revoke a runtime profile's approval on this machine",
	Args:  exactArgs(1),
	RunE:  runRuntimeProfileUnsetPath,
}

func init() {
	runtimeCmd.AddCommand(runtimeProfileCmd)
	runtimeProfileCmd.AddCommand(runtimeProfileListCmd)
	runtimeProfileCmd.AddCommand(runtimeProfileCreateCmd)
	runtimeProfileCmd.AddCommand(runtimeProfileUpdateCmd)
	runtimeProfileCmd.AddCommand(runtimeProfileDeleteCmd)
	runtimeProfileCmd.AddCommand(runtimeProfileSetPathCmd)
	runtimeProfileCmd.AddCommand(runtimeProfileUnsetPathCmd)

	// list
	runtimeProfileListCmd.Flags().String("output", "table", "Output format: table or json")

	// create
	runtimeProfileCreateCmd.Flags().String("runtime-type", "", "Base runtime compatibility target (e.g. pi or omp)")
	runtimeProfileCreateCmd.Flags().String("protocol-family", "", "Legacy base protocol family (use --runtime-type for Oh-My-Pi)")
	runtimeProfileCreateCmd.Flags().String("command-name", "", "Executable the daemon resolves on PATH (required)")
	runtimeProfileCreateCmd.Flags().String("display-name", "", "Human-readable profile name (required)")
	runtimeProfileCreateCmd.Flags().String("description", "", "Optional description")
	runtimeProfileCreateCmd.Flags().String("output", "json", "Output format: table or json")

	// update
	runtimeProfileUpdateCmd.Flags().String("display-name", "", "New display name")
	runtimeProfileUpdateCmd.Flags().String("command-name", "", "New command name")
	runtimeProfileUpdateCmd.Flags().String("description", "", "New description")
	// NOTE: --fixed-arg remains out of the CLI create/update surface for now:
	// the product path parses command + args in the UI and stores them as
	// command_name + fixed_args. Keep this CLI shape narrow until we add an
	// argv-aware command-line parser here too.
	runtimeProfileUpdateCmd.Flags().Bool("enabled", true, "Enable or disable the profile")
	runtimeProfileUpdateCmd.Flags().String("output", "json", "Output format: table or json")

	// set-path
	runtimeProfileSetPathCmd.Flags().String("path", "", "Absolute path to the executable on this machine (required)")
	runtimeProfileSetPathCmd.Flags().String("approve-args-sha256", "", "Approve the displayed fixed arguments by their SHA-256 digest (required)")
}

// runtimeProfilesPath builds the workspace-scoped collection path.
func runtimeProfilesPath(workspaceID string) string {
	return fmt.Sprintf("/api/workspaces/%s/runtime-profiles", workspaceID)
}

// validateProtocolFamily checks a protocol family against the canonical agent
// whitelist client-side so an obvious typo fails fast with a helpful list
// instead of an opaque server 400.
func validateProtocolFamily(family string) error {
	if !agent.IsSupportedType(family) {
		return fmt.Errorf("invalid --protocol-family %q: must be one of %s",
			family, strings.Join(agent.SupportedTypes, ", "))
	}
	return nil
}

// NOTE: a --visibility flag is intentionally NOT exposed in v1. The server
// forces every profile to 'workspace' because the read paths do not yet
// enforce 'private' (exposing it would leak "private" profiles). Re-add once
// creator-visibility filtering exists. Follow-up: MUL-3308.

func runRuntimeProfileList(cmd *cobra.Command, _ []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	workspaceID, err := requireWorkspaceID(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	var resp struct {
		RuntimeProfiles []map[string]any `json:"runtime_profiles"`
	}
	if err := client.GetJSON(ctx, runtimeProfilesPath(workspaceID), &resp); err != nil {
		return fmt.Errorf("list runtime profiles: %w", err)
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, resp.RuntimeProfiles)
	}
	printRuntimeProfileTable(resp.RuntimeProfiles)
	return nil
}

func runRuntimeProfileCreate(cmd *cobra.Command, _ []string) error {
	family, _ := cmd.Flags().GetString("protocol-family")
	commandName, _ := cmd.Flags().GetString("command-name")
	displayName, _ := cmd.Flags().GetString("display-name")
	description, _ := cmd.Flags().GetString("description")

	runtimeType, _ := cmd.Flags().GetString("runtime-type")
	runtimeType = strings.TrimSpace(runtimeType)
	if runtimeType == "" && strings.TrimSpace(family) == "" {
		return fmt.Errorf("--runtime-type or --protocol-family is required")
	}
	if runtimeType != "" {
		resolved, ok := agent.RuntimeProtocolFamily(runtimeType)
		if !ok {
			return fmt.Errorf("unsupported --runtime-type %q", runtimeType)
		}
		if family != "" && family != resolved {
			return fmt.Errorf("--protocol-family does not match --runtime-type")
		}
		family = resolved
	}
	if strings.TrimSpace(commandName) == "" {
		return fmt.Errorf("--command-name is required")
	}
	if strings.TrimSpace(displayName) == "" {
		return fmt.Errorf("--display-name is required")
	}
	if err := validateProtocolFamily(family); err != nil {
		return err
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	workspaceID, err := requireWorkspaceID(cmd)
	if err != nil {
		return err
	}

	body := map[string]any{
		"display_name":    displayName,
		"protocol_family": family,
		"command_name":    commandName,
	}
	if runtimeType != "" {
		body["runtime_type"] = runtimeType
		delete(body, "protocol_family")
	}
	if description != "" {
		body["description"] = description
	}

	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	var profile map[string]any
	if err := client.PostJSON(ctx, runtimeProfilesPath(workspaceID), body, &profile); err != nil {
		return fmt.Errorf("create runtime profile: %w", err)
	}
	return outputRuntimeProfile(cmd, profile)
}

func runRuntimeProfileUpdate(cmd *cobra.Command, args []string) error {
	profileID := args[0]

	body := map[string]any{}
	if cmd.Flags().Changed("display-name") {
		v, _ := cmd.Flags().GetString("display-name")
		body["display_name"] = v
	}
	if cmd.Flags().Changed("command-name") {
		v, _ := cmd.Flags().GetString("command-name")
		body["command_name"] = v
	}
	if cmd.Flags().Changed("description") {
		v, _ := cmd.Flags().GetString("description")
		body["description"] = v
	}
	if cmd.Flags().Changed("enabled") {
		v, _ := cmd.Flags().GetBool("enabled")
		body["enabled"] = v
	}

	if len(body) == 0 {
		return fmt.Errorf("no fields to update: pass at least one of --display-name, --command-name, --description, --enabled")
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	workspaceID, err := requireWorkspaceID(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	path := runtimeProfilesPath(workspaceID) + "/" + profileID
	var profile map[string]any
	if err := client.PatchJSON(ctx, path, body, &profile); err != nil {
		return fmt.Errorf("update runtime profile: %w", err)
	}
	return outputRuntimeProfile(cmd, profile)
}

func runRuntimeProfileDelete(cmd *cobra.Command, args []string) error {
	profileID := args[0]

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	workspaceID, err := requireWorkspaceID(cmd)
	if err != nil {
		return err
	}

	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()

	path := runtimeProfilesPath(workspaceID) + "/" + profileID
	if err := client.DeleteJSON(ctx, path); err != nil {
		// 409 means the server refused — usually because active agents are
		// still bound to this profile, on machines it names. Surface that
		// sentence rather than the raw response body: it already reads as
		// guidance, and it is what tells the user the blockers may be sitting
		// on a machine other than the one they were cleaning up.
		if _, msg, isConflict := serverConflictMessage(err); isConflict {
			return errors.New(msg)
		}
		var httpErr *cli.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusConflict {
			return fmt.Errorf("cannot delete runtime profile %s: profile still has active agents bound to it", profileID)
		}
		return fmt.Errorf("delete runtime profile: %w", err)
	}
	fmt.Printf("Deleted runtime profile %s\n", profileID)
	return nil
}

func runRuntimeProfileSetPath(cmd *cobra.Command, args []string) error {
	if err := requireHumanLocalCommand("runtime profile set-path"); err != nil {
		return err
	}
	profileID := args[0]
	path, _ := cmd.Flags().GetString("path")
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("--path is required")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("--path must be an absolute path, got %q", path)
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	workspaceID, err := requireWorkspaceID(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	var current struct {
		ID        string   `json:"id"`
		FixedArgs []string `json:"fixed_args"`
	}
	profilePath := runtimeProfilesPath(workspaceID) + "/" + profileID
	if err := client.GetJSON(ctx, profilePath, &current); err != nil {
		return fmt.Errorf("load runtime profile for local approval: %w", err)
	}
	if !strings.EqualFold(current.ID, profileID) {
		return fmt.Errorf("load runtime profile for local approval: response id %q does not match %q", current.ID, profileID)
	}
	profileID = current.ID
	fixedArgsSHA256 := cli.RuntimeProfileFixedArgsSHA256(current.FixedArgs)
	approvedSHA256, _ := cmd.Flags().GetString("approve-args-sha256")
	if strings.TrimSpace(approvedSHA256) == "" {
		encodedArgs, _ := json.Marshal(current.FixedArgs)
		return fmt.Errorf("review fixed_args %s and rerun with --approve-args-sha256 %s", encodedArgs, fixedArgsSHA256)
	}
	if approvedSHA256 != fixedArgsSHA256 {
		return fmt.Errorf("fixed_args changed or the approval digest is incorrect; review the profile and try again")
	}

	profile := resolveProfile(cmd)
	cfg, err := cli.LoadCLIConfigForProfile(profile)
	if err != nil {
		return fmt.Errorf("load CLI config: %w", err)
	}
	if cfg.RuntimeProfileApprovals == nil {
		cfg.RuntimeProfileApprovals = map[string]cli.RuntimeProfileApproval{}
	}
	cfg.RuntimeProfileApprovals[profileID] = cli.RuntimeProfileApproval{
		Path:            path,
		FixedArgsSHA256: fixedArgsSHA256,
	}
	if err := cli.SaveCLIConfigForProfile(cfg, profile); err != nil {
		return fmt.Errorf("save CLI config: %w", err)
	}
	fmt.Printf("Approved runtime profile %s at %s with its current fixed arguments.\n", profileID, path)
	fmt.Println("Restart the daemon for the change to take effect.")
	return nil
}

func runRuntimeProfileUnsetPath(cmd *cobra.Command, args []string) error {
	if err := requireHumanLocalCommand("runtime profile unset-path"); err != nil {
		return err
	}
	profileID := args[0]

	profile := resolveProfile(cmd)
	cfg, err := cli.LoadCLIConfigForProfile(profile)
	if err != nil {
		return fmt.Errorf("load CLI config: %w", err)
	}
	_, approved := cfg.RuntimeProfileApprovals[profileID]
	if !approved {
		fmt.Printf("No local approval set for runtime profile %s.\n", profileID)
		return nil
	}
	delete(cfg.RuntimeProfileApprovals, profileID)
	if len(cfg.RuntimeProfileApprovals) == 0 {
		cfg.RuntimeProfileApprovals = nil
	}
	if err := cli.SaveCLIConfigForProfile(cfg, profile); err != nil {
		return fmt.Errorf("save CLI config: %w", err)
	}
	fmt.Printf("Revoked the local approval for runtime profile %s.\n", profileID)
	fmt.Println("Restart the daemon for the change to take effect.")
	return nil
}

// outputRuntimeProfile renders a single profile honoring --output.
func outputRuntimeProfile(cmd *cobra.Command, profile map[string]any) error {
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, profile)
	}
	printRuntimeProfileTable([]map[string]any{profile})
	return nil
}

// printRuntimeProfileTable renders profiles as a stable, sorted table.
func printRuntimeProfileTable(profiles []map[string]any) {
	headers := []string{"ID", "DISPLAY_NAME", "PROTOCOL_FAMILY", "COMMAND_NAME", "ENABLED"}
	rows := make([][]string, 0, len(profiles))
	for _, p := range profiles {
		rows = append(rows, []string{
			strVal(p, "id"),
			strVal(p, "display_name"),
			strVal(p, "protocol_family"),
			strVal(p, "command_name"),
			strVal(p, "enabled"),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i][1] < rows[j][1] })
	cli.PrintTable(os.Stdout, headers, rows)
}
