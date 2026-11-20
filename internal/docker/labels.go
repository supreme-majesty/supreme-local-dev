package docker

import (
	"fmt"
)

const (
	// Core SLD Labels
	LabelSLD          = "com.supreme.sld"
	LabelVersion      = "com.supreme.sld.version"
	LabelProject      = "com.supreme.sld.project"
	LabelProjectPath  = "com.supreme.sld.project-path"
	LabelManagedBy    = "com.supreme.sld.managed-by"
	LabelConfigHash   = "com.supreme.sld.config-hash"
	LabelService      = "com.supreme.sld.service"
	LabelResourceType = "com.supreme.sld.resource-type"
	LabelApp          = "com.supreme.sld.app"      // app name from .sld/config.yaml (app containers only)
	LabelRuntime      = "com.supreme.sld.runtime"  // app runtime, e.g. php, node (app containers only)
	LabelImageID      = "com.supreme.sld.image-id" // built image ID (dockerfile runtime), so rebuilds recreate the container

	// Value Constants
	CurrentVersion = "2"
	ManagedBySLD   = "sld"

	// Resource Types
	ResourceTypeContainer = "container"
	ResourceTypeNetwork   = "network"
	ResourceTypeVolume    = "volume"

	// Services
	ServiceNginx  = "nginx"
	ServicePHP    = "php"
	ServiceDB     = "db"
	ServiceRedis  = "redis"
	ServiceNode   = "node"
	ServiceRouter = "router"
)

// LabelParams contains the metadata needed to construct SLD Docker labels.
type LabelParams struct {
	ProjectID     string
	CanonicalPath string
	ConfigHash    string
	Service       string // Required only for containers
}

// BuildContainerLabels constructs standardized labels for a container resource.
func BuildContainerLabels(params LabelParams) (map[string]string, error) {
	if params.ProjectID == "" {
		return nil, fmt.Errorf("project ID is required for container labels")
	}
	if params.Service == "" {
		return nil, fmt.Errorf("service name is required for container labels")
	}

	labels := map[string]string{
		LabelSLD:          "true",
		LabelVersion:      CurrentVersion,
		LabelProject:      params.ProjectID,
		LabelProjectPath:  params.CanonicalPath,
		LabelManagedBy:    ManagedBySLD,
		LabelConfigHash:   params.ConfigHash,
		LabelService:      params.Service,
		LabelResourceType: ResourceTypeContainer,
	}

	return labels, nil
}

// BuildNetworkLabels constructs standardized labels for a network resource.
// Note: Networks describe connectivity, not a service. LabelService is strictly omitted.
func BuildNetworkLabels(params LabelParams) (map[string]string, error) {
	if params.ProjectID == "" {
		return nil, fmt.Errorf("project ID is required for network labels")
	}

	labels := map[string]string{
		LabelSLD:          "true",
		LabelVersion:      CurrentVersion,
		LabelProject:      params.ProjectID,
		LabelProjectPath:  params.CanonicalPath,
		LabelManagedBy:    ManagedBySLD,
		LabelConfigHash:   params.ConfigHash,
		LabelResourceType: ResourceTypeNetwork,
	}

	return labels, nil
}

// BuildVolumeLabels constructs standardized labels for a volume resource.
// Note: Volumes describe storage, not a service. LabelService is strictly omitted.
func BuildVolumeLabels(params LabelParams) (map[string]string, error) {
	if params.ProjectID == "" {
		return nil, fmt.Errorf("project ID is required for volume labels")
	}

	labels := map[string]string{
		LabelSLD:          "true",
		LabelVersion:      CurrentVersion,
		LabelProject:      params.ProjectID,
		LabelProjectPath:  params.CanonicalPath,
		LabelManagedBy:    ManagedBySLD,
		LabelConfigHash:   params.ConfigHash,
		LabelResourceType: ResourceTypeVolume,
	}

	return labels, nil
}

// ValidateLabels verifies that a label map complies with the V2 label contract.
func ValidateLabels(labels map[string]string, expectedResourceType string) error {
	if labels[LabelSLD] != "true" {
		return fmt.Errorf("missing %s=true label", LabelSLD)
	}
	if labels[LabelVersion] != CurrentVersion {
		return fmt.Errorf("invalid or missing %s label", LabelVersion)
	}
	if labels[LabelProject] == "" {
		return fmt.Errorf("missing %s label", LabelProject)
	}
	if labels[LabelResourceType] != expectedResourceType {
		return fmt.Errorf("expected resource type %s, got %s", expectedResourceType, labels[LabelResourceType])
	}

	if expectedResourceType == ResourceTypeContainer {
		if labels[LabelService] == "" {
			return fmt.Errorf("container labels must specify %s", LabelService)
		}
	} else {
		// Networks and volumes must NOT possess a service label
		if _, exists := labels[LabelService]; exists {
			return fmt.Errorf("resource type %s must not possess %s label", expectedResourceType, LabelService)
		}
	}

	return nil
}
