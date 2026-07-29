package connector

type EndpointConfig struct {
	URL        string `json:"url" yaml:"url"`
	Method     string `json:"method" yaml:"method"`
	Pagination string `json:"pagination" yaml:"pagination"`
}

type ConnectorManifest struct {
	ConnectorID string                       `json:"connector_id" yaml:"connector_id"`
	Name        string                       `json:"name" yaml:"name"`
	Category    string                       `json:"category" yaml:"category"`
	AuthType    string                       `json:"auth_type" yaml:"auth_type"`
	AuthConfig  map[string]string            `json:"auth_config" yaml:"auth_config"`
	Endpoints   map[string]EndpointConfig    `json:"endpoints" yaml:"endpoints"`
	SchemaMap   map[string]map[string]string `json:"schema_map" yaml:"schema_map"`
}