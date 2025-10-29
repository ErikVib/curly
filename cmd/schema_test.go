package cmd

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestExtractRequestParameters(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		op         *openapi3.Operation
		wantPath   int
		wantQuery  int
		wantHeader int
		wantForm   int
	}{
		{
			name: "no parameters",
			path: "/users",
			op: &openapi3.Operation{
				Parameters: nil,
			},
			wantPath:   0,
			wantQuery:  0,
			wantHeader: 0,
			wantForm:   0,
		},
		{
			name: "path parameters only",
			path: "/users/{userId}/posts/{postId}",
			op: &openapi3.Operation{
				Parameters: nil,
			},
			wantPath:   2,
			wantQuery:  0,
			wantHeader: 0,
			wantForm:   0,
		},
		{
			name: "query parameters",
			path: "/users",
			op: &openapi3.Operation{
				Parameters: openapi3.Parameters{
					&openapi3.ParameterRef{
						Value: &openapi3.Parameter{
							Name: "limit",
							In:   "query",
						},
					},
					&openapi3.ParameterRef{
						Value: &openapi3.Parameter{
							Name: "offset",
							In:   "query",
						},
					},
				},
			},
			wantPath:   0,
			wantQuery:  2,
			wantHeader: 0,
			wantForm:   0,
		},
		{
			name: "header parameters",
			path: "/users",
			op: &openapi3.Operation{
				Parameters: openapi3.Parameters{
					&openapi3.ParameterRef{
						Value: &openapi3.Parameter{
							Name: "Authorization",
							In:   "header",
						},
					},
					&openapi3.ParameterRef{
						Value: &openapi3.Parameter{
							Name: "X-API-Key",
							In:   "header",
						},
					},
				},
			},
			wantPath:   0,
			wantQuery:  0,
			wantHeader: 2,
			wantForm:   0,
		},
		{
			name: "form data parameters",
			path: "/upload",
			op: &openapi3.Operation{
				Parameters: openapi3.Parameters{
					&openapi3.ParameterRef{
						Value: &openapi3.Parameter{
							Name: "file",
							In:   "formData",
						},
					},
					&openapi3.ParameterRef{
						Value: &openapi3.Parameter{
							Name: "description",
							In:   "formData",
						},
					},
				},
			},
			wantPath:   0,
			wantQuery:  0,
			wantHeader: 0,
			wantForm:   2,
		},
		{
			name: "mixed parameters",
			path: "/users/{id}",
			op: &openapi3.Operation{
				Parameters: openapi3.Parameters{
					&openapi3.ParameterRef{
						Value: &openapi3.Parameter{
							Name: "format",
							In:   "query",
						},
					},
					&openapi3.ParameterRef{
						Value: &openapi3.Parameter{
							Name: "Authorization",
							In:   "header",
						},
					},
				},
			},
			wantPath:   1,
			wantQuery:  1,
			wantHeader: 1,
			wantForm:   0,
		},
		{
			name: "nil parameter values",
			path: "/users",
			op: &openapi3.Operation{
				Parameters: openapi3.Parameters{
					&openapi3.ParameterRef{
						Value: nil,
					},
					&openapi3.ParameterRef{
						Value: &openapi3.Parameter{
							Name: "limit",
							In:   "query",
						},
					},
				},
			},
			wantPath:   0,
			wantQuery:  1,
			wantHeader: 0,
			wantForm:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRequestParameters(tt.path, tt.op, nil)

			if len(result.pathParams) != tt.wantPath {
				t.Errorf("pathParams count = %d, want %d", len(result.pathParams), tt.wantPath)
			}
			if len(result.queryParams) != tt.wantQuery {
				t.Errorf("queryParams count = %d, want %d", len(result.queryParams), tt.wantQuery)
			}
			if len(result.headerParams) != tt.wantHeader {
				t.Errorf("headerParams count = %d, want %d", len(result.headerParams), tt.wantHeader)
			}
			if len(result.formDataParams) != tt.wantForm {
				t.Errorf("formDataParams count = %d, want %d", len(result.formDataParams), tt.wantForm)
			}
		})
	}
}

func TestExtractRequestBody(t *testing.T) {
	tests := []struct {
		name            string
		op              *openapi3.Operation
		wantContentType string
		wantHasBody     bool
	}{
		{
			name: "no request body",
			op: &openapi3.Operation{
				RequestBody: nil,
			},
			wantContentType: "",
			wantHasBody:     false,
		},
		{
			name: "request body with example",
			op: &openapi3.Operation{
				RequestBody: &openapi3.RequestBodyRef{
					Value: &openapi3.RequestBody{
						Content: openapi3.Content{
							"application/json": &openapi3.MediaType{
								Example: map[string]interface{}{
									"name": "John",
									"age":  30,
								},
							},
						},
					},
				},
			},
			wantContentType: "application/json",
			wantHasBody:     true,
		},
		{
			name: "request body with examples",
			op: &openapi3.Operation{
				RequestBody: &openapi3.RequestBodyRef{
					Value: &openapi3.RequestBody{
						Content: openapi3.Content{
							"application/json": &openapi3.MediaType{
								Examples: map[string]*openapi3.ExampleRef{
									"example1": {
										Value: &openapi3.Example{
											Value: map[string]interface{}{
												"id": 123,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantContentType: "application/json",
			wantHasBody:     true,
		},
		{
			name: "request body with schema",
			op: &openapi3.Operation{
				RequestBody: &openapi3.RequestBodyRef{
					Value: &openapi3.RequestBody{
						Content: openapi3.Content{
							"application/json": &openapi3.MediaType{
								Schema: &openapi3.SchemaRef{
									Value: &openapi3.Schema{
										Type: &openapi3.Types{"object"},
										Properties: openapi3.Schemas{
											"name": &openapi3.SchemaRef{
												Value: &openapi3.Schema{
													Type: &openapi3.Types{"string"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantContentType: "application/json",
			wantHasBody:     true,
		},
		{
			name: "Swagger 2.0 style body parameter",
			op: &openapi3.Operation{
				Parameters: openapi3.Parameters{
					&openapi3.ParameterRef{
						Value: &openapi3.Parameter{
							Name: "body",
							In:   "body",
							Schema: &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type: &openapi3.Types{"object"},
									Properties: openapi3.Schemas{
										"email": &openapi3.SchemaRef{
											Value: &openapi3.Schema{
												Type: &openapi3.Types{"string"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			wantContentType: "application/json",
			wantHasBody:     true,
		},
		{
			name: "multiple content types - first wins",
			op: &openapi3.Operation{
				RequestBody: &openapi3.RequestBodyRef{
					Value: &openapi3.RequestBody{
						Content: openapi3.Content{
							"application/xml": &openapi3.MediaType{
								Example: map[string]interface{}{
									"data": "test",
								},
							},
							"application/json": &openapi3.MediaType{
								Example: map[string]interface{}{
									"data": "test2",
								},
							},
						},
					},
				},
			},
			wantHasBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRequestBody(tt.op, nil)

			if tt.wantContentType != "" && result.contentType != tt.wantContentType {
				t.Errorf("contentType = %q, want %q", result.contentType, tt.wantContentType)
			}

			hasBody := result.exampleBody != ""
			if hasBody != tt.wantHasBody {
				t.Errorf("hasBody = %v, want %v", hasBody, tt.wantHasBody)
			}
		})
	}
}

func TestGenerateExampleFromSchema(t *testing.T) {
	tests := []struct {
		name     string
		schema   *openapi3.Schema
		wantNil  bool
		wantType string
	}{
		{
			name:    "nil schema",
			schema:  nil,
			wantNil: true,
		},
		{
			name: "object schema with properties",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				Properties: openapi3.Schemas{
					"name": &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type: &openapi3.Types{"string"},
						},
					},
					"age": &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type: &openapi3.Types{"integer"},
						},
					},
				},
			},
			wantNil:  false,
			wantType: "object",
		},
		{
			name: "array schema",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"array"},
				Items: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"object"},
						Properties: openapi3.Schemas{
							"id": &openapi3.SchemaRef{
								Value: &openapi3.Schema{
									Type: &openapi3.Types{"integer"},
								},
							},
						},
					},
				},
			},
			wantNil:  false,
			wantType: "array",
		},
		{
			name: "string schema",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"string"},
			},
			wantNil:  false,
			wantType: "string",
		},
		{
			name: "integer schema",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"integer"},
			},
			wantNil:  false,
			wantType: "number",
		},
		{
			name: "boolean schema",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"boolean"},
			},
			wantNil:  false,
			wantType: "boolean",
		},
		{
			name: "string with enum",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"string"},
				Enum: []interface{}{"active", "inactive"},
			},
			wantNil:  false,
			wantType: "string",
		},
		{
			name: "string with default",
			schema: &openapi3.Schema{
				Type:    &openapi3.Types{"string"},
				Default: "default-value",
			},
			wantNil:  false,
			wantType: "string",
		},
		{
			name: "object with no properties",
			schema: &openapi3.Schema{
				Type:       &openapi3.Types{"object"},
				Properties: openapi3.Schemas{},
			},
			wantNil: true,
		},
		{
			name: "nested object",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"object"},
				Properties: openapi3.Schemas{
					"user": &openapi3.SchemaRef{
						Value: &openapi3.Schema{
							Type: &openapi3.Types{"object"},
							Properties: openapi3.Schemas{
								"name": &openapi3.SchemaRef{
									Value: &openapi3.Schema{
										Type: &openapi3.Types{"string"},
									},
								},
							},
						},
					},
				},
			},
			wantNil:  false,
			wantType: "object",
		},
		{
			name: "array of primitives",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"array"},
				Items: &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type: &openapi3.Types{"string"},
					},
				},
			},
			wantNil:  false,
			wantType: "array",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateExampleFromSchema(tt.schema, nil)

			if tt.wantNil && result != nil {
				t.Errorf("generateExampleFromSchema() = %v, want nil", result)
			}

			if !tt.wantNil && result == nil {
				t.Errorf("generateExampleFromSchema() = nil, want non-nil")
			}

			if result != nil && tt.wantType != "" {
				switch tt.wantType {
				case "object":
					if _, ok := result.(map[string]interface{}); !ok {
						t.Errorf("result type = %T, want map[string]interface{}", result)
					}
				case "array":
					if _, ok := result.([]interface{}); !ok {
						t.Errorf("result type = %T, want []interface{}", result)
					}
				case "string":
					if _, ok := result.(string); !ok {
						t.Errorf("result type = %T, want string", result)
					}
				case "number":
					switch result.(type) {
					case int, int64, float64:
						// OK
					default:
						t.Errorf("result type = %T, want numeric type", result)
					}
				case "boolean":
					if _, ok := result.(bool); !ok {
						t.Errorf("result type = %T, want bool", result)
					}
				}
			}
		})
	}
}

func TestGenerateExampleFromSchemaValues(t *testing.T) {
	tests := []struct {
		name      string
		schema    *openapi3.Schema
		wantValue interface{}
	}{
		{
			name: "string with example",
			schema: &openapi3.Schema{
				Type:    &openapi3.Types{"string"},
				Example: "example-value",
			},
			wantValue: "example-value",
		},
		{
			name: "integer with example",
			schema: &openapi3.Schema{
				Type:    &openapi3.Types{"integer"},
				Example: 42,
			},
			wantValue: 42,
		},
		{
			name: "boolean with example",
			schema: &openapi3.Schema{
				Type:    &openapi3.Types{"boolean"},
				Example: false,
			},
			wantValue: false,
		},
		{
			name: "enum first value",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"string"},
				Enum: []interface{}{"active", "inactive", "pending"},
			},
			wantValue: "active",
		},
		{
			name: "integer default value",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"integer"},
			},
			wantValue: 0,
		},
		{
			name: "boolean default value",
			schema: &openapi3.Schema{
				Type: &openapi3.Types{"boolean"},
			},
			wantValue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateExampleFromSchema(tt.schema, nil)

			if result == nil {
				t.Fatal("generateExampleFromSchema() returned nil")
			}

			if result != tt.wantValue {
				t.Errorf("generateExampleFromSchema() = %v, want %v", result, tt.wantValue)
			}
		})
	}
}

func TestExtractBodyVariables(t *testing.T) {
	tests := []struct {
		name      string
		example   interface{}
		prefix    string
		wantCount int
		wantKeys  []string
	}{
		{
			name: "simple object",
			example: map[string]interface{}{
				"name":  "John",
				"email": "john@example.com",
			},
			prefix:    "",
			wantCount: 2,
			wantKeys:  []string{"name", "email"},
		},
		{
			name: "object with nested objects (should skip)",
			example: map[string]interface{}{
				"name": "John",
				"address": map[string]interface{}{
					"street": "123 Main St",
				},
			},
			prefix:    "",
			wantCount: 1,
			wantKeys:  []string{"name"},
		},
		{
			name: "object with arrays (should skip)",
			example: map[string]interface{}{
				"name": "John",
				"tags": []interface{}{"admin", "user"},
			},
			prefix:    "",
			wantCount: 1,
			wantKeys:  []string{"name"},
		},
		{
			name: "object with different types",
			example: map[string]interface{}{
				"name":   "John",
				"age":    30,
				"active": true,
				"score":  95.5,
			},
			prefix:    "",
			wantCount: 4,
			wantKeys:  []string{"name", "age", "active", "score"},
		},
		{
			name:      "non-object input",
			example:   "just a string",
			prefix:    "",
			wantCount: 0,
		},
		{
			name:      "empty object",
			example:   map[string]interface{}{},
			prefix:    "",
			wantCount: 0,
		},
		{
			name: "with prefix",
			example: map[string]interface{}{
				"name": "John",
			},
			prefix:    "user",
			wantCount: 1,
			wantKeys:  []string{"user_name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBodyVariables(tt.example, tt.prefix)

			if len(result) != tt.wantCount {
				t.Errorf("extractBodyVariables() returned %d vars, want %d", len(result), tt.wantCount)
			}

			for _, key := range tt.wantKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("extractBodyVariables() missing key %q", key)
				}
			}
		})
	}
}

func TestExtractBodyVariablesFromAny(t *testing.T) {
	tests := []struct {
		name      string
		example   interface{}
		wantCount int
	}{
		{
			name: "object",
			example: map[string]interface{}{
				"name": "John",
				"age":  30,
			},
			wantCount: 2,
		},
		{
			name: "array with object",
			example: []interface{}{
				map[string]interface{}{
					"id":   1,
					"name": "Item 1",
				},
			},
			wantCount: 2,
		},
		{
			name: "array with primitive",
			example: []interface{}{
				"string1", "string2",
			},
			wantCount: 0,
		},
		{
			name:      "empty array",
			example:   []interface{}{},
			wantCount: 0,
		},
		{
			name:      "primitive value",
			example:   "just a string",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractBodyVariablesFromAny(tt.example)

			if len(result) != tt.wantCount {
				t.Errorf("extractBodyVariablesFromAny() returned %d vars, want %d", len(result), tt.wantCount)
			}
		})
	}
}

func TestFormatVariableValue(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  string
	}{
		{
			name:  "string value",
			value: "test",
			want:  `"test"`,
		},
		{
			name:  "integer as float64",
			value: float64(42),
			want:  `"42"`,
		},
		{
			name:  "float value",
			value: 3.14,
			want:  `"3.14"`,
		},
		{
			name:  "boolean true",
			value: true,
			want:  `"true"`,
		},
		{
			name:  "boolean false",
			value: false,
			want:  `"false"`,
		},
		{
			name:  "nil value",
			value: nil,
			want:  `"null"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatVariableValue(tt.value)

			if result != tt.want {
				t.Errorf("formatVariableValue(%v) = %q, want %q", tt.value, result, tt.want)
			}
		})
	}
}
