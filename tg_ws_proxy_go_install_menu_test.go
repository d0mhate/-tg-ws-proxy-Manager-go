package main

import (
	"os"
	"strings"
	"testing"
)

func TestManagerConfigureUpdateSourceViaAdvancedMenu(t *testing.T) {
	env := managerEnv(t)
	updateChannelPath := envValue(env, "PERSIST_UPDATE_CHANNEL_FILE")
	previewBranchPath := envValue(env, "PERSIST_PREVIEW_BRANCH_FILE")
	if updateChannelPath == "" || previewBranchPath == "" {
		t.Fatal("missing update source state files in env")
	}

	out, err := runManagerMenu(t, env, "4\n12\npreview\nfeature/auth-flow\n\n\n")
	if err != nil {
		t.Fatalf("configure update source failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Update source saved: preview feature/auth-flow") {
		t.Fatalf("expected update source saved message, got:\n%s", out)
	}
	if got := readTrimmed(t, updateChannelPath); got != "preview" {
		t.Fatalf("expected persisted update channel preview, got %q", got)
	}
	if got := readTrimmed(t, previewBranchPath); got != "feature/auth-flow" {
		t.Fatalf("expected persisted preview branch, got %q", got)
	}

	statusOut, err := runManager(t, env, "status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "src mode  : preview") || !strings.Contains(statusOut, "ref       : feature/auth-flow") {
		t.Fatalf("expected status to reflect preview update source, got:\n%s", statusOut)
	}
}

func TestManagerConfigureUpdateSourceViaAdvancedMenuSupportsArrowSelection(t *testing.T) {
	env := setEnvValue(managerEnv(t), "FORCE_ARROW_UPDATE_SOURCE_PICKER", "1")
	updateChannelPath := envValue(env, "PERSIST_UPDATE_CHANNEL_FILE")
	previewBranchPath := envValue(env, "PERSIST_PREVIEW_BRANCH_FILE")
	if updateChannelPath == "" || previewBranchPath == "" {
		t.Fatal("missing update source state files in env")
	}

	out, err := runManagerMenu(t, env, "4\n12\n\033[B\nfeature/auth-flow\n\n\n")
	if err != nil {
		t.Fatalf("configure update source with arrows failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Mode (use arrows, Enter to confirm):") {
		t.Fatalf("expected arrow selection prompt, got:\n%s", out)
	}
	if !strings.Contains(out, "Update source saved: preview feature/auth-flow") {
		t.Fatalf("expected preview update source saved message, got:\n%s", out)
	}
	if got := readTrimmed(t, updateChannelPath); got != "preview" {
		t.Fatalf("expected persisted update channel preview, got %q", got)
	}
	if got := readTrimmed(t, previewBranchPath); got != "feature/auth-flow" {
		t.Fatalf("expected persisted preview branch, got %q", got)
	}
}

func TestManagerConfigureUpdateSourceViaAdvancedMenuSupportsNumberedSelection(t *testing.T) {
	env := setEnvValue(managerEnv(t), "FORCE_NUMBERED_UPDATE_SOURCE_PICKER", "1")
	updateChannelPath := envValue(env, "PERSIST_UPDATE_CHANNEL_FILE")
	previewBranchPath := envValue(env, "PERSIST_PREVIEW_BRANCH_FILE")
	if updateChannelPath == "" || previewBranchPath == "" {
		t.Fatal("missing update source state files in env")
	}

	out, err := runManagerMenu(t, env, "4\n12\n2\nfeature/auth-flow\n\n\n")
	if err != nil {
		t.Fatalf("configure update source with numbered picker failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Select mode [1-2] (Enter for release):") {
		t.Fatalf("expected numbered selection prompt, got:\n%s", out)
	}
	if !strings.Contains(out, "Update source saved: preview feature/auth-flow") {
		t.Fatalf("expected preview update source saved message, got:\n%s", out)
	}
	if got := readTrimmed(t, updateChannelPath); got != "preview" {
		t.Fatalf("expected persisted update channel preview, got %q", got)
	}
	if got := readTrimmed(t, previewBranchPath); got != "feature/auth-flow" {
		t.Fatalf("expected persisted preview branch, got %q", got)
	}
}

func TestManagerConfigureUpdateSourcePreviewBranchPromptShowsExampleAndCanReuseSavedBranch(t *testing.T) {
	env := setEnvValue(managerEnv(t), "FORCE_NUMBERED_UPDATE_SOURCE_PICKER", "1")
	updateChannelPath := envValue(env, "PERSIST_UPDATE_CHANNEL_FILE")
	previewBranchPath := envValue(env, "PERSIST_PREVIEW_BRANCH_FILE")
	if updateChannelPath == "" || previewBranchPath == "" {
		t.Fatal("missing update source state files in env")
	}

	out, err := runManagerMenu(t, env, "4\n12\n2\nfeature/auth-flow\n\n\n")
	if err != nil {
		t.Fatalf("initial configure update source failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Preview branch name (for example: preview-channel):") {
		t.Fatalf("expected example preview branch prompt, got:\n%s", out)
	}

	out, err = runManagerMenu(t, env, "4\n12\n2\n\n\n")
	if err != nil {
		t.Fatalf("reusing saved preview branch failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Preview branch name (Enter to keep feature/auth-flow):") {
		t.Fatalf("expected keep-current preview branch prompt, got:\n%s", out)
	}
	if got := readTrimmed(t, updateChannelPath); got != "preview" {
		t.Fatalf("expected persisted update channel preview, got %q", got)
	}
	if got := readTrimmed(t, previewBranchPath); got != "feature/auth-flow" {
		t.Fatalf("expected persisted preview branch to stay unchanged, got %q", got)
	}
}

func TestManagerConfigureUpdateSourceCanResetToLatestRelease(t *testing.T) {
	env := managerEnv(t)
	updateChannelPath := envValue(env, "PERSIST_UPDATE_CHANNEL_FILE")
	previewBranchPath := envValue(env, "PERSIST_PREVIEW_BRANCH_FILE")
	releaseTagPath := envValue(env, "PERSIST_RELEASE_TAG_FILE")
	if updateChannelPath == "" || previewBranchPath == "" || releaseTagPath == "" {
		t.Fatal("missing update source state files in env")
	}

	writeFile(t, updateChannelPath, "preview\n", 0o644)
	writeFile(t, previewBranchPath, "feature/auth-flow\n", 0o644)
	writeFile(t, releaseTagPath, "v1.1.25\n", 0o644)

	out, err := runManagerMenu(t, env, "4\n12\nrelease\nlatest\n\n\n")
	if err != nil {
		t.Fatalf("reset update source failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Update source saved: latest release") {
		t.Fatalf("expected latest release saved message, got:\n%s", out)
	}
	if got := readTrimmed(t, updateChannelPath); got != "release" {
		t.Fatalf("expected persisted update channel release, got %q", got)
	}
	if _, err := os.Stat(previewBranchPath); !os.IsNotExist(err) {
		t.Fatalf("expected preview branch state to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(releaseTagPath); !os.IsNotExist(err) {
		t.Fatalf("expected release tag pin to be removed, stat err=%v", err)
	}
}

func TestManagerConfigureUpdateSourceCanSelectPinnedReleaseTagFromNumberedMenu(t *testing.T) {
	env := setEnvValue(managerEnv(t), "FORCE_NUMBERED_UPDATE_SOURCE_PICKER", "1")
	updateChannelPath := envValue(env, "PERSIST_UPDATE_CHANNEL_FILE")
	releaseTagPath := envValue(env, "PERSIST_RELEASE_TAG_FILE")
	if updateChannelPath == "" || releaseTagPath == "" {
		t.Fatal("missing release update source state files in env")
	}

	out, err := runManagerMenu(t, env, "4\n12\n1\n3\n\n\n")
	if err != nil {
		t.Fatalf("configure release tag from numbered menu failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Select mode [1-2] (Enter for release):") {
		t.Fatalf("expected numbered mode picker, got:\n%s", out)
	}
	if !strings.Contains(out, "Select release ref [1-") {
		t.Fatalf("expected numbered release ref picker, got:\n%s", out)
	}
	if strings.Contains(out, "v1.1.28") {
		t.Fatalf("expected release picker to hide tags below v1.1.29, got:\n%s", out)
	}
	if !strings.Contains(out, "Update source saved: release v1.1.29") {
		t.Fatalf("expected pinned release tag saved message, got:\n%s", out)
	}
	if got := readTrimmed(t, updateChannelPath); got != "release" {
		t.Fatalf("expected persisted update channel release, got %q", got)
	}
	if got := readTrimmed(t, releaseTagPath); got != "v1.1.29" {
		t.Fatalf("expected persisted release tag v1.1.29, got %q", got)
	}

	statusOut, err := runManager(t, env, "status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "src mode  : release") || !strings.Contains(statusOut, "ref       : v1.1.29") {
		t.Fatalf("expected status to reflect pinned release tag, got:\n%s", statusOut)
	}

	menuOut, err := runManagerMenu(t, env, "\n")
	if err != nil {
		t.Fatalf("menu failed: %v\n%s", err, menuOut)
	}
	if !strings.Contains(menuOut, "track: release/v1.1.29") {
		t.Fatalf("expected main menu track to show pinned release tag, got:\n%s", menuOut)
	}
}

func TestManagerConfigureUpdateSourceRejectsManualReleaseTagBelowMinimum(t *testing.T) {
	env := setEnvValue(managerEnv(t), "FORCE_NUMBERED_UPDATE_SOURCE_PICKER", "1")
	updateChannelPath := envValue(env, "PERSIST_UPDATE_CHANNEL_FILE")
	releaseTagPath := envValue(env, "PERSIST_RELEASE_TAG_FILE")
	if updateChannelPath == "" || releaseTagPath == "" {
		t.Fatal("missing release update source state files in env")
	}

	out, err := runManagerMenu(t, env, "4\n12\n1\n4\nv1.1.28\n\n\n")
	if err != nil {
		t.Fatalf("manual release tag selection should stay in menu flow, got error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Release tag must be v1.1.29 or newer") {
		t.Fatalf("expected minimum release tag validation message, got:\n%s", out)
	}
	if _, err := os.Stat(releaseTagPath); !os.IsNotExist(err) {
		t.Fatalf("expected no pinned release tag to be persisted, stat err=%v", err)
	}

	statusOut, err := runManager(t, env, "status")
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, statusOut)
	}
	if !strings.Contains(statusOut, "ref       : latest") {
		t.Fatalf("expected status to stay on latest release after rejected old tag, got:\n%s", statusOut)
	}
}

func TestManagerInstallShowsRateLimitHintWhenAPIReturnsEmpty(t *testing.T) {
	env := managerEnv(t)

	sourceBin := envValue(env, "SOURCE_BIN")
	if sourceBin == "" {
		t.Fatal("missing SOURCE_BIN in env")
	}

	if err := os.Remove(sourceBin); err != nil {
		t.Fatalf("remove source bin: %v", err)
	}

	rateLimitFile := sourceBin + ".ratelimit.json"
	writeFile(t, rateLimitFile, "{\"message\":\"API rate limit exceeded\"}\n", 0o644)
	env = setEnvValue(env, "RELEASE_API_URL", "file://"+rateLimitFile)

	out, err := runManager(t, env, "install")
	if err == nil {
		t.Fatalf("expected install to fail when API returns empty tag, got output:\n%s", out)
	}
	if !strings.Contains(out, "Could not detect latest release version") {
		t.Fatalf("expected version detection error, got:\n%s", out)
	}
	if !strings.Contains(out, "rate limit") {
		t.Fatalf("expected rate limit hint in output, got:\n%s", out)
	}
}

func TestManagerUpdateShowsRateLimitHintWhenAPIReturnsEmpty(t *testing.T) {
	env := managerEnv(t)

	rateLimitFile := t.TempDir() + "/ratelimit.json"
	writeFile(t, rateLimitFile, "{\"message\":\"API rate limit exceeded\"}\n", 0o644)
	env = setEnvValue(env, "RELEASE_API_URL", "file://"+rateLimitFile)

	out, err := runManager(t, env, "update")
	if err == nil {
		t.Fatalf("expected update to fail when API returns empty tag, got output:\n%s", out)
	}
	if !strings.Contains(out, "Could not detect latest release version") {
		t.Fatalf("expected version detection error, got:\n%s", out)
	}
	if !strings.Contains(out, "rate limit") {
		t.Fatalf("expected rate limit hint in output, got:\n%s", out)
	}
}
