package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	mcpserver "git.corezoid.com/mw161089sar/swagger-mcp/app/mcp-server"
	"git.corezoid.com/mw161089sar/swagger-mcp/app/models"
	"git.corezoid.com/mw161089sar/swagger-mcp/app/swagger"
)

func getSseUrlAddr(sseUrl, sseAddr string) (string, string) {
	// Only complement if one is empty; if both are set, use as-is
	if sseAddr == "" && sseUrl == "" {
		return "http://localhost:8080", "localhost:8080"
	}
	if sseAddr != "" {
		// ":Port" or "IP:Port"
		if strings.HasPrefix(sseAddr, ":") {
			// sseUrl = http://localhost:Port
			return "http://localhost" + sseAddr, sseAddr
		}
		if !strings.Contains(sseAddr, ":") {
			log.Fatal("sseAddr must be in :Port or IP:Port format")
		}
		return "http://" + sseAddr, sseAddr
	} else if sseUrl != "" {
		u, err := url.Parse(sseUrl)
		if err != nil {
			log.Fatalf("Invalid sseUrl: %v", err)
		}
		host := u.Host
		port := ""
		if strings.Contains(host, ":") {
			parts := strings.Split(host, ":")
			host = parts[0]
			port = parts[1]
		}
		// 没有端口时根据 scheme 补全
		if port == "" {
			switch u.Scheme {
			case "http":
				port = "80"
			case "https":
				port = "443"
			default:
				log.Fatalf("Unknown scheme for sseUrl: %s", u.Scheme)
			}
		}
		return sseUrl, host + ":" + port
	} else {
		log.Fatal("Either sseAddr or sseUrl must be provided")
	}
	return "", ""
}

func main() {
	var finalSseUrl, finalSseAddr string
	specUrl := flag.String("specUrl", "", "URL of the Swagger JSON specification")
	spec := flag.String("spec", "", "Built-in spec name (e.g. 'simulator')")
	configFile := flag.String("config", "", "Path to configuration file for multiple services")
	sseMode := flag.Bool("sse", false, "Run in SSE mode instead of stdio mode")
	sseAddr := flag.String("sseAddr", "", "SSE server listen address in :Port or IP:Port format")
	sseUrl := flag.String("sseUrl", "", "Base URL for the SSE server")
	baseUrl := flag.String("baseUrl", "", "Base URL for API requests")
	apiUrl := flag.String("url", "", "API URL to use instead of extracting from Swagger spec")
	includePaths := flag.String("includePaths", "", "Comma-separated list of paths or regex to include")
	excludePaths := flag.String("excludePaths", "", "Comma-separated list of paths or regex to exclude")
	includeMethods := flag.String("includeMethods", "", "Comma-separated list of HTTP methods to include")
	excludeMethods := flag.String("excludeMethods", "", "Comma-separated list of HTTP methods to exclude")
	security := flag.String("security", "", "API security type: basic, apiKey, or bearer")
	basicAuth := flag.String("basicAuth", "", "Basic auth credentials in user:password format, used in Authorization header")
	authorization := flag.String("authorization", "", "Authorization token for Authorization header")
	apiKeyAuth := flag.String("apiKeyAuth", "", "API key auth, format: 'passAs:name=value', passAs=header/query/cookie, multiple by comma")
	headers := flag.String("headers", "", "Additional headers to include in requests (format: name1=value1,name2=value2)")
	sseHeaders := flag.String("sseHeaders", "", "Read headers from sse request, and pass to API request (format: name1,name2)")

	flag.Parse()

	// Validate that either specUrl, spec, or config file is provided
	if *specUrl == "" && *spec == "" && *configFile == "" {
		log.Fatal("Please provide --specUrl, --spec, or --config flag")
	}

	if *specUrl != "" && *configFile != "" {
		log.Fatal("Please provide either --specUrl OR --config flag, not both")
	}

	// Validate specUrl if provided
	if *specUrl != "" {
		if strings.HasPrefix(*specUrl, "http://") || strings.HasPrefix(*specUrl, "https://") {
			_, err := url.ParseRequestURI(*specUrl)
			if err != nil {
				log.Fatalf("Invalid spec URL: %v", err)
			}
		} else if strings.HasPrefix(*specUrl, "file://") {
			filePath := strings.TrimPrefix(*specUrl, "file://")
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				log.Fatalf("Spec file does not exist: %v", err)
			}
		} else {
			log.Fatal("Invalid specUrl format. Must be a valid HTTP URL or file:// path")
		}
	}

	// Validate baseUrl
	if *baseUrl != "" {
		if !strings.HasPrefix(*baseUrl, "http://") && !strings.HasPrefix(*baseUrl, "https://") {
			log.Fatal("baseUrl must start with http:// or https://")
		}
	}

	// Validate url parameter
	if *apiUrl != "" {
		if !strings.HasPrefix(*apiUrl, "http://") && !strings.HasPrefix(*apiUrl, "https://") {
			log.Fatal("url must start with http:// or https://")
		}
	}

	if *sseMode { // get final sseAddr and sseUrl
		finalSseUrl, finalSseAddr = getSseUrlAddr(*sseUrl, *sseAddr)
	}
	// Resolve built-in spec name to a swagger spec
	if *spec != "" {
		data, err := getBuiltinSpec(*spec)
		if err != nil {
			log.Fatal(err)
		}
		swaggerSpec, err := swagger.LoadSwaggerFromBytes(data)
		if err != nil {
			log.Fatalf("Failed to load built-in spec %q: %v", *spec, err)
		}
		config := models.Config{
			SpecUrl: *spec,
			SseCfg: models.SseConfig{
				SseMode: *sseMode,
				SseAddr: finalSseAddr,
				SseUrl:  finalSseUrl,
			},
			ApiCfg: models.ApiConfig{
				BaseUrl:        *baseUrl,
				Url:            *apiUrl,
				IncludePaths:   *includePaths,
				ExcludePaths:   *excludePaths,
				IncludeMethods: *includeMethods,
				ExcludeMethods: *excludeMethods,
				Security:       *security,
				BasicAuth:      *basicAuth,
				ApiKeyAuth:     *apiKeyAuth,
				Authorization:  *authorization,
				Headers:        *headers,
				SseHeaders:     *sseHeaders,
			},
		}
		//log.Printf("Starting server with built-in spec: %s\n", *spec)
		mcpserver.CreateServer(swaggerSpec, config)
		return
	}

	// Check if we should use multi-service mode or single service mode
	// For backward compatibility, if specUrl is provided, use single service mode
	if *specUrl != "" {
		// Single service mode
		swaggerSpec, err := swagger.LoadSwagger(*specUrl)
		if err != nil {
			log.Fatalf("Failed to load Swagger spec: %v", err)
		}
		// Only extract swagger info if not in stdio mode (commented out for MCP)
		// swagger.ExtractSwagger(swaggerSpec)

		config := models.Config{
			SpecUrl: *specUrl,
			SseCfg: models.SseConfig{
				SseMode: *sseMode,
				SseAddr: finalSseAddr,
				SseUrl:  finalSseUrl,
			},
			ApiCfg: models.ApiConfig{
				BaseUrl:        *baseUrl,
				Url:            *apiUrl,
				IncludePaths:   *includePaths,
				ExcludePaths:   *excludePaths,
				IncludeMethods: *includeMethods,
				ExcludeMethods: *excludeMethods,
				Security:       *security,
				BasicAuth:      *basicAuth,
				ApiKeyAuth:     *apiKeyAuth,
				Authorization:  *authorization,
				Headers:        *headers,
				SseHeaders:     *sseHeaders,
			},
		}

		log.Printf("Starting single service server with specUrl: %s, SSE mode: %v, SSE URL: %s, SSE Addr: %s, URL: %s, Base URL: %s, Include Paths: %s, Exclude Paths: %s, Include Methods: %s, Exclude Methods: %s, Security: %s, BasicAuth: %s, ApiKeyAuth: %s, Authorization: %s, Headers: %s, SSE Headers: %s\n",
			config.SpecUrl, config.SseCfg.SseMode, config.SseCfg.SseUrl, config.SseCfg.SseAddr, config.ApiCfg.Url, config.ApiCfg.BaseUrl, config.ApiCfg.IncludePaths, config.ApiCfg.ExcludePaths, config.ApiCfg.IncludeMethods, config.ApiCfg.ExcludeMethods, config.ApiCfg.Security, config.ApiCfg.BasicAuth, config.ApiCfg.ApiKeyAuth, config.ApiCfg.Authorization, config.ApiCfg.Headers, config.ApiCfg.SseHeaders)
		mcpserver.CreateServer(swaggerSpec, config)
	} else if *configFile != "" {
		// Multi-service mode using configuration file
		services, err := loadServicesFromConfig(*configFile)
		if err != nil {
			log.Fatalf("Failed to load services configuration: %v", err)
		}

		config := models.Config{
			Services: services,
			SseCfg: models.SseConfig{
				SseMode: *sseMode,
				SseAddr: finalSseAddr,
				SseUrl:  finalSseUrl,
			},
		}

		log.Printf("Starting multi-service server with %d services from config: %s, SSE mode: %v, SSE URL: %s, SSE Addr: %s\n",
			len(services), *configFile, config.SseCfg.SseMode, config.SseCfg.SseUrl, config.SseCfg.SseAddr)

		mcpserver.CreateMultiServiceServer(services, config)
	}
}

// loadServicesFromConfig loads service configurations from JSON file
func loadServicesFromConfig(configPath string) ([]models.ServiceConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var configData struct {
		Services []models.ServiceConfig `json:"services"`
	}

	if err := json.Unmarshal(data, &configData); err != nil {
		return nil, err
	}

	if len(configData.Services) == 0 {
		return nil, fmt.Errorf("no services found in configuration file")
	}

	return configData.Services, nil
}
