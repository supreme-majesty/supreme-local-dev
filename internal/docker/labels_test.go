package docker

import (
	"testing"
)

func TestBuildContainerLabels(t *testing.T) {
	params := LabelParams{
		ProjectID:     "my-app-3f7b49a1",
		CanonicalPath: "/home/user/my-app",
		ConfigHash:    "a1b2c3d4e5f6",
		Service:       ServicePHP,
	}

	labels, err := BuildContainerLabels(params)
	if err != nil {
		t.Fatalf("BuildContainerLabels failed: %v", err)
	}

	if labels[LabelSLD] != "true" {
		t.Errorf("expected %s=true, got %s", LabelSLD, labels[LabelSLD])
	}
	if labels[LabelVersion] != "2" {
		t.Errorf("expected version 2, got %s", labels[LabelVersion])
	}
	if labels[LabelProject] != "my-app-3f7b49a1" {
		t.Errorf("unexpected project: %s", labels[LabelProject])
	}
	if labels[LabelService] != ServicePHP {
		t.Errorf("unexpected service: %s", labels[LabelService])
	}
	if labels[LabelResourceType] != ResourceTypeContainer {
		t.Errorf("unexpected resource type: %s", labels[LabelResourceType])
	}

	if err := ValidateLabels(labels, ResourceTypeContainer); err != nil {
		t.Errorf("validation failed: %v", err)
	}
}

func TestBuildNetworkLabels_NoServiceLabel(t *testing.T) {
	params := LabelParams{
		ProjectID:     "my-app-3f7b49a1",
		CanonicalPath: "/home/user/my-app",
		ConfigHash:    "a1b2c3d4e5f6",
	}

	labels, err := BuildNetworkLabels(params)
	if err != nil {
		t.Fatalf("BuildNetworkLabels failed: %v", err)
	}

	if labels[LabelResourceType] != ResourceTypeNetwork {
		t.Errorf("expected resource type network, got %s", labels[LabelResourceType])
	}

	// Crucial rule: networks must not be labeled as services
	if _, exists := labels[LabelService]; exists {
		t.Errorf("network must NOT have a service label, but found %s=%s", LabelService, labels[LabelService])
	}

	if err := ValidateLabels(labels, ResourceTypeNetwork); err != nil {
		t.Errorf("validation failed: %v", err)
	}
}

func TestBuildVolumeLabels_NoServiceLabel(t *testing.T) {
	params := LabelParams{
		ProjectID:     "my-app-3f7b49a1",
		CanonicalPath: "/home/user/my-app",
		ConfigHash:    "a1b2c3d4e5f6",
	}

	labels, err := BuildVolumeLabels(params)
	if err != nil {
		t.Fatalf("BuildVolumeLabels failed: %v", err)
	}

	if labels[LabelResourceType] != ResourceTypeVolume {
		t.Errorf("expected resource type volume, got %s", labels[LabelResourceType])
	}

	// Crucial rule: volumes must not be labeled as services
	if _, exists := labels[LabelService]; exists {
		t.Errorf("volume must NOT have a service label, but found %s=%s", LabelService, labels[LabelService])
	}

	if err := ValidateLabels(labels, ResourceTypeVolume); err != nil {
		t.Errorf("validation failed: %v", err)
	}
}

func TestValidateLabels_RejectsMismatches(t *testing.T) {
	invalidNetwork := map[string]string{
		LabelSLD:          "true",
		LabelVersion:      "2",
		LabelProject:      "my-app-3f7b49a1",
		LabelResourceType: ResourceTypeNetwork,
		LabelService:      "network-service", // ILLEGAL
	}

	if err := ValidateLabels(invalidNetwork, ResourceTypeNetwork); err == nil {
		t.Errorf("expected validation to fail when network has service label, but passed")
	}

	containerMissingService := map[string]string{
		LabelSLD:          "true",
		LabelVersion:      "2",
		LabelProject:      "my-app-3f7b49a1",
		LabelResourceType: ResourceTypeContainer,
		// Missing LabelService
	}

	if err := ValidateLabels(containerMissingService, ResourceTypeContainer); err == nil {
		t.Errorf("expected validation to fail when container lacks service label, but passed")
	}
}
