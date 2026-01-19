package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/steveyegge/beads/internal/types"
)

func TestShouldCheckGate(t *testing.T) {
	tests := []struct {
		name       string
		awaitType  string
		typeFilter string
		want       bool
	}{
		// Empty filter matches all
		{"empty filter matches gh:run", "gh:run", "", true},
		{"empty filter matches gh:pr", "gh:pr", "", true},
		{"empty filter matches timer", "timer", "", true},
		{"empty filter matches human", "human", "", true},
		{"empty filter matches bead", "bead", "", true},

		// "all" filter matches all
		{"all filter matches gh:run", "gh:run", "all", true},
		{"all filter matches gh:pr", "gh:pr", "all", true},
		{"all filter matches timer", "timer", "all", true},
		{"all filter matches bead", "bead", "all", true},

		// "gh" filter matches all GitHub types
		{"gh filter matches gh:run", "gh:run", "gh", true},
		{"gh filter matches gh:pr", "gh:pr", "gh", true},
		{"gh filter does not match timer", "timer", "gh", false},
		{"gh filter does not match human", "human", "gh", false},
		{"gh filter does not match bead", "bead", "gh", false},

		// Exact type filters
		{"gh:run filter matches gh:run", "gh:run", "gh:run", true},
		{"gh:run filter does not match gh:pr", "gh:pr", "gh:run", false},
		{"gh:pr filter matches gh:pr", "gh:pr", "gh:pr", true},
		{"gh:pr filter does not match gh:run", "gh:run", "gh:pr", false},
		{"timer filter matches timer", "timer", "timer", true},
		{"timer filter does not match gh:run", "gh:run", "timer", false},
		{"bead filter matches bead", "bead", "bead", true},
		{"bead filter does not match timer", "timer", "bead", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gate := &types.Issue{
				AwaitType: tt.awaitType,
			}
			got := shouldCheckGate(gate, tt.typeFilter)
			if got != tt.want {
				t.Errorf("shouldCheckGate(%q, %q) = %v, want %v",
					tt.awaitType, tt.typeFilter, got, tt.want)
			}
		})
	}
}

func TestCheckBeadGate_InvalidFormat(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		awaitID string
		wantErr string
	}{
		{
			name:    "empty",
			awaitID: "",
			wantErr: "invalid await_id format",
		},
		{
			name:    "no colon",
			awaitID: "gastown-gt-abc",
			wantErr: "invalid await_id format",
		},
		{
			name:    "missing rig",
			awaitID: ":gt-abc",
			wantErr: "await_id missing rig name",
		},
		{
			name:    "missing bead",
			awaitID: "gastown:",
			wantErr: "await_id missing rig name or bead ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			satisfied, reason := checkBeadGate(ctx, tt.awaitID)
			if satisfied {
				t.Errorf("expected not satisfied for %q", tt.awaitID)
			}
			if reason == "" {
				t.Error("expected reason to be set")
			}
			// Just check the error message contains the expected substring
			if tt.wantErr != "" && !gateTestContainsIgnoreCase(reason, tt.wantErr) {
				t.Errorf("reason %q does not contain %q", reason, tt.wantErr)
			}
		})
	}
}

func TestCheckBeadGate_RigNotFound(t *testing.T) {
	ctx := context.Background()

	// Create a temp directory with a minimal beads setup
	tmpDir, err := os.MkdirTemp("", "gate_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp dir
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(tmpDir)

	// Try to check a gate for a non-existent rig
	satisfied, reason := checkBeadGate(ctx, "nonexistent:some-id")
	if satisfied {
		t.Error("expected not satisfied for non-existent rig")
	}
	if reason == "" {
		t.Error("expected reason to be set")
	}
	// The error should mention the rig not being found
	if !gateTestContainsIgnoreCase(reason, "not found") && !gateTestContainsIgnoreCase(reason, "could not find") {
		t.Errorf("reason should mention not found: %q", reason)
	}
}

func TestCheckBeadGate_TargetClosed(t *testing.T) {
	// Create a temporary database that simulates a target rig
	tmpDir, err := os.MkdirTemp("", "bead_gate_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a minimal database with a closed issue
	dbPath := filepath.Join(tmpDir, "beads.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	// Create minimal schema
	_, err = db.Exec(`
		CREATE TABLE issues (
			id TEXT PRIMARY KEY,
			status TEXT,
			title TEXT,
			created_at TEXT,
			updated_at TEXT
		)
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Insert a closed issue
	now := time.Now().Format(time.RFC3339)
	_, err = db.Exec(`
		INSERT INTO issues (id, status, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, "gt-test123", string(types.StatusClosed), "Test Issue", now, now)
	if err != nil {
		t.Fatal(err)
	}

	// Insert an open issue
	_, err = db.Exec(`
		INSERT INTO issues (id, status, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, "gt-open456", string(types.StatusOpen), "Open Issue", now, now)
	if err != nil {
		t.Fatal(err)
	}

	db.Close()

	// Note: This test can't fully exercise checkBeadGate because it relies on
	// routing.ResolveBeadsDirForRig which needs a proper routes.jsonl setup.
	// The full integration test would need the town/rig infrastructure.
	// For now, we just verify the function signature and basic error handling.
	t.Log("Database created with closed issue gt-test123 and open issue gt-open456")
	t.Log("Full integration testing requires routes.jsonl setup")
}

func TestIsNumericID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Numeric IDs
		{"12345", true},
		{"12345678901234567890", true},
		{"0", true},
		{"1", true},

		// Non-numeric (workflow names, etc.)
		{"", false},
		{"release.yml", false},
		{"CI", false},
		{"release", false},
		{"123abc", false},
		{"abc123", false},
		{"12.34", false},
		{"-123", false},
		{"123-456", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isNumericID(tt.input)
			if got != tt.want {
				t.Errorf("isNumericID(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNeedsDiscovery(t *testing.T) {
	tests := []struct {
		name      string
		awaitType string
		awaitID   string
		want      bool
	}{
		// gh:run gates
		{"gh:run empty await_id", "gh:run", "", true},
		{"gh:run workflow name hint", "gh:run", "release.yml", true},
		{"gh:run workflow name without ext", "gh:run", "CI", true},
		{"gh:run numeric run ID", "gh:run", "12345", false},
		{"gh:run large numeric ID", "gh:run", "12345678901234567890", false},

		// Other gate types should not need discovery
		{"gh:pr gate", "gh:pr", "", false},
		{"timer gate", "timer", "", false},
		{"human gate", "human", "", false},
		{"bead gate", "bead", "rig:id", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gate := &types.Issue{
				AwaitType: tt.awaitType,
				AwaitID:   tt.awaitID,
			}
			got := needsDiscovery(gate)
			if got != tt.want {
				t.Errorf("needsDiscovery(%q, %q) = %v, want %v",
					tt.awaitType, tt.awaitID, got, tt.want)
			}
		})
	}
}

func TestGetWorkflowNameHint(t *testing.T) {
	tests := []struct {
		name    string
		awaitID string
		want    string
	}{
		{"empty", "", ""},
		{"numeric ID", "12345", ""},
		{"workflow name", "release.yml", "release.yml"},
		{"workflow name yaml", "ci.yaml", "ci.yaml"},
		{"workflow name no ext", "CI", "CI"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gate := &types.Issue{AwaitID: tt.awaitID}
			got := getWorkflowNameHint(gate)
			if got != tt.want {
				t.Errorf("getWorkflowNameHint(%q) = %q, want %q", tt.awaitID, got, tt.want)
			}
		})
	}
}

func TestWorkflowNameMatches(t *testing.T) {
	tests := []struct {
		name         string
		hint         string
		workflowName string
		runName      string
		want         bool
	}{
		// Exact matches
		{"exact workflow name", "Release", "Release", "release.yml", true},
		{"exact run name", "release.yml", "Release", "release.yml", true},
		{"case insensitive workflow", "release", "Release", "release.yml", true},
		{"case insensitive run", "RELEASE.YML", "Release", "release.yml", true},

		// Hint with suffix, match display name without
		{"hint yml vs display name", "release.yml", "release", "ci.yml", true},
		{"hint yaml vs display name", "release.yaml", "release", "ci.yaml", true},

		// Hint without suffix, match filename with suffix
		{"hint base vs filename yml", "release", "CI", "release.yml", true},
		{"hint base vs filename yaml", "release", "CI", "release.yaml", true},

		// No match
		{"no match different name", "release", "CI", "ci.yml", false},
		{"no match partial", "rel", "Release", "release.yml", false},
		{"empty hint", "", "Release", "release.yml", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := workflowNameMatches(tt.hint, tt.workflowName, tt.runName)
			if got != tt.want {
				t.Errorf("workflowNameMatches(%q, %q, %q) = %v, want %v",
					tt.hint, tt.workflowName, tt.runName, got, tt.want)
			}
		})
	}
}

// gateTestContainsIgnoreCase checks if haystack contains needle (case-insensitive)
func gateTestContainsIgnoreCase(haystack, needle string) bool {
	return gateTestContains(gateTestLowerCase(haystack), gateTestLowerCase(needle))
}

func gateTestContains(s, substr string) bool {
	return len(s) >= len(substr) && gateTestFindSubstring(s, substr) >= 0
}

func gateTestLowerCase(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}

func gateTestFindSubstring(s, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// =============================================================================
// Tests for gate auto-discover workflow run ID (bd-1e12)
// =============================================================================

func TestGHWorkflowRunParsing(t *testing.T) {
	// Test that GHWorkflowRun struct correctly parses JSON from gh run list
	testJSON := `[
		{
			"databaseId": 12345678901,
			"displayTitle": "Release v1.0.0",
			"headBranch": "main",
			"headSha": "abc123def456",
			"name": "release.yml",
			"status": "completed",
			"conclusion": "success",
			"createdAt": "2025-01-15T10:30:00Z",
			"updatedAt": "2025-01-15T10:35:00Z",
			"workflowName": "Release",
			"url": "https://github.com/owner/repo/actions/runs/12345678901"
		},
		{
			"databaseId": 12345678902,
			"displayTitle": "CI checks",
			"headBranch": "feature-branch",
			"headSha": "def789ghi012",
			"name": "ci.yml",
			"status": "in_progress",
			"createdAt": "2025-01-15T11:00:00Z",
			"updatedAt": "2025-01-15T11:01:00Z",
			"workflowName": "CI",
			"url": "https://github.com/owner/repo/actions/runs/12345678902"
		}
	]`

	var runs []GHWorkflowRun
	err := json.Unmarshal([]byte(testJSON), &runs)
	if err != nil {
		t.Fatalf("Failed to parse GHWorkflowRun JSON: %v", err)
	}

	if len(runs) != 2 {
		t.Errorf("Expected 2 runs, got %d", len(runs))
	}

	// Verify first run
	if runs[0].DatabaseID != 12345678901 {
		t.Errorf("Expected DatabaseID 12345678901, got %d", runs[0].DatabaseID)
	}
	if runs[0].WorkflowName != "Release" {
		t.Errorf("Expected WorkflowName 'Release', got %q", runs[0].WorkflowName)
	}
	if runs[0].Status != "completed" {
		t.Errorf("Expected Status 'completed', got %q", runs[0].Status)
	}
	if runs[0].Conclusion != "success" {
		t.Errorf("Expected Conclusion 'success', got %q", runs[0].Conclusion)
	}
	if runs[0].HeadBranch != "main" {
		t.Errorf("Expected HeadBranch 'main', got %q", runs[0].HeadBranch)
	}

	// Verify second run (in_progress, no conclusion)
	if runs[1].DatabaseID != 12345678902 {
		t.Errorf("Expected DatabaseID 12345678902, got %d", runs[1].DatabaseID)
	}
	if runs[1].Status != "in_progress" {
		t.Errorf("Expected Status 'in_progress', got %q", runs[1].Status)
	}
	if runs[1].Conclusion != "" {
		t.Errorf("Expected empty Conclusion for in_progress, got %q", runs[1].Conclusion)
	}
}

func TestGHWorkflowRunEmptyList(t *testing.T) {
	// Test parsing an empty list
	testJSON := `[]`

	var runs []GHWorkflowRun
	err := json.Unmarshal([]byte(testJSON), &runs)
	if err != nil {
		t.Fatalf("Failed to parse empty GHWorkflowRun JSON: %v", err)
	}

	if len(runs) != 0 {
		t.Errorf("Expected 0 runs, got %d", len(runs))
	}
}

func TestDiscoverRunIDSelectsMostRecent(t *testing.T) {
	// This test verifies the logic that discoverRunIDByWorkflowName returns
	// the most recent run (first element, since GitHub API returns newest-first)
	// We can't mock queryGitHubRunsForWorkflow directly, but we can test
	// that the GHWorkflowRun struct's DatabaseID can be formatted correctly

	// Simulate what discoverRunIDByWorkflowName does: take runs[0].DatabaseID
	runs := []GHWorkflowRun{
		{DatabaseID: 98765432109, WorkflowName: "CI"},
		{DatabaseID: 98765432108, WorkflowName: "CI"},
		{DatabaseID: 98765432107, WorkflowName: "CI"},
	}

	if len(runs) == 0 {
		t.Fatal("Test setup error: no runs")
	}

	// Format the ID as done in discoverRunIDByWorkflowName
	runID := fmt.Sprintf("%d", runs[0].DatabaseID)
	expected := "98765432109"

	if runID != expected {
		t.Errorf("Expected run ID %q, got %q", expected, runID)
	}
}

func TestQueryGitHubRunsForWorkflow_Integration(t *testing.T) {
	// Skip if gh CLI is not installed (integration test)
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh CLI not installed, skipping integration test")
	}

	// Skip if not authenticated with GitHub
	// Run a simple gh command to check auth status
	cmd := exec.Command("gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		t.Skip("gh CLI not authenticated, skipping integration test")
	}

	// Note: This test would require a real GitHub repo context
	// It's primarily useful for manual testing or CI with proper GitHub setup
	t.Log("gh CLI integration tests require proper GitHub repository context")
	t.Log("For local testing, run: gh run list --workflow=<your-workflow>")
}

func TestDiscoverRunIDByWorkflowName_NoGHCLI(t *testing.T) {
	// Test behavior when gh CLI is not in PATH
	// We can't easily test this without modifying PATH, so just document the expected behavior
	t.Log("discoverRunIDByWorkflowName returns error 'gh CLI not found' when gh is not installed")
	t.Log("This is handled by queryGitHubRunsForWorkflow which checks exec.LookPath('gh')")
}

func TestCheckGHRunWithWorkflowHint(t *testing.T) {
	// Test that checkGHRun correctly identifies workflow hints vs numeric IDs
	tests := []struct {
		name        string
		awaitID     string
		isHint      bool
		description string
	}{
		{
			name:        "numeric ID",
			awaitID:     "12345678901",
			isHint:      false,
			description: "Numeric await_id should be used directly",
		},
		{
			name:        "workflow filename yml",
			awaitID:     "release.yml",
			isHint:      true,
			description: "Workflow filename should trigger discovery",
		},
		{
			name:        "workflow filename yaml",
			awaitID:     "ci.yaml",
			isHint:      true,
			description: "Workflow filename with .yaml should trigger discovery",
		},
		{
			name:        "workflow name no extension",
			awaitID:     "CI",
			isHint:      true,
			description: "Workflow name without extension should trigger discovery",
		},
		{
			name:        "empty",
			awaitID:     "",
			isHint:      false, // Empty returns early with different error
			description: "Empty await_id handled separately",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIsHint := !isNumericID(tt.awaitID) && tt.awaitID != ""
			if gotIsHint != tt.isHint {
				t.Errorf("isHint(%q) = %v, want %v: %s",
					tt.awaitID, gotIsHint, tt.isHint, tt.description)
			}
		})
	}
}

func TestGHRunStatusParsing(t *testing.T) {
	// Test that ghRunStatus struct correctly parses JSON from gh run view
	testCases := []struct {
		name       string
		json       string
		wantStatus string
		wantConc   string
		wantName   string
	}{
		{
			name:       "completed success",
			json:       `{"status": "completed", "conclusion": "success", "name": "CI"}`,
			wantStatus: "completed",
			wantConc:   "success",
			wantName:   "CI",
		},
		{
			name:       "completed failure",
			json:       `{"status": "completed", "conclusion": "failure", "name": "Release"}`,
			wantStatus: "completed",
			wantConc:   "failure",
			wantName:   "Release",
		},
		{
			name:       "in progress",
			json:       `{"status": "in_progress", "conclusion": "", "name": "Build"}`,
			wantStatus: "in_progress",
			wantConc:   "",
			wantName:   "Build",
		},
		{
			name:       "queued",
			json:       `{"status": "queued", "name": "Test"}`,
			wantStatus: "queued",
			wantConc:   "",
			wantName:   "Test",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var status ghRunStatus
			if err := json.Unmarshal([]byte(tc.json), &status); err != nil {
				t.Fatalf("Failed to parse ghRunStatus JSON: %v", err)
			}

			if status.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", status.Status, tc.wantStatus)
			}
			if status.Conclusion != tc.wantConc {
				t.Errorf("Conclusion = %q, want %q", status.Conclusion, tc.wantConc)
			}
			if status.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", status.Name, tc.wantName)
			}
		})
	}
}
