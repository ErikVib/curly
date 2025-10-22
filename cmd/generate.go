package cmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"
)

func NewGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate <openapi-file>",
		Short: "Generate a directory full of .curl files from an OpenAPI YAML/JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			openapiFile := args[0]
			outDir := "collection"
			return generateCollection(openapiFile, outDir)
		},
	}
	return cmd
}

func generateCollection(openapiFile, outDir string) error {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	doc, err := loader.LoadFromFile(openapiFile)
	if err != nil {
		return fmt.Errorf("failed to load OpenAPI file: %w", err)
	}

	baseURL := "http://localhost"
	if len(doc.Servers) > 0 && doc.Servers[0].URL != "" {
		baseURL = doc.Servers[0].URL
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	write := func(name, contents string) error {
		path := filepath.Join(outDir, name)
		return ioutil.WriteFile(path, []byte(contents), 0644)
	}

	sanitize := func(s string) string {
		s = strings.Trim(s, "/")
		s = strings.ReplaceAll(s, "/", "_")
		s = strings.ReplaceAll(s, "{", "_")
		s = strings.ReplaceAll(s, "}", "")
		re := regexp.MustCompile(`[^a-zA-Z0-9_\-\.]`)
		s = re.ReplaceAllString(s, "")
		if s == "" {
			return "root"
		}
		return s
	}

	for path, item := range doc.Paths.Map() {
		if item == nil {
			continue
		}
		maybeMake := func(method string, op *openapi3.Operation) error {
			if op == nil {
				return nil
			}
			fileName := fmt.Sprintf("%s_%s.curl", strings.ToUpper(method), sanitize(path))

			curl := new(bytes.Buffer)
			fmt.Fprintf(curl, "# %s %s\n", strings.ToUpper(method), path)
			if op.Summary != "" {
				fmt.Fprintf(curl, "# %s\n", op.Summary)
			}
			fmt.Fprintf(curl, "\n# Variables\n")

			vars := []string{}

			vars = append(vars, fmt.Sprintf("BASE_URL=\"%s\"", baseURL))

			pathParams := extractPathParams(path)
			for _, param := range pathParams {
				vars = append(vars, fmt.Sprintf("%s=\"VALUE\"", strings.ToUpper(param)))
			}

			if op.Parameters != nil {
				for _, paramRef := range op.Parameters {
					if paramRef.Value != nil && paramRef.Value.In == "query" {
						paramName := strings.ToUpper(paramRef.Value.Name)
						vars = append(vars, fmt.Sprintf("%s=\"VALUE\"", paramName))
					}
				}
			}

			if op.Parameters != nil {
				for _, paramRef := range op.Parameters {
					if paramRef.Value != nil && paramRef.Value.In == "header" {
						paramName := strings.ToUpper(strings.ReplaceAll(paramRef.Value.Name, "-", "_"))
						vars = append(vars, fmt.Sprintf("%s=\"VALUE\"", paramName))
					}
				}
			}

			for _, v := range vars {
				fmt.Fprintf(curl, "%s\n", v)
			}

			urlPath := path
			for _, param := range pathParams {
				urlPath = strings.ReplaceAll(urlPath, "{"+param+"}", "${"+strings.ToUpper(param)+"}")
			}

			fmt.Fprintf(curl, "\ncurl -s -X %s \"${BASE_URL}%s", strings.ToUpper(method), urlPath)

			if op.Parameters != nil {
				queryParams := []string{}
				for _, paramRef := range op.Parameters {
					if paramRef.Value != nil && paramRef.Value.In == "query" {
						paramName := strings.ToUpper(paramRef.Value.Name)
						queryParams = append(queryParams, fmt.Sprintf("%s=${%s}", paramRef.Value.Name, paramName))
					}
				}
				if len(queryParams) > 0 {
					fmt.Fprintf(curl, "?%s", strings.Join(queryParams, "&"))
				}
			}

			fmt.Fprintf(curl, "\" \\\n  -H 'Accept: application/json' \\\n  -H 'Content-Type: application/json'")

			if op.Parameters != nil {
				for _, paramRef := range op.Parameters {
					if paramRef.Value != nil && paramRef.Value.In == "header" {
						paramName := strings.ToUpper(strings.ReplaceAll(paramRef.Value.Name, "-", "_"))
						fmt.Fprintf(curl, " \\\n  -H '%s: ${%s}'", paramRef.Value.Name, paramName)
					}
				}
			}

			if op.RequestBody != nil {
				fmt.Fprintf(curl, " \\\n  -d '{\"foo\": \"bar\"}'")
			}

			fmt.Fprintf(curl, "\n")

			return write(fileName, curl.String())
		}

		_ = maybeMake("GET", item.Get)
		_ = maybeMake("POST", item.Post)
		_ = maybeMake("PUT", item.Put)
		_ = maybeMake("PATCH", item.Patch)
		_ = maybeMake("DELETE", item.Delete)
		_ = maybeMake("OPTIONS", item.Options)
		_ = maybeMake("HEAD", item.Head)
	}

	envsExample := `# Example environment configurations
# Usage: curly -e dev
environments:
  dev:
    BASE_URL: "http://localhost:8081"
    AUTHORIZATION: "dev-token"
    QUERYVAR: "dev-value"
  staging:
    BASE_URL: "http://localhost:8081"
    AUTHORIZATION: "staging-token"
    QUERYVAR: "staging-value"
`
	if err := write("envs.yml", envsExample); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create envs.yml: %v\n", err)
	}

	fmt.Printf("Generated collection in %s/\n", outDir)
	return nil
}

func extractPathParams(path string) []string {
	re := regexp.MustCompile(`\{([^}]+)\}`)
	matches := re.FindAllStringSubmatch(path, -1)
	params := []string{}
	for _, match := range matches {
		if len(match) > 1 {
			params = append(params, match[1])
		}
	}
	return params
}
