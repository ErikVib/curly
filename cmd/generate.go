package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"
)

type parameterSet struct {
	pathParams     []string
	queryParams    []string
	headerParams   []string
	formDataParams []string
	bodyVars       map[string]any
}

type requestBodyInfo struct {
	exampleBody string
	contentType string
	bodyVars    map[string]any
}

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

	// Load OpenAPI spec from file or URL
	doc, err := func() (*openapi3.T, error) {
		if strings.HasPrefix(openapiFile, "http://") || strings.HasPrefix(openapiFile, "https://") {
			parsedURL, err := url.Parse(openapiFile)
			if err != nil {
				return nil, fmt.Errorf("invalid URL '%s': %w", openapiFile, err)
			}
			return loader.LoadFromURI(parsedURL)
		}
		return loader.LoadFromFile(openapiFile)
	}()
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
		return os.WriteFile(path, []byte(contents), 0644)
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

			params := extractRequestParameters(path, op, doc)
			bodyInfo := extractRequestBody(op, doc)

			fmt.Fprintf(curl, "\nBASE_URL=\"%s\"\n", baseURL)
			writeVariableSections(curl, params, bodyInfo)
			buildCurlCommand(curl, method, path, params.pathParams, op, params.formDataParams, bodyInfo)

			return write(fileName, curl.String())
		}

		if err := maybeMake("GET", item.Get); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to generate GET %s: %v\n", path, err)
		}
		if err := maybeMake("POST", item.Post); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to generate POST %s: %v\n", path, err)
		}
		if err := maybeMake("PUT", item.Put); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to generate PUT %s: %v\n", path, err)
		}
		if err := maybeMake("PATCH", item.Patch); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to generate PATCH %s: %v\n", path, err)
		}
		if err := maybeMake("DELETE", item.Delete); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to generate DELETE %s: %v\n", path, err)
		}
		if err := maybeMake("OPTIONS", item.Options); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to generate OPTIONS %s: %v\n", path, err)
		}
		if err := maybeMake("HEAD", item.Head); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to generate HEAD %s: %v\n", path, err)
		}
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

// extractRequestParameters extracts all parameters from an OpenAPI operation
func extractRequestParameters(path string, op *openapi3.Operation, doc *openapi3.T) parameterSet {
	params := parameterSet{
		pathParams:     extractPathParams(path),
		queryParams:    []string{},
		headerParams:   []string{},
		formDataParams: []string{},
		bodyVars:       map[string]any{},
	}

	if op.Parameters == nil {
		return params
	}

	for _, paramRef := range op.Parameters {
		if paramRef.Value == nil {
			continue
		}

		switch paramRef.Value.In {
		case "query":
			paramName := strings.ToUpper(paramRef.Value.Name)
			params.queryParams = append(params.queryParams, paramName)
		case "header":
			paramName := strings.ToUpper(strings.ReplaceAll(paramRef.Value.Name, "-", "_"))
			params.headerParams = append(params.headerParams, paramName)
		case "formData":
			paramName := strings.ToUpper(paramRef.Value.Name)
			params.formDataParams = append(params.formDataParams, paramName)
		}
	}

	return params
}

// extractRequestBody extracts request body information from an OpenAPI operation
func extractRequestBody(op *openapi3.Operation, doc *openapi3.T) requestBodyInfo {
	bodyInfo := requestBodyInfo{
		bodyVars: make(map[string]any),
	}

	// OpenAPI 3.0 style (requestBody)
	if op.RequestBody != nil && op.RequestBody.Value != nil {
		for ct, mediaType := range op.RequestBody.Value.Content {
			bodyInfo.contentType = ct
			if mediaType.Example != nil {
				bodyInfo.bodyVars = extractBodyVariablesFromAny(mediaType.Example)
				bodyInfo.exampleBody = formatExampleWithVars(mediaType.Example, bodyInfo.contentType)
				return bodyInfo
			} else if len(mediaType.Examples) > 0 {
				for _, exampleRef := range mediaType.Examples {
					if exampleRef.Value != nil && exampleRef.Value.Value != nil {
						bodyInfo.bodyVars = extractBodyVariablesFromAny(exampleRef.Value.Value)
						bodyInfo.exampleBody = formatExampleWithVars(exampleRef.Value.Value, bodyInfo.contentType)
						return bodyInfo
					}
				}
				return bodyInfo
			} else if mediaType.Schema != nil {
				schemaExample := generateExampleFromSchema(mediaType.Schema.Value, doc)
				if schemaExample != nil {
					bodyInfo.bodyVars = extractBodyVariablesFromAny(schemaExample)
					bodyInfo.exampleBody = formatExampleWithVars(schemaExample, bodyInfo.contentType)
					return bodyInfo
				}
			}
		}
	}

	// Swagger 2.0 style (parameters with in: "body")
	if op.Parameters != nil {
		for _, paramRef := range op.Parameters {
			if paramRef.Value != nil && paramRef.Value.In == "body" && paramRef.Value.Schema != nil {
				bodyInfo.contentType = "application/json"
				schema := paramRef.Value.Schema.Value
				schemaExample := generateExampleFromSchema(schema, doc)
				if schemaExample != nil {
					bodyInfo.bodyVars = extractBodyVariablesFromAny(schemaExample)
					bodyInfo.exampleBody = formatExampleWithVars(schemaExample, bodyInfo.contentType)
					return bodyInfo
				}
			}
		}
	}

	return bodyInfo
}

// writeVariableSections writes all variable sections to the curl buffer
func writeVariableSections(curl *bytes.Buffer, params parameterSet, bodyInfo requestBodyInfo) {
	if len(params.pathParams) > 0 {
		fmt.Fprintf(curl, "\n# Path Parameters\n")
		for _, param := range params.pathParams {
			fmt.Fprintf(curl, "%s=\"VALUE\"\n", strings.ToUpper(param))
		}
	}
	if len(params.queryParams) > 0 {
		fmt.Fprintf(curl, "\n# Query Parameters\n")
		for _, paramName := range params.queryParams {
			fmt.Fprintf(curl, "%s=\"VALUE\"\n", paramName)
		}
	}
	if len(params.headerParams) > 0 {
		fmt.Fprintf(curl, "\n# Headers\n")
		for _, paramName := range params.headerParams {
			fmt.Fprintf(curl, "%s=\"VALUE\"\n", paramName)
		}
	}
	if len(params.formDataParams) > 0 {
		fmt.Fprintf(curl, "\n# Form Data\n")
		for _, paramName := range params.formDataParams {
			fmt.Fprintf(curl, "%s=\"VALUE\"\n", paramName)
		}
	}
	if len(bodyInfo.bodyVars) > 0 {
		fmt.Fprintf(curl, "\n# Body\n")
		keys := make([]string, 0, len(bodyInfo.bodyVars))
		for k := range bodyInfo.bodyVars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(curl, "%s=%s\n", strings.ToUpper(k), formatVariableValue(bodyInfo.bodyVars[k]))
		}
	}
}

// buildCurlCommand builds the curl command string

// buildCurlCommand builds the curl command string
func buildCurlCommand(curl *bytes.Buffer, method, path string, pathParams []string, op *openapi3.Operation, formDataParams []string, bodyInfo requestBodyInfo) {
	urlPath := path
	for _, param := range pathParams {
		urlPath = strings.ReplaceAll(urlPath, "{"+param+"}", "${"+strings.ToUpper(param)+"}")
	}

	fmt.Fprintf(curl, "\ncurl -s -X %s \"${BASE_URL}%s", strings.ToUpper(method), urlPath)

	// Add query parameters
	if op.Parameters != nil {
		queryStrs := []string{}
		for _, paramRef := range op.Parameters {
			if paramRef.Value != nil && paramRef.Value.In == "query" {
				paramName := strings.ToUpper(paramRef.Value.Name)
				queryStrs = append(queryStrs, fmt.Sprintf("%s=${%s}", paramRef.Value.Name, paramName))
			}
		}
		if len(queryStrs) > 0 {
			fmt.Fprintf(curl, "?%s", strings.Join(queryStrs, "&"))
		}
	}

	fmt.Fprintf(curl, "\"")

	// Add headers
	if bodyInfo.contentType != "" {
		fmt.Fprintf(curl, " \\\n  -H \"Content-Type: %s\"", bodyInfo.contentType)
	}
	fmt.Fprintf(curl, " \\\n  -H \"Accept: application/json\"")

	if op.Parameters != nil {
		for _, paramRef := range op.Parameters {
			if paramRef.Value != nil && paramRef.Value.In == "header" {
				paramName := strings.ToUpper(strings.ReplaceAll(paramRef.Value.Name, "-", "_"))
				fmt.Fprintf(curl, " \\\n  -H \"%s: ${%s}\"", paramRef.Value.Name, paramName)
			}
		}
	}

	// Add form data or body
	if len(formDataParams) > 0 {
		addFormDataFields(curl, op, formDataParams)
	} else if bodyInfo.exampleBody != "" {
		fmt.Fprintf(curl, " \\\n  --data-binary @- << EOF\n%s\nEOF", bodyInfo.exampleBody)
	} else if op.RequestBody != nil {
		fmt.Fprintf(curl, " \\\n  -d '{\"foo\": \"bar\"}'")
	}

	fmt.Fprintf(curl, "\n")
}

// addFormDataFields adds form data fields to the curl command
func addFormDataFields(curl *bytes.Buffer, op *openapi3.Operation, formDataParams []string) {
	for _, paramName := range formDataParams {
		var originalName string
		if op.Parameters != nil {
			for _, paramRef := range op.Parameters {
				if paramRef.Value != nil && paramRef.Value.In == "formData" {
					if strings.ToUpper(paramRef.Value.Name) == paramName {
						originalName = paramRef.Value.Name
						break
					}
				}
			}
		}

		if originalName != "" {
			lowerName := strings.ToLower(originalName)
			if strings.Contains(lowerName, "file") || strings.Contains(lowerName, "image") || strings.Contains(lowerName, "attachment") {
				fmt.Fprintf(curl, " \\\n  -F \"%s=@${%s}\"", originalName, paramName)
			} else {
				fmt.Fprintf(curl, " \\\n  -F \"%s=${%s}\"", originalName, paramName)
			}
		}
	}
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

// extractBodyVariables extracts top-level fields from example body as variables
func extractBodyVariables(example any, prefix string) map[string]any {
	vars := make(map[string]any)

	switch v := example.(type) {
	case map[string]any:
		for key, value := range v {
			varName := key
			if prefix != "" {
				varName = prefix + "_" + key
			}
			// Only extract primitives and simple values
			switch value.(type) {
			case string, int, int64, float64, bool, nil:
				vars[varName] = value
			case map[string]any, []any:
				// Don't extract nested objects/arrays - keep them inline
				continue
			default:
				// Try to extract as string
				vars[varName] = fmt.Sprintf("%v", value)
			}
		}
	}

	return vars
}

// extractBodyVariablesFromAny extracts variables from any type (object or array)
func extractBodyVariablesFromAny(example any) map[string]any {
	switch v := example.(type) {
	case map[string]any:
		// Object - extract top-level fields
		return extractBodyVariables(v, "")
	case []any:
		// Array - extract from first item if it's an object
		if len(v) > 0 {
			if obj, ok := v[0].(map[string]any); ok {
				return extractBodyVariables(obj, "")
			}
		}
	}
	return make(map[string]any)
}

// formatVariableValue formats a value for variable assignment
func formatVariableValue(value any) string {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("\"%s\"", v)
	case bool:
		return fmt.Sprintf("\"%v\"", v)
	case nil:
		return "\"null\""
	case float64:
		// Check if it's an integer
		if v == float64(int64(v)) {
			return fmt.Sprintf("\"%d\"", int64(v))
		}
		return fmt.Sprintf("\"%v\"", v)
	default:
		return fmt.Sprintf("\"%v\"", v)
	}
}

// formatExampleWithVars formats an example body with variable substitutions
func formatExampleWithVars(example any, contentType string) string {
	// Handle arrays
	if arr, ok := example.([]any); ok {
		if len(arr) > 0 {
			// Format array with first item using variables if it's an object
			if obj, ok := arr[0].(map[string]any); ok {
				formattedItem := formatJSONWithVars(obj)
				return fmt.Sprintf("[\n%s\n]", indentString(formattedItem, "  "))
			}
		}
		// Empty array or non-object items
		data, _ := json.MarshalIndent(arr, "", "  ")
		return string(data)
	}

	// Handle maps/objects with variable substitution
	if _, ok := example.(map[string]any); ok {
		return formatJSONWithVars(example)
	}

	// For other types, marshal as JSON
	data, err := json.MarshalIndent(example, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

// indentString adds indentation to each line of a string
func indentString(s string, indent string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = indent + line
		}
	}
	return strings.Join(lines, "\n")
}

// formatJSONWithVars formats JSON with variables substituted
func formatJSONWithVars(example any) string {
	switch v := example.(type) {
	case map[string]any:
		var buf bytes.Buffer
		buf.WriteString("{\n")

		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for i, key := range keys {
			value := v[key]
			buf.WriteString(fmt.Sprintf("  \"%s\": ", key))

			// Format value with variable substitution
			switch val := value.(type) {
			case string:
				buf.WriteString(fmt.Sprintf("\"${%s}\"", strings.ToUpper(key)))
			case bool:
				buf.WriteString(fmt.Sprintf("${%s}", strings.ToUpper(key)))
			case nil:
				buf.WriteString(fmt.Sprintf("${%s}", strings.ToUpper(key)))
			case float64:
				buf.WriteString(fmt.Sprintf("${%s}", strings.ToUpper(key)))
			case int, int64:
				buf.WriteString(fmt.Sprintf("${%s}", strings.ToUpper(key)))
			case map[string]any:
				// Nested object - format inline without variables
				nested, _ := json.MarshalIndent(val, "  ", "  ")
				buf.WriteString(string(nested))
			case []any:
				// Array - format inline without variables
				arr, _ := json.MarshalIndent(val, "  ", "  ")
				buf.WriteString(string(arr))
			default:
				buf.WriteString(fmt.Sprintf("\"%v\"", val))
			}

			if i < len(keys)-1 {
				buf.WriteString(",")
			}
			buf.WriteString("\n")
		}

		buf.WriteString("}")
		return buf.String()

	case []any:
		// Array at root - just marshal it
		data, _ := json.MarshalIndent(v, "", "  ")
		return string(data)

	default:
		return "{}"
	}
}

// generateExampleFromSchema generates an example object from an OpenAPI schema
func generateExampleFromSchema(schema *openapi3.Schema, doc *openapi3.T) any {
	if schema == nil {
		return nil
	}

	// Handle array schemas
	if schema.Type != nil && schema.Type.Is("array") {
		// Generate one example item
		if schema.Items != nil && schema.Items.Value != nil {
			item := generateExampleFromSchema(schema.Items.Value, doc)
			if item != nil {
				return []any{item}
			}
		}
		return []any{}
	}

	// Handle object schemas
	if schema.Type != nil && schema.Type.Is("object") {
		example := make(map[string]any)

		// If no properties defined but it's an object, return empty example
		// This will trigger the fallback {"foo": "bar"}
		if len(schema.Properties) == 0 {
			return nil
		}

		for propName, propSchemaRef := range schema.Properties {
			if propSchemaRef == nil || propSchemaRef.Value == nil {
				continue
			}

			propSchema := propSchemaRef.Value

			// Use example if provided
			if propSchema.Example != nil {
				example[propName] = propSchema.Example
				continue
			}

			// Generate based on type
			if propSchema.Type != nil {
				if propSchema.Type.Is("string") {
					if len(propSchema.Enum) > 0 {
						example[propName] = propSchema.Enum[0]
					} else if propSchema.Default != nil {
						example[propName] = propSchema.Default
					} else {
						example[propName] = "string"
					}
				} else if propSchema.Type.Is("integer") || propSchema.Type.Is("number") {
					if propSchema.Default != nil {
						example[propName] = propSchema.Default
					} else {
						example[propName] = 0
					}
				} else if propSchema.Type.Is("boolean") {
					if propSchema.Default != nil {
						example[propName] = propSchema.Default
					} else {
						example[propName] = true
					}
				} else if propSchema.Type.Is("array") {
					// Recursively generate array
					if arrayExample := generateExampleFromSchema(propSchema, doc); arrayExample != nil {
						example[propName] = arrayExample
					} else {
						example[propName] = []any{}
					}
				} else if propSchema.Type.Is("object") {
					// Recursively generate nested object
					if nested := generateExampleFromSchema(propSchema, doc); nested != nil {
						example[propName] = nested
					} else {
						example[propName] = map[string]any{}
					}
				}
			}
		}

		if len(example) == 0 {
			return nil
		}

		return example
	}

	// Handle primitive types at root level
	if schema.Type != nil {
		if schema.Type.Is("string") {
			if schema.Example != nil {
				return schema.Example
			}
			if len(schema.Enum) > 0 {
				return schema.Enum[0]
			}
			return "string"
		} else if schema.Type.Is("integer") {
			if schema.Example != nil {
				return schema.Example
			}
			return 0
		} else if schema.Type.Is("number") {
			if schema.Example != nil {
				return schema.Example
			}
			return 0.0
		} else if schema.Type.Is("boolean") {
			if schema.Example != nil {
				return schema.Example
			}
			return true
		}
	}

	return nil
}
