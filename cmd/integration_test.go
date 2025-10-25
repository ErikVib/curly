// +build integration

package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Integration tests require the binary to be built first
// Run with: go test -tags=integration ./...

func TestEndToEndGenerate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	// Create a simple OpenAPI spec
	openapiFile := filepath.Join(tmpDir, "openapi.yml")
	openapiContent := `openapi: 3.0.1
info:
  title: Integration Test API
  version: v1
servers:
  - url: http://localhost:9999
paths:
  /test:
    get:
      operationId: test
      responses:
        '200':
          description: OK
`

	if err := os.WriteFile(openapiFile, []byte(openapiContent), 0644); err != nil {
		t.Fatalf("failed to create test openapi file: %v", err)
	}

	// Change to tmpDir for output
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(tmpDir)

	// Test generate command
	err := generateCollection(openapiFile, "collection")
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	// Verify files exist
	curlFile := filepath.Join(tmpDir, "collection", "GET_test.curl")
	if _, err := os.Stat(curlFile); os.IsNotExist(err) {
		t.Errorf("expected curl file not created: %s", curlFile)
	}

	envsFile := filepath.Join(tmpDir, "collection", "envs.yml")
	if _, err := os.Stat(envsFile); os.IsNotExist(err) {
		t.Errorf("expected envs.yml not created: %s", envsFile)
	}
}

func TestExecutionStats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	stats := &ExecutionStats{
		Total:     100,
		StartTime: time.Now(),
	}

	// Simulate some successes and failures
	for i := 0; i < 95; i++ {
		stats.RecordSuccess()
	}

	for i := 0; i < 5; i++ {
		stats.RecordFailure(exec.ErrNotFound)
	}

	stats.EndTime = time.Now().Add(5 * time.Second)

	// Verify counts
	if stats.Success != 95 {
		t.Errorf("success count = %d, want 95", stats.Success)
	}
	if stats.Failed != 5 {
		t.Errorf("failed count = %d, want 5", stats.Failed)
	}

	// Verify errors collected
	if len(stats.Errors) != 5 {
		t.Errorf("collected %d errors, want 5", len(stats.Errors))
	}
}

func TestConcurrentStatsRecording(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	stats := &ExecutionStats{
		Total:     1000,
		StartTime: time.Now(),
	}

	// Simulate concurrent recording
	done := make(chan bool)
	
	// Record 500 successes concurrently
	for i := 0; i < 500; i++ {
		go func() {
			stats.RecordSuccess()
			done <- true
		}()
	}

	// Record 500 failures concurrently
	for i := 0; i < 500; i++ {
		go func(n int) {
			stats.RecordFailure(exec.ErrNotFound)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 1000; i++ {
		<-done
	}

	stats.EndTime = time.Now()

	// Verify final counts (should be thread-safe)
	if stats.Success != 500 {
		t.Errorf("success count = %d, want 500", stats.Success)
	}
	if stats.Failed != 500 {
		t.Errorf("failed count = %d, want 500", stats.Failed)
	}
	if len(stats.Errors) != 500 {
		t.Errorf("collected %d errors, want 500", len(stats.Errors))
	}
}

func TestApplyEnvironmentVarsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	// Create a curl file
	curlFile := filepath.Join(tmpDir, "test.curl")
	curlContent := `# GET /test

# Variables
BASE_URL="http://localhost"
TOKEN="default-token"

curl -s -X GET "${BASE_URL}/test" -H "Authorization: ${TOKEN}"
`

	if err := os.WriteFile(curlFile, []byte(curlContent), 0644); err != nil {
		t.Fatalf("failed to create test curl file: %v", err)
	}

	// Create envs.yml
	envsFile := filepath.Join(tmpDir, "envs.yml")
	envsContent := `environments:
  test:
    BASE_URL: "http://test-server:8080"
    TOKEN: "test-token-123"
`

	if err := os.WriteFile(envsFile, []byte(envsContent), 0644); err != nil {
		t.Fatalf("failed to create envs.yml: %v", err)
	}

	// Load environment
	config, err := loadEnvConfig(envsFile)
	if err != nil {
		t.Fatalf("failed to load env config: %v", err)
	}

	testEnv, ok := config.Environments["test"]
	if !ok {
		t.Fatal("test environment not found")
	}

	// Apply environment variables
	content, err := os.ReadFile(curlFile)
	if err != nil {
		t.Fatalf("failed to read curl file: %v", err)
	}

	result := applyEnvironmentVars(string(content), testEnv)

	// Verify replacements
	if !strings.Contains(result, `BASE_URL="http://test-server:8080"`) {
		t.Error("BASE_URL was not replaced correctly")
	}
	if !strings.Contains(result, `TOKEN="test-token-123"`) {
		t.Error("TOKEN was not replaced correctly")
	}

	// Original curl command should remain unchanged
	if !strings.Contains(result, `curl -s -X GET "${BASE_URL}/test"`) {
		t.Error("curl command was modified")
	}
}

func TestRunFileWithInsecureFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	// Create a curl file
	curlFile := filepath.Join(tmpDir, "test.curl")
	curlContent := `# GET /test

# Variables
BASE_URL="https://localhost:8443"

curl -s -X GET "${BASE_URL}/test"
`

	if err := os.WriteFile(curlFile, []byte(curlContent), 0644); err != nil {
		t.Fatalf("failed to create test curl file: %v", err)
	}

	// Test without insecure flag
	cmdText, err := runFile(curlFile, tmpDir, "", false)
	if err != nil {
		t.Fatalf("runFile failed: %v", err)
	}

	if !strings.Contains(cmdText, "curl -s -X GET") {
		t.Error("expected standard curl command without -k flag")
	}
	if strings.Contains(cmdText, "curl -k") {
		t.Error("unexpected -k flag in command")
	}

	// Test with insecure flag
	cmdTextInsecure, err := runFile(curlFile, tmpDir, "", true)
	if err != nil {
		t.Fatalf("runFile with insecure failed: %v", err)
	}

	if !strings.Contains(cmdTextInsecure, "curl -k -s -X GET") {
		t.Errorf("expected curl with -k flag, got: %s", cmdTextInsecure)
	}
}

func TestRunFileWithInsecureAndEnv(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir := t.TempDir()

	// Create a curl file
	curlFile := filepath.Join(tmpDir, "test.curl")
	curlContent := `# GET /test

# Variables
BASE_URL="https://localhost"
TOKEN="default"

curl -s -X GET "${BASE_URL}/test" -H "Authorization: ${TOKEN}"
`

	if err := os.WriteFile(curlFile, []byte(curlContent), 0644); err != nil {
		t.Fatalf("failed to create test curl file: %v", err)
	}

	// Create envs.yml
	envsFile := filepath.Join(tmpDir, "envs.yml")
	envsContent := `environments:
  dev:
    BASE_URL: "https://dev.example.com"
    TOKEN: "dev-token"
`

	if err := os.WriteFile(envsFile, []byte(envsContent), 0644); err != nil {
		t.Fatalf("failed to create envs.yml: %v", err)
	}

	// Test with both env and insecure flag
	cmdText, err := runFile(curlFile, tmpDir, "dev", true)
	if err != nil {
		t.Fatalf("runFile failed: %v", err)
	}

	// Should have -k flag
	if !strings.Contains(cmdText, "curl -k") {
		t.Error("expected -k flag in curl command")
	}

	// Should have env vars applied
	if !strings.Contains(cmdText, `BASE_URL="https://dev.example.com"`) {
		t.Error("BASE_URL was not replaced with env value")
	}
	if !strings.Contains(cmdText, `TOKEN="dev-token"`) {
		t.Error("TOKEN was not replaced with env value")
	}
}
