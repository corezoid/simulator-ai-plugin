package swagger

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"git.corezoid.com/mw161089sar/swagger-mcp/app/models"
)

// GetSpecVersion determines the OpenAPI/Swagger version from the spec
func GetSpecVersion(spec models.SwaggerSpec) string {
	if spec.OpenAPI != "" {
		return spec.OpenAPI
	}
	if spec.Swagger != "" {
		return spec.Swagger
	}
	return "unknown"
}

// ValidateSpecVersion validates if the specification version is supported
func ValidateSpecVersion(version string) error {
	switch {
	case version == "2.0":
		return nil
	case strings.HasPrefix(version, "3.0"):
		return nil
	case strings.HasPrefix(version, "3.1"):
		return nil
	default:
		return fmt.Errorf("unsupported OpenAPI/Swagger version: %s", version)
	}
}

// ServiceSwaggerSpec contains a swagger spec with its service information
type ServiceSwaggerSpec struct {
	ServiceName string             `json:"serviceName"`
	Spec        models.SwaggerSpec `json:"spec"`
}

func LoadSwaggerFromBytes(data []byte) (models.SwaggerSpec, error) {
	var swaggerSpec models.SwaggerSpec
	if err := json.Unmarshal(data, &swaggerSpec); err != nil {
		return models.SwaggerSpec{}, fmt.Errorf("error parsing JSON: %v", err)
	}
	version := GetSpecVersion(swaggerSpec)
	if err := ValidateSpecVersion(version); err != nil {
		return models.SwaggerSpec{}, err
	}
	//log.Printf("Loaded OpenAPI/Swagger specification version: %s\n", version)
	return swaggerSpec, nil
}

func LoadSwagger(specUrl string) (models.SwaggerSpec, error) {
	var body []byte
	var err error

	if strings.HasPrefix(specUrl, "file://") {
		filePath := strings.TrimPrefix(specUrl, "file://")
		body, err = os.ReadFile(filePath)
		if err != nil {
			return models.SwaggerSpec{}, fmt.Errorf("error reading file: %v", err)
		}
	} else {
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
		resp, err := client.Get(specUrl)
		if err != nil {
			return models.SwaggerSpec{}, fmt.Errorf("error getting spec: %v", err)
		}
		defer resp.Body.Close()

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return models.SwaggerSpec{}, fmt.Errorf("error reading spec: %v", err)
		}
	}
	var swaggerSpec models.SwaggerSpec
	if err := json.Unmarshal(body, &swaggerSpec); err != nil {
		return models.SwaggerSpec{}, fmt.Errorf("error parsing JSON:, %v", err.Error())
	}

	// Validate the specification version
	version := GetSpecVersion(swaggerSpec)
	if err := ValidateSpecVersion(version); err != nil {
		return models.SwaggerSpec{}, err
	}

	log.Printf("Loaded OpenAPI/Swagger specification version: %s\n", version)
	return swaggerSpec, nil
}
