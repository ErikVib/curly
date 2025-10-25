package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInputValidation(t *testing.T) {
	tests := []struct {
		name      string
		times     int
		parallel  int
		delay     int
		wantError bool
	}{
		{
			name:      "valid inputs",
			times:     10,
			parallel:  5,
			delay:     1,
			wantError: false,
		},
		{
			name:      "times less than 1",
			times:     0,
			parallel:  1,
			delay:     0,
			wantError: true,
		},
		{
			name:      "times negative",
			times:     -5,
			parallel:  1,
			delay:     0,
			wantError: true,
		},
		{
			name:      "parallel less than 1",
			times:     10,
			parallel:  0,
			delay:     0,
			wantError: true,
		},
		{
			name:      "negative delay",
			times:     10,
			parallel:  5,
			delay:     -1,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validation logic from NewRootCmd
			hasError := false
			if tt.times < 1 {
				hasError = true
			}
			if tt.parallel < 1 {
				hasError = true
			}
			if tt.delay < 0 {
				hasError = true
			}

			if hasError != tt.wantError {
				t.Errorf("validation error = %v, want %v", hasError, tt.wantError)
			}
		})
	}
}

func TestParallelAutoAdjust(t *testing.T) {
	tests := []struct {
		name     string
		times    int
		parallel int
		expected int
	}{
		{
			name:     "parallel greater than times",
			times:    5,
			parallel: 10,
			expected: 5,
		},
		{
			name:     "parallel equal to times",
			times:    10,
			parallel: 10,
			expected: 10,
		},
		{
			name:     "parallel less than times",
			times:    100,
			parallel: 10,
			expected: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parallel := tt.parallel
			if parallel > tt.times {
				parallel = tt.times
			}

			if parallel != tt.expected {
				t.Errorf("adjusted parallel = %d, want %d", parallel, tt.expected)
			}
		})
	}
}

func TestBatchCalculation(t *testing.T) {
	tests := []struct {
		name           string
		times          int
		parallel       int
		expectedBatches int
	}{
		{
			name:           "evenly divisible",
			times:          100,
			parallel:       10,
			expectedBatches: 10,
		},
		{
			name:           "remainder",
			times:          25,
			parallel:       10,
			expectedBatches: 3,
		},
		{
			name:           "single batch",
			times:          5,
			parallel:       10,
			expectedBatches: 1,
		},
		{
			name:           "sequential",
			times:          10,
			parallel:       1,
			expectedBatches: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Ceiling division formula
			batches := (tt.times + tt.parallel - 1) / tt.parallel

			if batches != tt.expectedBatches {
				t.Errorf("batches = %d, want %d", batches, tt.expectedBatches)
			}
		})
	}
}

func TestExtractPathParams(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected []string
	}{
		{
			name:     "no parameters",
			path:     "/users",
			expected: []string{},
		},
		{
			name:     "single parameter",
			path:     "/users/{id}",
			expected: []string{"id"},
		},
		{
			name:     "multiple parameters",
			path:     "/users/{userId}/posts/{postId}",
			expected: []string{"userId", "postId"},
		},
		{
			name:     "parameter at start",
			path:     "/{resource}/items",
			expected: []string{"resource"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPathParams(tt.path)

			if len(result) != len(tt.expected) {
				t.Errorf("got %d params, want %d", len(result), len(tt.expected))
				return
			}

			for i, param := range result {
				if param != tt.expected[i] {
					t.Errorf("param[%d] = %s, want %s", i, param, tt.expected[i])
				}
			}
		})
	}
}

func TestExtractShellCommand(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name: "basic curl with variables",
			content: `# GET /test

# Variables
BASE_URL="http://localhost:8081"

curl -s -X GET "${BASE_URL}/test"`,
			expected: `BASE_URL="http://localhost:8081"

curl -s -X GET "${BASE_URL}/test"`,
		},
		{
			name: "skip leading comments",
			content: `# Some comment
# Another comment

VAR="value"
curl test`,
			expected: `VAR="value"
curl test`,
		},
		{
			name: "empty lines before command",
			content: `

curl test`,
			expected: "curl test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractShellCommand(tt.content)

			if result != tt.expected {
				t.Errorf("extractShellCommand() =\n%q\n\nwant:\n%q", result, tt.expected)
			}
		})
	}
}

func TestApplyEnvironmentVars(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		envVars  Environment
		expected string
	}{
		{
			name: "replace single variable",
			content: `# Variables
BASE_URL="http://localhost"

curl "${BASE_URL}/test"`,
			envVars: Environment{
				"BASE_URL": "http://production.com",
			},
			expected: `# Variables
BASE_URL="http://production.com"

curl "${BASE_URL}/test"`,
		},
		{
			name: "replace multiple variables",
			content: `# Variables
BASE_URL="VALUE"
TOKEN="VALUE"

curl test`,
			envVars: Environment{
				"BASE_URL": "http://dev.local",
				"TOKEN":    "dev-token-123",
			},
			expected: `# Variables
BASE_URL="http://dev.local"
TOKEN="dev-token-123"

curl test`,
		},
		{
			name: "variable not in env keeps original",
			content: `# Variables
BASE_URL="VALUE"
OTHER="VALUE"

curl test`,
			envVars: Environment{
				"BASE_URL": "http://test.com",
			},
			expected: `# Variables
BASE_URL="http://test.com"
OTHER="VALUE"

curl test`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyEnvironmentVars(tt.content, tt.envVars)

			if result != tt.expected {
				t.Errorf("applyEnvironmentVars() =\n%q\n\nwant:\n%q", result, tt.expected)
			}
		})
	}
}

func TestLoadEnvConfig(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	envsFile := filepath.Join(tmpDir, "envs.yml")

	content := `environments:
  dev:
    BASE_URL: "http://localhost:8081"
    TOKEN: "dev-token"
  prod:
    BASE_URL: "https://api.production.com"
    TOKEN: "prod-token"
`

	if err := os.WriteFile(envsFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	config, err := loadEnvConfig(envsFile)
	if err != nil {
		t.Fatalf("loadEnvConfig() error = %v", err)
	}

	// Check environments exist
	if _, ok := config.Environments["dev"]; !ok {
		t.Error("dev environment not found")
	}
	if _, ok := config.Environments["prod"]; !ok {
		t.Error("prod environment not found")
	}

	// Check dev values
	devEnv := config.Environments["dev"]
	if devEnv["BASE_URL"] != "http://localhost:8081" {
		t.Errorf("dev BASE_URL = %s, want http://localhost:8081", devEnv["BASE_URL"])
	}
	if devEnv["TOKEN"] != "dev-token" {
		t.Errorf("dev TOKEN = %s, want dev-token", devEnv["TOKEN"])
	}

	// Check prod values
	prodEnv := config.Environments["prod"]
	if prodEnv["BASE_URL"] != "https://api.production.com" {
		t.Errorf("prod BASE_URL = %s, want https://api.production.com", prodEnv["BASE_URL"])
	}
}

func TestLoadEnvConfigFileNotFound(t *testing.T) {
	_, err := loadEnvConfig("nonexistent.yml")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestInsecureFlagAddsKToCurl(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		insecure bool
		expected string
	}{
		{
			name: "insecure flag adds -k to basic curl",
			content: `# Variables
BASE_URL="http://localhost"

curl -s -X GET "${BASE_URL}/test"`,
			insecure: true,
			expected: `# Variables
BASE_URL="http://localhost"

curl -k -s -X GET "${BASE_URL}/test"`,
		},
		{
			name: "insecure flag adds -k to curl with headers",
			content: `curl -X POST "${BASE_URL}/api" \
  -H "Content-Type: application/json"`,
			insecure: true,
			expected: `curl -k -X POST "${BASE_URL}/api" \
  -H "Content-Type: application/json"`,
		},
		{
			name: "insecure false does not modify curl",
			content: `curl -s -X GET "${BASE_URL}/test"`,
			insecure: false,
			expected: `curl -s -X GET "${BASE_URL}/test"`,
		},
		{
			name: "multiple curl commands all get -k",
			content: `curl -X GET test1
curl -X POST test2`,
			insecure: true,
			expected: `curl -k -X GET test1
curl -k -X POST test2`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.content
			if tt.insecure {
				// This is the same logic used in runFile and launchCollection
				result = strings.ReplaceAll(result, "curl ", "curl -k ")
			}

			if result != tt.expected {
				t.Errorf("insecure flag application =\n%q\n\nwant:\n%q", result, tt.expected)
			}
		})
	}
}
