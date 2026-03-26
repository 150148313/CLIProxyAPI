package management

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

func TestListAuthFiles_ExposesStructuredLastErrorFieldsFromStatusMessage(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	authDir := t.TempDir()
	fileName := "codex-user.json"
	filePath := filepath.Join(authDir, fileName)
	if err := os.WriteFile(filePath, []byte(`{"type":"codex","email":"user@example.com"}`), 0o600); err != nil {
		t.Fatalf("failed to write auth file: %v", err)
	}

	manager := coreauth.NewManager(nil, nil, nil)
	statusMessage := "{\n  \"error\": {\n    \"message\": \"Encountered invalidated oauth token for user, failing request\",\n    \"code\": \"token_revoked\"\n  },\n  \"status\": 401\n}"
	auth := &coreauth.Auth{
		ID:            fileName,
		FileName:      fileName,
		Provider:      "codex",
		Label:         "user@example.com",
		Status:        coreauth.StatusError,
		StatusMessage: statusMessage,
		Unavailable:   true,
		Attributes: map[string]string{
			"path": filePath,
		},
		Metadata: map[string]any{
			"type":  "codex",
			"email": "user@example.com",
		},
	}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("failed to register auth: %v", err)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v0/management/auth-files", nil)

	h.ListAuthFiles(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected list status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var payload struct {
		Files []map[string]any `json:"files"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(payload.Files) != 1 {
		t.Fatalf("expected 1 file entry, got %d", len(payload.Files))
	}
	if got := int(payload.Files[0]["last_error_http_status"].(float64)); got != http.StatusUnauthorized {
		t.Fatalf("expected last_error_http_status=401, got %d", got)
	}
	if got := payload.Files[0]["last_error_code"]; got != "token_revoked" {
		t.Fatalf("expected last_error_code token_revoked, got %#v", got)
	}
	if got := payload.Files[0]["last_error_message"]; got != "Encountered invalidated oauth token for user, failing request" {
		t.Fatalf("unexpected last_error_message: %#v", got)
	}
}

func TestCleanupAuthFiles_Returns503WithoutAuthManager(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: t.TempDir()}, nil)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/cleanup", nil)

	h.CleanupAuthFiles(ctx)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusServiceUnavailable, rec.Code, rec.Body.String())
	}
}

func TestCleanupAuthFiles_DryRunMatchesTokenRevokedStatusMessage(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	authDir := t.TempDir()
	fileName := "aleece450c70@r9.leadharbor.org.json"
	filePath := filepath.Join(authDir, fileName)
	if err := os.WriteFile(filePath, []byte(`{"type":"codex","email":"aleece450c70@r9.leadharbor.org"}`), 0o600); err != nil {
		t.Fatalf("failed to write auth file: %v", err)
	}

	manager := coreauth.NewManager(nil, nil, nil)
	auth := &coreauth.Auth{
		ID:             fileName,
		FileName:       fileName,
		Provider:       "codex",
		Label:          "aleece450c70@r9.leadharbor.org",
		Status:         coreauth.StatusError,
		StatusMessage:  "{\n  \"error\": {\n    \"message\": \"Encountered invalidated oauth token for user, failing request\",\n    \"type\": null,\n    \"code\": \"token_revoked\",\n    \"param\": null\n  },\n  \"status\": 401\n}",
		Unavailable:    true,
		NextRetryAfter: time.Now().Add(30 * time.Minute),
		Attributes: map[string]string{
			"path": filePath,
		},
		Metadata: map[string]any{
			"type":  "codex",
			"email": "aleece450c70@r9.leadharbor.org",
		},
	}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("failed to register auth: %v", err)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/cleanup", http.NoBody)

	h.CleanupAuthFiles(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if matched := int(payload["matched"].(float64)); matched != 1 {
		t.Fatalf("expected matched=1, got %d", matched)
	}
	if dryRun, ok := payload["dry_run"].(bool); !ok || !dryRun {
		t.Fatalf("expected dry_run=true, got %#v", payload["dry_run"])
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("expected auth file to remain during dry_run: %v", err)
	}
	candidates, ok := payload["candidates"].([]any)
	if !ok || len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %#v", payload["candidates"])
	}
}

func TestCleanupAuthFiles_DeletesMatchedAuthFile(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	authDir := t.TempDir()
	fileName := "delete-me.json"
	filePath := filepath.Join(authDir, fileName)
	if err := os.WriteFile(filePath, []byte(`{"type":"codex","email":"delete@example.com"}`), 0o600); err != nil {
		t.Fatalf("failed to write auth file: %v", err)
	}

	manager := coreauth.NewManager(nil, nil, nil)
	auth := &coreauth.Auth{
		ID:          fileName,
		FileName:    fileName,
		Provider:    "codex",
		Status:      coreauth.StatusError,
		Unavailable: true,
		Attributes: map[string]string{
			"path": filePath,
		},
		LastError: &coreauth.Error{
			HTTPStatus: http.StatusUnauthorized,
			Code:       "token_revoked",
			Message:    "revoked",
		},
	}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("failed to register auth: %v", err)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)
	h.tokenStore = &memoryAuthStore{}
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	body := `{"dry_run":false,"match":{"last_error_http_status":401}}`
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/cleanup", strings.NewReader(body))

	h.CleanupAuthFiles(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected auth file to be removed, stat err: %v", err)
	}
}

func TestCleanupAuthFiles_SkipsSingleModelUnauthorizedWhenAuthStillAvailable(t *testing.T) {
	t.Setenv("MANAGEMENT_PASSWORD", "")
	gin.SetMode(gin.TestMode)

	authDir := t.TempDir()
	fileName := "single-model.json"
	filePath := filepath.Join(authDir, fileName)
	if err := os.WriteFile(filePath, []byte(`{"type":"codex","email":"model@example.com"}`), 0o600); err != nil {
		t.Fatalf("failed to write auth file: %v", err)
	}

	manager := coreauth.NewManager(nil, nil, nil)
	auth := &coreauth.Auth{
		ID:            fileName,
		FileName:      fileName,
		Provider:      "codex",
		Status:        coreauth.StatusError,
		StatusMessage: "{\"status\":401,\"error\":{\"code\":\"token_revoked\",\"message\":\"revoked\"}}",
		Unavailable:   false,
		Attributes: map[string]string{
			"path": filePath,
		},
		ModelStates: map[string]*coreauth.ModelState{
			"bad-model": {
				Status:         coreauth.StatusError,
				StatusMessage:  "{\"status\":401,\"error\":{\"code\":\"token_revoked\",\"message\":\"revoked\"}}",
				Unavailable:    true,
				NextRetryAfter: time.Now().Add(15 * time.Minute),
				LastError: &coreauth.Error{
					HTTPStatus: http.StatusUnauthorized,
					Code:       "token_revoked",
					Message:    "revoked",
				},
			},
			"good-model": {
				Status: coreauth.StatusActive,
			},
		},
	}
	if _, err := manager.Register(context.Background(), auth); err != nil {
		t.Fatalf("failed to register auth: %v", err)
	}

	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v0/management/auth-files/cleanup", http.NoBody)

	h.CleanupAuthFiles(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if matched := int(payload["matched"].(float64)); matched != 0 {
		t.Fatalf("expected matched=0, got %d", matched)
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("expected auth file to remain, stat err: %v", err)
	}
}
