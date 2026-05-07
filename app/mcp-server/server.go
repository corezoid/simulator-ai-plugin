package mcpserver

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"git.corezoid.com/mw161089sar/swagger-mcp/app/auth"
	"git.corezoid.com/mw161089sar/swagger-mcp/app/models"
	"git.corezoid.com/mw161089sar/swagger-mcp/app/swagger"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const sseHeadersKey = "__sseHeadersKey"

func ExtractSchemaName(ref, schemaType string) string {
	if ref != "" {
		parts := strings.Split(ref, "/")
		return parts[len(parts)-1]
	}
	return schemaType
}

// getDefinition gets a definition from either components.schemas (OpenAPI 3.0+) or definitions (Swagger 2.0)
func getDefinition(swaggerSpec models.SwaggerSpec, schemaName string) (models.Definition, bool) {
	// Try OpenAPI 3.0+ components first
	if swaggerSpec.Components != nil {
		if definition, found := swaggerSpec.Components.Schemas[schemaName]; found {
			return definition, true
		}
	}
	// Fall back to Swagger 2.0 definitions
	if definition, found := swaggerSpec.Definitions[schemaName]; found {
		return definition, true
	}

	return models.Definition{}, false
}

// resolveParameterRef resolves a $ref parameter reference
func resolveParameterRef(swaggerSpec models.SwaggerSpec, ref string) (models.Parameter, bool) {
	// Handle internal refs like "#/paths/~1download~1zip~1%7BaccId%7D/post/parameters/1"
	if strings.HasPrefix(ref, "#/") {
		parts := strings.Split(ref, "/")
		if len(parts) >= 6 && parts[1] == "paths" {
			// Decode URL-encoded path
			pathKey := strings.ReplaceAll(parts[2], "~1", "/")
			pathKey = strings.ReplaceAll(pathKey, "%7B", "{")
			pathKey = strings.ReplaceAll(pathKey, "%7D", "}")

			method := parts[3]
			if parts[4] == "parameters" {
				// Get parameter by index
				if paramIdx, err := strconv.Atoi(parts[5]); err == nil {
					if pathMethods, exists := swaggerSpec.Paths[pathKey]; exists {
						if endpoint, exists := pathMethods[method]; exists {
							if paramIdx < len(endpoint.Parameters) {
								return endpoint.Parameters[paramIdx], true
							}
						}
					}
				}
			}
		}
	}

	return models.Parameter{}, false
}

func compileRegexes(paths string) []*regexp.Regexp {
	var regexes []*regexp.Regexp
	for _, path := range strings.Split(paths, ",") {
		if path = strings.TrimSpace(path); path != "" {
			regex, err := regexp.Compile(path)
			if err != nil {
				log.Printf("Invalid regex pattern: %s, error: %v", path, err)
				continue
			}
			regexes = append(regexes, regex)
		}
	}
	return regexes
}

func shouldIncludePath(path string, includeRegexes, excludeRegexes []*regexp.Regexp) bool {
	// If no include regexes are specified, include all paths by default
	include := len(includeRegexes) == 0

	for _, regex := range includeRegexes {
		if regex.MatchString(path) {
			include = true
			break
		}
	}

	if !include {
		return false
	}

	for _, regex := range excludeRegexes {
		if regex.MatchString(path) {
			return false
		}
	}

	return true
}

func shouldIncludeMethod(method string, includeMethods, excludeMethods []string) bool {
	// If no include methods are specified, include all methods by default
	include := len(includeMethods) == 0

	for _, m := range includeMethods {
		if strings.EqualFold(strings.TrimSpace(m), method) {
			include = true
			break
		}
	}

	if !include {
		return false
	}

	for _, m := range excludeMethods {
		if strings.EqualFold(strings.TrimSpace(m), method) {
			return false
		}
	}

	return true
}

func CreateServer(swaggerSpec models.SwaggerSpec, config models.Config) {
	mcpServer := server.NewMCPServer(
		"swagegr-mcp",
		"1.0.0",
	)

	LoadSwaggerServer(mcpServer, swaggerSpec, config.ApiCfg)

	if config.SseCfg.SseMode {
		// Create and start SSE server
		sseServer := server.NewSSEServer(mcpServer, server.WithBaseURL(config.SseCfg.SseUrl), server.WithSSEContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			if len(config.ApiCfg.SseHeaders) == 0 {
				return ctx
			}
			keys := strings.Split(config.ApiCfg.SseHeaders, ",")
			sseHeaders := map[string]string{}
			for _, key := range keys {
				sseHeaders[key] = r.Header.Get(key)
			}
			return context.WithValue(ctx, sseHeadersKey, sseHeaders)
		}))
		endpoint, err := sseServer.CompleteSseEndpoint()
		if err != nil {
			log.Fatalf("Error creating SSE endpoint: %v", err)
		}
		log.Printf("Starting SSE server on %s, endpoint: %s", config.SseCfg.SseAddr, endpoint)
		if err := sseServer.Start(config.SseCfg.SseAddr); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	} else {
		// Run as stdio server
		if err := server.ServeStdio(mcpServer); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}
}

// CreateMultiServiceServer creates MCP server for multiple services
func CreateMultiServiceServer(services []models.ServiceConfig, config models.Config) {
	mcpServer := server.NewMCPServer(
		"swagger-mcp-multi",
		"1.0.0",
	)

	// Load all services into the MCP server
	LoadMultipleSwaggerServices(mcpServer, services)

	if config.SseCfg.SseMode {
		// Create and start SSE server with potential headers from any service
		sseServer := server.NewSSEServer(mcpServer, server.WithBaseURL(config.SseCfg.SseUrl), server.WithSSEContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			// Get headers from the first service that has SseHeaders configured
			var sseHeadersConfig string
			for _, service := range services {
				if service.ApiCfg.SseHeaders != "" {
					sseHeadersConfig = service.ApiCfg.SseHeaders
					break
				}
			}

			if sseHeadersConfig == "" {
				return ctx
			}
			keys := strings.Split(sseHeadersConfig, ",")
			sseHeaders := map[string]string{}
			for _, key := range keys {
				sseHeaders[key] = r.Header.Get(key)
			}
			return context.WithValue(ctx, sseHeadersKey, sseHeaders)
		}))
		endpoint, err := sseServer.CompleteSseEndpoint()
		if err != nil {
			log.Fatalf("Error creating SSE endpoint: %v", err)
		}
		log.Printf("Starting SSE server on %s, endpoint: %s", config.SseCfg.SseAddr, endpoint)
		if err := sseServer.Start(config.SseCfg.SseAddr); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	} else {
		// Run as stdio server
		if err := server.ServeStdio(mcpServer); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}
}

type Operation struct {
	ID          string      `json:"id"`
	Description string      `json:"description"`
	Method      string      `json:"method"`
	Path        string      `json:"path"`
	Summary     string      `json:"summary"`
	URL         string      `json:"url"`
	Parameters  []Parameter `json:"parameters"`
	RequestBody interface{} `json:"request_body"`
	Responses   interface{} `json:"responses"`
}

type Parameter struct {
	Name        string      `json:"name"`
	In          string      `json:"in"`
	TIn         string      `json:"-"`
	Required    bool        `json:"required"`
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Schema      interface{} `json:"schema"`
}

var globalOperations []Operation
var globalSwaggerSpec models.SwaggerSpec
var globalApiConfig models.ApiConfig
var globalServiceConfigs map[string]models.ApiConfig
var globalMCPServer *server.MCPServer
var globalOAuthClientID string

// operationScore is used for scoring search results
type operationScore struct {
	operation Operation
	score     float64
}

func LoadSwaggerServer(mcpServer *server.MCPServer, swaggerSpec models.SwaggerSpec, apiCfg models.ApiConfig) {
	globalOperations = buildOperations(swaggerSpec, apiCfg)
	globalSwaggerSpec = swaggerSpec
	globalApiConfig = apiCfg
	globalMCPServer = mcpServer

	// Resolve OAuth client ID: flag > env var > built-in default
	globalOAuthClientID = apiCfg.OAuthClientID
	if globalOAuthClientID == "" {
		globalOAuthClientID = os.Getenv("SIMULATOR_OAUTH_CLIENT_ID")
	}
	if globalOAuthClientID == "" {
		globalOAuthClientID = auth.DefaultClientID
	}

	// If no auth token is set, try to load from saved credentials
	if globalApiConfig.Authorization == "" {
		if creds, err := auth.Load(); err == nil && creds != nil && !auth.IsExpired(creds) {
			globalApiConfig.Authorization = creds.AuthorizationHeader()
			log.Printf("Loaded auth token from saved credentials (expires %s)", creds.ExpiresAt.Format("2006-01-02 15:04"))
		}
	}

	mcpServer.AddTool(
		mcp.NewTool("get_oper",
			mcp.WithString("id",
				mcp.Description("Operation id to get the full schema"),
				mcp.Required(),
			),
			mcp.WithDescription("Get the full schema of an API operation by id."),
		),
		handleGetOper,
	)

	mcpServer.AddTool(
		mcp.NewTool("run_oper",
			mcp.WithString("id",
				mcp.Description("Operation id to execute"),
				mcp.Required(),
			),
			mcp.WithString("query",
				mcp.Description("Path and query parameters as JSON object"),
			),
			mcp.WithString("header",
				mcp.Description("Header parameters as JSON object"),
			),
			mcp.WithString("body",
				mcp.Description("Request body as JSON object"),
			),
			mcp.WithDescription("Execute an API operation with the provided parameters."),
		),
		handleRunOper,
	)

	mcpServer.AddTool(
		mcp.NewTool("list_opers",
			mcp.WithDescription("List all available API operations with their ID and summary."),
		),
		handleListOpers,
	)

	mcpServer.AddTool(
		mcp.NewTool("login",
			mcp.WithDescription("Authenticate with Simulator via OAuth2 browser flow. Saves the token so it persists across sessions."),
		),
		handleLogin,
	)

	// Add MCP resources capability
	initializeResources(mcpServer)
}

// LoadMultipleSwaggerServices loads multiple services into a single MCP server
func LoadMultipleSwaggerServices(mcpServer *server.MCPServer, services []models.ServiceConfig) {
	var allOperations []Operation

	// Load and merge operations from all services
	for _, service := range services {
		// Load swagger spec for this service
		swaggerSpec, err := swagger.LoadSwagger(service.SpecUrl)
		if err != nil {
			log.Fatalf("Failed to load swagger spec for service %s: %v", service.Name, err)
		}

		// Build operations for this service, with service name prefix
		serviceOps := buildOperationsWithServicePrefix(service.Name, swaggerSpec, service.ApiCfg)
		allOperations = append(allOperations, serviceOps...)

		log.Printf("Loaded %d operations from service: %s", len(serviceOps), service.Name)
	}

	// Set global operations to combined list
	globalOperations = allOperations

	mcpServer.AddTool(
		mcp.NewTool("get_oper",
			mcp.WithString("id",
				mcp.Description("Operation id to get the full schema (format: serviceName:METHOD:path)"),
				mcp.Required(),
			),
			mcp.WithDescription("Get the full schema of an API operation by id from any service."),
		),
		handleGetOper,
	)

	mcpServer.AddTool(
		mcp.NewTool("run_oper",
			mcp.WithString("id",
				mcp.Description("Operation id to execute (format: serviceName:METHOD:path)"),
				mcp.Required(),
			),
			mcp.WithString("query",
				mcp.Description("Path and query parameters as JSON object"),
			),
			mcp.WithString("header",
				mcp.Description("Header parameters as JSON object"),
			),
			mcp.WithString("body",
				mcp.Description("Request body as JSON object"),
			),
			mcp.WithDescription("Execute an API operation from any service with the provided parameters."),
		),
		handleRunOper,
	)

	mcpServer.AddTool(
		mcp.NewTool("list_opers",
			mcp.WithDescription("List all available API operations from all services with their ID, service name and summary."),
		),
		handleListOpers,
	)

	// Add service-specific information tool
	mcpServer.AddTool(
		mcp.NewTool("list_services",
			mcp.WithDescription("List all configured services with their names and operation counts."),
		),
		handleListServices,
	)

	// Add MCP resources capability
	initializeResources(mcpServer)

	log.Printf("Successfully loaded %d total operations from %d services", len(allOperations), len(services))
}

func buildOperations(swaggerSpec models.SwaggerSpec, apiCfg models.ApiConfig) []Operation {
	includeRegexes := compileRegexes(apiCfg.IncludePaths)
	excludeRegexes := compileRegexes(apiCfg.ExcludePaths)
	includedMethods := []string{}
	if len(strings.TrimSpace(apiCfg.IncludeMethods)) > 0 {
		includedMethods = strings.Split(apiCfg.IncludeMethods, ",")
	}
	excludedMethods := []string{}
	if len(strings.TrimSpace(apiCfg.ExcludeMethods)) > 0 {
		excludedMethods = strings.Split(apiCfg.ExcludeMethods, ",")
	}

	var operations []Operation
	operationID := 0

	for path, methods := range swaggerSpec.Paths {
		if !shouldIncludePath(path, includeRegexes, excludeRegexes) {
			continue
		}

		for method, details := range methods {
			if !shouldIncludeMethod(method, includedMethods, excludedMethods) {
				continue
			}

			var baseURL string
			if apiCfg.Url != "" {
				// Use the --url parameter with highest priority
				baseURL = apiCfg.Url
			} else if apiCfg.BaseUrl != "" {
				// Use the --baseUrl parameter with second priority
				baseURL = apiCfg.BaseUrl
			} else {
				// Fall back to extracting from Swagger spec
				if swaggerSpec.OpenAPI != "" {
					if len(swaggerSpec.Servers) > 0 {
						baseURL = strings.TrimSuffix(swaggerSpec.Servers[0].URL, "/")
					} else {
						baseURL = "/"
					}
				} else {
					baseURL = swaggerSpec.Host
					if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
						baseURL = "https://" + baseURL
					}
					if swaggerSpec.BasePath != "" {
						baseURL = strings.TrimSuffix(baseURL, "/") + "/" + strings.TrimPrefix(swaggerSpec.BasePath, "/")
					}
				}
			}

			reqURL := strings.TrimSuffix(baseURL, "/") + "/" + strings.TrimPrefix(path, "/")

			var parameters []Parameter
			for _, param := range details.Parameters {
				// Check if this is a $ref parameter
				if param.Ref != "" {
					// Resolve the $ref
					if resolvedParam, found := resolveParameterRef(swaggerSpec, param.Ref); found {
						// Skip Authorization header parameters
						if resolvedParam.In == "header" && strings.ToLower(resolvedParam.Name) == "authorization" {
							continue
						}

						in := resolvedParam.In
						if in == "path" {
							in = "query"
						}

						// Get type from schema if not directly specified
						paramType := resolvedParam.Type
						if paramType == "" && resolvedParam.Schema != nil {
							paramType = resolvedParam.Schema.Type
						}

						parameters = append(parameters, Parameter{
							Name:        resolvedParam.Name,
							In:          in,
							TIn:         resolvedParam.In,
							Required:    resolvedParam.Required,
							Type:        paramType,
							Description: resolvedParam.Description,
							Schema:      resolvedParam.Schema,
						})
					}
				} else {
					// Skip Authorization header parameters
					if param.In == "header" && strings.ToLower(param.Name) == "authorization" {
						continue
					}

					// Regular parameter
					in := param.In
					if in == "path" {
						in = "query"
					}

					// Get type from schema if not directly specified
					paramType := param.Type
					if paramType == "" && param.Schema != nil {
						paramType = param.Schema.Type
					}

					parameters = append(parameters, Parameter{
						Name:        param.Name,
						In:          in,
						TIn:         param.In,
						Required:    param.Required,
						Type:        paramType,
						Description: param.Description,
						Schema:      param.Schema,
					})
				}
			}

			var requestBody interface{}
			if details.RequestBody != nil {
				requestBody = details.RequestBody
			}

			operationID++
			path1 := strings.ReplaceAll(path, "{", "")
			path1 = strings.ReplaceAll(path1, "}", "")
			operations = append(operations, Operation{

				ID:          fmt.Sprintf("%s:%s", strings.ToUpper(method), path1),
				Description: details.Description,
				Method:      method,
				Path:        path1,
				Summary:     details.Summary,
				URL:         reqURL,
				Parameters:  parameters,
				RequestBody: requestBody,
				Responses:   details.Responses,
			})
		}
	}

	return operations
}

// buildOperationsWithServicePrefix builds operations with service name prefixed to operation IDs
func buildOperationsWithServicePrefix(serviceName string, swaggerSpec models.SwaggerSpec, apiCfg models.ApiConfig) []Operation {
	includeRegexes := compileRegexes(apiCfg.IncludePaths)
	excludeRegexes := compileRegexes(apiCfg.ExcludePaths)
	includedMethods := []string{}
	if len(strings.TrimSpace(apiCfg.IncludeMethods)) > 0 {
		includedMethods = strings.Split(apiCfg.IncludeMethods, ",")
	}
	excludedMethods := []string{}
	if len(strings.TrimSpace(apiCfg.ExcludeMethods)) > 0 {
		excludedMethods = strings.Split(apiCfg.ExcludeMethods, ",")
	}

	var operations []Operation
	operationID := 0

	for path, methods := range swaggerSpec.Paths {
		if !shouldIncludePath(path, includeRegexes, excludeRegexes) {
			continue
		}

		for method, details := range methods {
			if !shouldIncludeMethod(method, includedMethods, excludedMethods) {
				continue
			}

			var baseURL string
			if apiCfg.Url != "" {
				// Use the --url parameter with highest priority
				baseURL = apiCfg.Url
			} else if apiCfg.BaseUrl != "" {
				// Use the --baseUrl parameter with second priority
				baseURL = apiCfg.BaseUrl
			} else {
				// Fall back to extracting from Swagger spec
				if swaggerSpec.OpenAPI != "" {
					if len(swaggerSpec.Servers) > 0 {
						baseURL = strings.TrimSuffix(swaggerSpec.Servers[0].URL, "/")
					} else {
						baseURL = "/"
					}
				} else {
					baseURL = swaggerSpec.Host
					if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
						baseURL = "https://" + baseURL
					}
					if swaggerSpec.BasePath != "" {
						baseURL = strings.TrimSuffix(baseURL, "/") + "/" + strings.TrimPrefix(swaggerSpec.BasePath, "/")
					}
				}
			}

			reqURL := strings.TrimSuffix(baseURL, "/") + "/" + strings.TrimPrefix(path, "/")

			var parameters []Parameter
			for _, param := range details.Parameters {
				// Check if this is a $ref parameter
				if param.Ref != "" {
					// Resolve the $ref
					if resolvedParam, found := resolveParameterRef(swaggerSpec, param.Ref); found {
						// Skip Authorization header parameters
						if resolvedParam.In == "header" && strings.ToLower(resolvedParam.Name) == "authorization" {
							continue
						}

						in := resolvedParam.In
						if in == "path" {
							in = "query"
						}

						// Get type from schema if not directly specified
						paramType := resolvedParam.Type
						if paramType == "" && resolvedParam.Schema != nil {
							paramType = resolvedParam.Schema.Type
						}

						parameters = append(parameters, Parameter{
							Name:        resolvedParam.Name,
							In:          in,
							TIn:         resolvedParam.In,
							Required:    resolvedParam.Required,
							Type:        paramType,
							Description: resolvedParam.Description,
							Schema:      resolvedParam.Schema,
						})
					}
				} else {
					// Skip Authorization header parameters
					if param.In == "header" && strings.ToLower(param.Name) == "authorization" {
						continue
					}

					// Regular parameter
					in := param.In
					if in == "path" {
						in = "query"
					}

					// Get type from schema if not directly specified
					paramType := param.Type
					if paramType == "" && param.Schema != nil {
						paramType = param.Schema.Type
					}

					parameters = append(parameters, Parameter{
						Name:        param.Name,
						In:          in,
						TIn:         param.In,
						Required:    param.Required,
						Type:        paramType,
						Description: param.Description,
						Schema:      param.Schema,
					})
				}
			}

			var requestBody interface{}
			if details.RequestBody != nil {
				requestBody = details.RequestBody
			}

			operationID++
			path1 := strings.ReplaceAll(path, "{", "")
			path1 = strings.ReplaceAll(path1, "}", "")

			// Add service name prefix to operation ID
			operations = append(operations, Operation{
				ID:          fmt.Sprintf("%s:%s:%s", serviceName, strings.ToUpper(method), path1),
				Description: details.Description,
				Method:      method,
				Path:        path1,
				Summary:     details.Summary,
				URL:         reqURL,
				Parameters:  parameters,
				RequestBody: requestBody,
				Responses:   details.Responses,
			})
		}
	}

	return operations
}

// handleListServices handles list_services tool requests
func handleListServices(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	serviceMap := make(map[string]int)

	// Count operations by service
	for _, op := range globalOperations {
		parts := strings.Split(op.ID, ":")
		if len(parts) >= 3 {
			serviceName := parts[0]
			serviceMap[serviceName]++
		}
	}

	// Create response
	var services []map[string]interface{}
	for serviceName, operationCount := range serviceMap {
		services = append(services, map[string]interface{}{
			"name":            serviceName,
			"operation_count": operationCount,
		})
	}

	result, _ := json.Marshal(services)
	return mcp.NewToolResultText(string(result)), nil
}

// checkIfRequestBodyIsArray checks if the request body schema requires an array
func checkIfRequestBodyIsArray(op Operation) bool {
	log.Printf("DEBUG: checkIfRequestBodyIsArray called for operation %s", op.ID)

	if op.RequestBody == nil {
		log.Printf("DEBUG: RequestBody is nil")
		return false
	}

	// First try to cast to *models.RequestBody
	if requestBody, ok := op.RequestBody.(*models.RequestBody); ok {
		log.Printf("DEBUG: RequestBody is *models.RequestBody")

		// Check application/json content type
		if mediaType, exists := requestBody.Content["application/json"]; exists {
			log.Printf("DEBUG: found application/json content type")

			if mediaType.Schema != nil {
				log.Printf("DEBUG: schema found in media type")

				// Check if schema type is array
				if mediaType.Schema.Type == "array" {
					log.Printf("DEBUG: schema type is array - returning true")
					return true
				}

				// Check if schema has a reference to an array definition
				if mediaType.Schema.Ref != "" {
					log.Printf("DEBUG: schema has $ref: %s", mediaType.Schema.Ref)
					schemaName := ExtractSchemaName(mediaType.Schema.Ref, "")
					if definition, found := getDefinition(globalSwaggerSpec, schemaName); found {
						log.Printf("DEBUG: found definition %s with type: %s", schemaName, definition.Type)
						return definition.Type == "array"
					}
				}
			}
		}
	}

	// Fallback: try to cast to map[string]interface{}
	requestBodyMap, ok := op.RequestBody.(map[string]interface{})
	if !ok {
		log.Printf("DEBUG: RequestBody is neither *models.RequestBody nor map[string]interface{}")
		return false
	}

	// Check the content field
	content, exists := requestBodyMap["content"]
	if !exists {
		log.Printf("DEBUG: content field not found in RequestBody")
		return false
	}

	contentMap, ok := content.(map[string]interface{})
	if !ok {
		log.Printf("DEBUG: content is not a map[string]interface{}")
		return false
	}

	// Check application/json content type
	appJson, exists := contentMap["application/json"]
	if !exists {
		log.Printf("DEBUG: application/json not found in content")
		return false
	}

	appJsonMap, ok := appJson.(map[string]interface{})
	if !ok {
		log.Printf("DEBUG: application/json is not a map[string]interface{}")
		return false
	}

	// Check the schema
	schema, exists := appJsonMap["schema"]
	if !exists {
		log.Printf("DEBUG: schema not found in application/json")
		return false
	}

	schemaMap, ok := schema.(map[string]interface{})
	if !ok {
		log.Printf("DEBUG: schema is not a map[string]interface{}")
		return false
	}

	log.Printf("DEBUG: schema found: %+v", schemaMap)

	// Check if type is array
	if schemaType, exists := schemaMap["type"]; exists {
		log.Printf("DEBUG: schema type found: %v", schemaType)
		if typeStr, ok := schemaType.(string); ok && typeStr == "array" {
			log.Printf("DEBUG: schema type is array - returning true")
			return true
		}
	}

	// Check if schema has a reference to an array definition
	if ref, exists := schemaMap["$ref"]; exists {
		if refStr, ok := ref.(string); ok {
			log.Printf("DEBUG: schema has $ref: %s", refStr)
			schemaName := ExtractSchemaName(refStr, "")
			if definition, found := getDefinition(globalSwaggerSpec, schemaName); found {
				log.Printf("DEBUG: found definition %s with type: %s", schemaName, definition.Type)
				return definition.Type == "array"
			}
		}
	}

	log.Printf("DEBUG: no array indication found - returning false")
	return false
}

func handleGetOper(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	name, ok := args["id"].(string)
	if !ok {
		return mcp.NewToolResultError("[Error] missing or invalid name parameter"), nil
	}

	for _, op := range globalOperations {
		if op.ID == name {

			// Create response without ID field
			response := map[string]interface{}{
				"id":           op.ID,
				"description":  op.Description,
				"method":       op.Method,
				"path":         op.Path,
				"summary":      op.Summary,
				"url":          op.URL,
				"parameters":   op.Parameters,
				"request_body": op.RequestBody,
				//"responses":    op.Responses,
			}
			result, _ := json.Marshal(response)
			return mcp.NewToolResultText(string(result)), nil
		}
	}

	return mcp.NewToolResultError("[Error] operation not found"), nil
}

func handleRunOper(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	name, ok := args["id"].(string)
	if !ok {
		return mcp.NewToolResultError("[Error] missing or invalid name parameter"), nil
	}

	// Extract combined query parameters (contains both path and query params)
	var combinedParams map[string]interface{}
	if queryStr, ok := args["query"].(string); ok && queryStr != "" {
		if err := json.Unmarshal([]byte(queryStr), &combinedParams); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] invalid JSON in query parameter: %v", err)), nil
		}
	}

	// Separate path and query parameters based on operation definition

	// Find the operation to get parameter definitions

	// Extract header parameters
	var headerParams map[string]interface{}
	if headerStr, ok := args["header"].(string); ok && headerStr != "" {
		if err := json.Unmarshal([]byte(headerStr), &headerParams); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("[Error] invalid JSON in header parameter: %v", err)), nil
		}
	}

	// Extract body parameters (for backwards compatibility)
	var bodyParams map[string]interface{}
	if bodyStr, ok := args["body"].(string); ok && bodyStr != "" {
		// Try to unmarshal as an object for backwards compatibility
		// The actual body processing will be handled in executeOperation
		json.Unmarshal([]byte(bodyStr), &bodyParams)
	}

	// Ensure we have a valid auth token before executing
	if authErr := ensureAuth(ctx); authErr != nil {
		return authErr, nil
	}

	for _, op := range globalOperations {
		if op.ID == name {
			return executeOperation(ctx, op, combinedParams, headerParams, bodyParams, request)
		}
	}

	return mcp.NewToolResultError("[Error] operation not found"), nil
}

func handleListOpers(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var operations []map[string]interface{}

	for _, op := range globalOperations {
		operations = append(operations, map[string]interface{}{
			"id":      op.ID,
			"summary": op.Summary,
		})
	}

	result, _ := json.Marshal(operations)
	return mcp.NewToolResultText(string(result)), nil
}

// handleLogin runs the OAuth2 PKCE flow, saves credentials, and updates the global auth token.
func handleLogin(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	creds, err := auth.PKCEFlow(globalOAuthClientID, nil)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] OAuth2 login failed: %v", err)), nil
	}

	if err := auth.Save(creds); err != nil {
		log.Printf("Warning: failed to save credentials: %v", err)
	}

	globalApiConfig.Authorization = creds.AuthorizationHeader()
	return mcp.NewToolResultText("Authorization successful! Token saved. You can now use Simulator tools."), nil
}

// ensureAuth checks that globalApiConfig.Authorization is set.
// If not, it tries to load/refresh from saved credentials.
// If still empty, uses MCP Elicitation to ask the user whether to start OAuth2.
// Returns an error result if authentication cannot be established.
func ensureAuth(ctx context.Context) *mcp.CallToolResult {
	// Already authenticated
	if globalApiConfig.Authorization != "" {
		return nil
	}

	// Try loading saved credentials
	creds, err := auth.Load()
	if err == nil && creds != nil {
		if !auth.IsExpired(creds) {
			globalApiConfig.Authorization = creds.AuthorizationHeader()
			return nil
		}
	}

	// No valid token — use elicitation to ask the user whether to open OAuth2 in browser
	mcpSrv := server.ServerFromContext(ctx)
	if mcpSrv != nil {
		elicitReq := mcp.ElicitationRequest{
			Request: mcp.Request{Method: string(mcp.MethodElicitationCreate)},
			Params: mcp.ElicitationParams{
				Message: "Simulator authentication required. Would you like to open a browser and sign in via OAuth2?",
				RequestedSchema: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"confirm": map[string]interface{}{
							"type":        "boolean",
							"title":       "Open browser to sign in",
							"description": "Click OK to open your browser for OAuth2 login",
						},
					},
				},
			},
		}

		result, err := mcpSrv.RequestElicitation(ctx, elicitReq)
		if err == nil && result != nil && result.Action == mcp.ElicitationResponseActionAccept {
			if creds, err := auth.PKCEFlow(globalOAuthClientID, nil); err == nil {
				_ = auth.Save(creds)
				globalApiConfig.Authorization = creds.AuthorizationHeader()
				return nil
			}
		}
	}

	return mcp.NewToolResultError("[Error] Not authenticated. Set SIMULATOR_TOKEN env var, or run the 'login' tool to authenticate via OAuth2.")
}

func executeOperation(ctx context.Context, op Operation, queryParams, headerParams, bodyParams map[string]interface{}, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	currentReqURL := op.URL
	log.Printf("DEBUG: pathParams called for operation %+v, URL: %s", queryParams, currentReqURL)

	// Handle path parameters
	for _, param := range op.Parameters {
		log.Printf("DEBUG: Found  parameter: %+v", param)
		if param.TIn == "path" {

			if queryParams != nil {
				if paramValue, exists := queryParams[param.Name]; exists {
					if paramStr, ok := paramValue.(string); ok {
						log.Printf("DEBUG: Replacing path parameter %s with value %s, in URL %s", param.Name, paramStr, currentReqURL)
						currentReqURL = strings.Replace(currentReqURL, fmt.Sprintf("{%s}", param.Name), paramStr, 1)
					} else {
						if paramBool, ok := paramValue.(bool); ok {
							currentReqURL = strings.Replace(currentReqURL, fmt.Sprintf("{%s}", param.Name), strconv.FormatBool(paramBool), 1)
						} else {
							if paramInt, ok := paramValue.(float64); ok {
								currentReqURL = strings.Replace(currentReqURL, fmt.Sprintf("{%s}", param.Name), strconv.Itoa(int(paramInt)), 1)
							} else {
								return mcp.NewToolResultError(fmt.Sprintf("[Error] invalid path parameter %s.", param.Name)), nil
							}

						}
					}
				} else if param.Required {
					return mcp.NewToolResultError(fmt.Sprintf("[Error] missing required path parameter: '%+v'", param)), nil
				}
			} else if param.Required {
				return mcp.NewToolResultError(fmt.Sprintf("[Error] missing required path parameter:'%s'", param.Name)), nil
			}
		}
	}

	// Handle query parameters
	u, err := url.Parse(currentReqURL)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] failed to parse URL: %v", err)), nil
	}
	q := u.Query()
	for _, param := range op.Parameters {
		if param.In == "query" {
			if queryParams != nil {
				if paramValue, exists := queryParams[param.Name]; exists {
					if paramStr, ok := paramValue.(string); ok {
						q.Set(param.Name, paramStr)
					} else {
						if paramBool, ok := paramValue.(bool); ok {
							q.Set(param.Name, strconv.FormatBool(paramBool))
						} else {
							if paramInt, ok := paramValue.(float64); ok {
								q.Set(param.Name, strconv.Itoa(int(paramInt)))
							} else {
								return mcp.NewToolResultError(fmt.Sprintf("[Error] invalid query parameter %s", param.Name)), nil
							}
						}

					}
				} else if param.Required {
					return mcp.NewToolResultError(fmt.Sprintf("[Error] missing required query parameter: %s", param.Name)), nil
				}
			} else if param.Required {
				return mcp.NewToolResultError(fmt.Sprintf("[Error] missing required query parameter: %s", param.Name)), nil
			}
		}
	}
	u.RawQuery = q.Encode()
	currentReqURL = u.String()

	// Handle request body
	var reqBodyBytes []byte
	hasBody := false

	// Check if we have a body to process
	if bodyStr, ok := request.GetArguments()["body"].(string); ok && bodyStr != "" {
		// Check if the schema expects an array and wrap the body parameter if needed
		isArray := checkIfRequestBodyIsArray(op)
		log.Printf("DEBUG: checkIfRequestBodyIsArray returned: %t for operation %s", isArray, op.ID)

		if isArray {
			// First, try to unmarshal the original body string to check if it's already an array
			var originalBody interface{}
			if err := json.Unmarshal([]byte(bodyStr), &originalBody); err == nil {
				// Check if the original body is already an array
				if _, isSlice := originalBody.([]interface{}); isSlice {
					log.Printf("DEBUG: Body is already an array, using as-is")
					reqBodyBytes, _ = json.Marshal(originalBody)
				} else {
					log.Printf("DEBUG: Body is not an array, wrapping in array")
					// Convert body to array
					arrayBody := []interface{}{originalBody}
					reqBodyBytes, _ = json.Marshal(arrayBody)
				}
			} else {
				log.Printf("DEBUG: Failed to unmarshal original body, error: %v", err)
				return mcp.NewToolResultError(fmt.Sprintf("[Error] invalid JSON in body parameter: %v", err)), nil
			}
			log.Printf("DEBUG: Final body: %s", string(reqBodyBytes))
		} else {
			// Schema doesn't expect an array, use the original body as-is
			reqBodyBytes = []byte(bodyStr)
			log.Printf("DEBUG: Using body as-is: %s", string(reqBodyBytes))
		}
		hasBody = true
	} else if bodyParams != nil && len(bodyParams) > 0 {
		// Fallback: use bodyParams if available
		reqBodyBytes, _ = json.Marshal(bodyParams)
		log.Printf("DEBUG: Using bodyParams: %s", string(reqBodyBytes))
		hasBody = true
	}

	// Create HTTP request
	var reqBody io.Reader
	if hasBody {
		reqBody = bytes.NewBuffer(reqBodyBytes)
	}

	req, err := http.NewRequest(strings.ToUpper(op.Method), currentReqURL, reqBody)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] failed to create HTTP request: %v", err)), nil
	}

	// Handle headers
	for _, param := range op.Parameters {
		if param.In == "header" {
			if headerParams != nil {
				if paramValue, exists := headerParams[param.Name]; exists {
					if paramStr, ok := paramValue.(string); ok {
						req.Header.Set(param.Name, paramStr)
					} else {
						return mcp.NewToolResultError(fmt.Sprintf("[Error] invalid header parameter %s", param.Name)), nil
					}
				} else if param.Required {
					return mcp.NewToolResultError(fmt.Sprintf("[Error] missing required header parameter: %s", param.Name)), nil
				}
			} else if param.Required {
				return mcp.NewToolResultError(fmt.Sprintf("[Error] missing required header parameter: %s", param.Name)), nil
			}
		}
	}
	log.Printf("Request  : %s %s %s %s\n", strings.ToUpper(op.Method), string(reqBodyBytes), currentReqURL, req.Header)

	// Add Authorization header if configured
	if globalApiConfig.Authorization != "" {
		req.Header.Set("Authorization", globalApiConfig.Authorization)
	}

	// Only set Content-Type if there's actually a body
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}

	// Execute request
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] failed to make HTTP request: %v", err)), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("[Error] failed to read HTTP response: %v", err)), nil
	}



	return mcp.NewToolResultText(string(body)), nil
}

func setRequestSecurity(req *http.Request, security string, basicAuth string, apiKeyAuth string, bearerAuth string) {
	securityType := strings.TrimSpace(security)

	// basic auth
	if securityType == "basic" && basicAuth != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(basicAuth))
		req.Header.Set("Authorization", "Basic "+auth)
	}

	// bearer auth
	if securityType == "bearer" && bearerAuth != "" {
		req.Header.Set("Authorization", "Bearer "+bearerAuth)
	}

	// apiKey auth
	// Example: header:token=abc,query:token=xyz,cookie:sid=ccc
	queryValues := make(map[string]string)
	cookieValues := []*http.Cookie{}
	if securityType == "apiKey" && apiKeyAuth != "" {
		for _, part := range strings.Split(apiKeyAuth, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			// format passAs:name=value
			colonIdx := strings.Index(part, ":")
			eqIdx := strings.Index(part, "=")
			if colonIdx == -1 || eqIdx == -1 || eqIdx < colonIdx+2 {
				continue
			}
			passAs := strings.ToLower(strings.TrimSpace(part[:colonIdx]))
			name := strings.TrimSpace(part[colonIdx+1 : eqIdx])
			value := strings.TrimSpace(part[eqIdx+1:])
			switch passAs {
			case "header":
				req.Header.Set(name, value)
			case "query":
				queryValues[name] = value
			case "cookie":
				cookieValues = append(cookieValues, &http.Cookie{Name: name, Value: value})
			}
		}
	}
	// add query
	if len(queryValues) > 0 {
		origUrl := req.URL.String()
		u, err := url.Parse(origUrl)
		if err == nil {
			q := u.Query()
			for k, v := range queryValues {
				q.Set(k, v)
			}
			u.RawQuery = q.Encode()
			req.URL = u
		}
	}
	// add cookie
	for _, c := range cookieValues {
		req.AddCookie(c)
	}
}

// initializeResources scans the data/resources directory and adds them as MCP resources
func initializeResources(mcpServer *server.MCPServer) {
	resourcesPath := "data/resources"

	if _, err := os.Stat(resourcesPath); os.IsNotExist(err) {
		return
	}

	// Walk through the resources directory and add each file as a resource
	err := filepath.Walk(resourcesPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Only process markdown files for now
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}

		// Create a resource URI from the relative path
		relPath, err := filepath.Rel(resourcesPath, path)
		if err != nil {
			return err
		}

		// Replace backslashes with forward slashes for URI
		resourceURI := "docs/" + strings.ReplaceAll(relPath, "\\", "/")

		// Create and add the resource
		resource := mcp.NewResource(
			resourceURI,
			filepath.Base(path),
			mcp.WithResourceDescription(fmt.Sprintf("Documentation file: %s", relPath)),
			mcp.WithMIMEType("text/markdown"),
		)

		mcpServer.AddResource(resource, handleResourceRead)

		// Log available resource
		log.Printf("Added MCP resource: %s", resourceURI)

		return nil
	})

	if err != nil {
		log.Printf("Error walking resources directory: %v", err)
	}
}

// handleResourceRead handles MCP resource read requests
func handleResourceRead(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	uri := request.Params.URI

	// Remove the "docs/" prefix to get the file path
	if !strings.HasPrefix(uri, "docs/") {
		return nil, fmt.Errorf("invalid resource URI: %s", uri)
	}

	relPath := strings.TrimPrefix(uri, "docs/")
	fullPath := filepath.Join("data/resources", relPath)

	// Check if file exists and is within the resources directory
	absResourcePath, err := filepath.Abs("data/resources")
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %v", err)
	}

	absFullPath, err := filepath.Abs(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %v", err)
	}

	if !strings.HasPrefix(absFullPath, absResourcePath) {
		return nil, fmt.Errorf("access denied: %s", uri)
	}

	// Read the file
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read resource %s: %v", uri, err)
	}

	// Return the content as TextResourceContents
	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      uri,
			MIMEType: "text/markdown",
			Text:     string(content),
		},
	}, nil
}
