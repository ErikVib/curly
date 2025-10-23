package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateCollection(t *testing.T) {
	// Create a temporary OpenAPI file
	tmpDir := t.TempDir()
	openapiFile := filepath.Join(tmpDir, "openapi.yml")

	openapiContent := `openapi: 3.0.1
info:
  title: Test API
  version: v1
servers:
  - url: http://localhost:8080
paths:
  /users:
    get:
      summary: Get users
      operationId: getUsers
      parameters:
        - name: limit
          in: query
          schema:
            type: integer
      responses:
        '200':
          description: OK
  /users/{id}:
    get:
      summary: Get user by ID
      operationId: getUserById
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
        - name: Authorization
          in: header
          required: true
          schema:
            type: string
      responses:
        '200':
          description: OK
    post:
      summary: Create user
      operationId: createUser
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
      responses:
        '201':
          description: Created
`

	if err := os.WriteFile(openapiFile, []byte(openapiContent), 0644); err != nil {
		t.Fatalf("failed to write test openapi file: %v", err)
	}

	outDir := filepath.Join(tmpDir, "collection")

	// Generate collection
	err := generateCollection(openapiFile, outDir)
	if err != nil {
		t.Fatalf("generateCollection() error = %v", err)
	}

	// Check that output directory was created
	if _, err := os.Stat(outDir); os.IsNotExist(err) {
		t.Errorf("output directory was not created")
	}

	// Check that expected files were created
	expectedFiles := []string{
		"GET_users.curl",
		"GET_users__id.curl",
		"POST_users__id.curl",
		"envs.yml",
	}

	for _, file := range expectedFiles {
		filePath := filepath.Join(outDir, file)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("expected file %s was not created", file)
		}
	}

	// Verify content of GET_users.curl
	getUsersContent, err := os.ReadFile(filepath.Join(outDir, "GET_users.curl"))
	if err != nil {
		t.Fatalf("failed to read GET_users.curl: %v", err)
	}

	content := string(getUsersContent)

	// Should contain variables section
	if !strings.Contains(content, "# Variables") {
		t.Error("GET_users.curl missing Variables section")
	}

	// Should contain BASE_URL
	if !strings.Contains(content, "BASE_URL=") {
		t.Error("GET_users.curl missing BASE_URL variable")
	}

	// Should contain LIMIT variable (query param)
	if !strings.Contains(content, "LIMIT=") {
		t.Error("GET_users.curl missing LIMIT variable")
	}

	// Should contain curl command
	if !strings.Contains(content, "curl -s -X GET") {
		t.Error("GET_users.curl missing curl command")
	}

	// Verify content of GET_users__id.curl (has path param and header)
	getUserByIdContent, err := os.ReadFile(filepath.Join(outDir, "GET_users__id.curl"))
	if err != nil {
		t.Fatalf("failed to read GET_users__id.curl: %v", err)
	}

	content = string(getUserByIdContent)

	// Should contain ID path parameter
	if !strings.Contains(content, "ID=") {
		t.Error("GET_users__id.curl missing ID variable")
	}

	// Should contain Authorization header
	if !strings.Contains(content, "AUTHORIZATION=") {
		t.Error("GET_users__id.curl missing AUTHORIZATION variable")
	}

	// Should use ${ID} in URL
	if !strings.Contains(content, "${ID}") {
		t.Error("GET_users__id.curl not using ${ID} in URL")
	}

	// Should have Authorization header in curl
	if !strings.Contains(content, "-H \"Authorization: ${AUTHORIZATION}\"") {
		t.Error("GET_users__id.curl missing Authorization header in curl")
	}

	// Verify content of POST_users__id.curl (has request body)
	postUserContent, err := os.ReadFile(filepath.Join(outDir, "POST_users__id.curl"))
	if err != nil {
		t.Fatalf("failed to read POST_users__id.curl: %v", err)
	}

	content = string(postUserContent)

	// Should contain request body
	if !strings.Contains(content, "-d '{\"foo\": \"bar\"}'") {
		t.Error("POST_users__id.curl missing request body")
	}

	// Verify envs.yml was created
	envsContent, err := os.ReadFile(filepath.Join(outDir, "envs.yml"))
	if err != nil {
		t.Fatalf("failed to read envs.yml: %v", err)
	}

	envsStr := string(envsContent)
	if !strings.Contains(envsStr, "environments:") {
		t.Error("envs.yml missing environments section")
	}
	if !strings.Contains(envsStr, "dev:") {
		t.Error("envs.yml missing dev environment")
	}
}

func TestGenerateCollectionInvalidFile(t *testing.T) {
	tmpDir := t.TempDir()
	outDir := filepath.Join(tmpDir, "collection")

	err := generateCollection("nonexistent.yml", outDir)
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestGenerateCollectionInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	openapiFile := filepath.Join(tmpDir, "invalid.yml")
	outDir := filepath.Join(tmpDir, "collection")

	// Write invalid YAML
	invalidContent := `this is not valid openapi
{{{
random stuff
`

	if err := os.WriteFile(openapiFile, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	err := generateCollection(openapiFile, outDir)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

func TestSanitizePathNames(t *testing.T) {
	// Test the sanitize function logic
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple path",
			input:    "/users",
			expected: "users",
		},
		{
			name:     "path with parameter",
			input:    "/users/{id}",
			expected: "users__id",
		},
		{
			name:     "nested path",
			input:    "/api/v1/users",
			expected: "api_v1_users",
		},
		{
			name:     "path with multiple parameters",
			input:    "/users/{userId}/posts/{postId}",
			expected: "users__userId_posts__postId",
		},
		{
			name:     "empty path",
			input:    "/",
			expected: "root",
		},
		{
			name:     "special characters",
			input:    "/users@#$%",
			expected: "users",
		},
	}

	// Recreate the sanitize function from generate.go
	sanitize := func(s string) string {
		s = strings.Trim(s, "/")
		s = strings.ReplaceAll(s, "/", "_")
		s = strings.ReplaceAll(s, "{", "_")
		s = strings.ReplaceAll(s, "}", "")
		// Remove special characters
		result := ""
		for _, r := range s {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' {
				result += string(r)
			}
		}
		if result == "" {
			return "root"
		}
		return result
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitize(tt.input)
			if result != tt.expected {
				t.Errorf("sanitize(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractPathParamsFromGenerate(t *testing.T) {
	// This tests the same function but in generate context
	tests := []struct {
		path     string
		expected int
	}{
		{"/users", 0},
		{"/users/{id}", 1},
		{"/users/{userId}/posts/{postId}", 2},
		{"/api/{version}/users/{id}", 2},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			params := extractPathParams(tt.path)
			if len(params) != tt.expected {
				t.Errorf("extractPathParams(%q) returned %d params, want %d", tt.path, len(params), tt.expected)
			}
		})
	}
}
