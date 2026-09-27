package reconcile

import (
	"testing"
)

func TestDiffContainer_MissingResource(t *testing.T) {
	desired := &DesiredContainer{
		Name:       "sld-app-3f7b49a1-php",
		Service:    "php",
		Image:      "ghcr.io/supreme-majesty/sld-php:8.3",
		ConfigHash: "hash-123",
	}

	plan := DiffContainer(desired, nil)
	if plan.Action != ActionCreate {
		t.Errorf("expected ActionCreate for missing container, got %s", plan.Action)
	}
}

func TestDiffContainer_HealthyRunningMatch(t *testing.T) {
	desired := &DesiredContainer{
		Name:       "sld-app-3f7b49a1-php",
		Service:    "php",
		Image:      "ghcr.io/supreme-majesty/sld-php:8.3",
		ConfigHash: "hash-123",
	}

	actual := &ActualContainer{
		Name:       "sld-app-3f7b49a1-php",
		Service:    "php",
		Image:      "ghcr.io/supreme-majesty/sld-php:8.3",
		ConfigHash: "hash-123",
		State:      "running",
	}

	plan := DiffContainer(desired, actual)
	if plan.Action != ActionNoOp {
		t.Errorf("expected ActionNoOp for matching running container, got %s", plan.Action)
	}
}

func TestDiffContainer_StoppedResource_NeedsStart(t *testing.T) {
	desired := &DesiredContainer{
		Name:       "sld-app-3f7b49a1-php",
		Service:    "php",
		Image:      "ghcr.io/supreme-majesty/sld-php:8.3",
		ConfigHash: "hash-123",
	}

	actual := &ActualContainer{
		Name:       "sld-app-3f7b49a1-php",
		Service:    "php",
		Image:      "ghcr.io/supreme-majesty/sld-php:8.3",
		ConfigHash: "hash-123",
		State:      "exited",
	}

	plan := DiffContainer(desired, actual)
	if plan.Action != ActionStart {
		t.Errorf("expected ActionStart for stopped container, got %s", plan.Action)
	}
}

func TestDiffContainer_ImageChanged_NeedsRecreate(t *testing.T) {
	desired := &DesiredContainer{
		Name:       "sld-app-3f7b49a1-php",
		Service:    "php",
		Image:      "ghcr.io/supreme-majesty/sld-php:8.4", // changed to 8.4
		ConfigHash: "hash-new",
	}

	actual := &ActualContainer{
		Name:       "sld-app-3f7b49a1-php",
		Service:    "php",
		Image:      "ghcr.io/supreme-majesty/sld-php:8.3",
		ConfigHash: "hash-old",
		State:      "running",
	}

	plan := DiffContainer(desired, actual)
	if plan.Action != ActionRecreate {
		t.Errorf("expected ActionRecreate for changed image, got %s", plan.Action)
	}
}

func TestComputeContainerPlan_ObsoleteResource(t *testing.T) {
	desiredList := []*DesiredContainer{
		{Name: "sld-app-3f7b49a1-php", Service: "php", Image: "sld-php:8.3", ConfigHash: "hash1"},
	}

	actualMap := map[string]*ActualContainer{
		"sld-app-3f7b49a1-php":   {Name: "sld-app-3f7b49a1-php", State: "running", Image: "sld-php:8.3", ConfigHash: "hash1"},
		"sld-app-3f7b49a1-redis": {Name: "sld-app-3f7b49a1-redis", State: "running", Image: "redis:7"}, // no longer in desired
	}

	plans := ComputeContainerPlan(desiredList, actualMap)
	if len(plans) != 2 {
		t.Fatalf("expected 2 plans, got %d", len(plans))
	}

	var phpPlan, redisPlan *DesiredResourcePlan
	for i := range plans {
		if plans[i].ResourceName == "sld-app-3f7b49a1-php" {
			phpPlan = &plans[i]
		}
		if plans[i].ResourceName == "sld-app-3f7b49a1-redis" {
			redisPlan = &plans[i]
		}
	}

	if phpPlan == nil || phpPlan.Action != ActionNoOp {
		t.Errorf("expected php plan to be NoOp")
	}
	if redisPlan == nil || redisPlan.Action != ActionRemove {
		t.Errorf("expected redis plan to be Remove")
	}
}
