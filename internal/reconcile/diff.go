package reconcile

import "time"

// ActionType represents the action determined by the diff engine.
type ActionType string

const (
	ActionNoOp     ActionType = "NOOP"
	ActionCreate   ActionType = "CREATE"
	ActionStart    ActionType = "START"
	ActionUpdate   ActionType = "UPDATE"
	ActionRecreate ActionType = "RECREATE"
	ActionRemove   ActionType = "REMOVE"
)

// HealthcheckConfig defines a Docker container healthcheck specification.
type HealthcheckConfig struct {
	Test        []string
	Interval    time.Duration
	Timeout     time.Duration
	StartPeriod time.Duration
	Retries     int
}

// Mount types supported by Mount.
const (
	MountTypeBind   = "bind"
	MountTypeVolume = "volume"
)

// Mount is a structured filesystem mount. Unlike the Volumes bind strings it
// can express a named-volume subpath, which lets SLD share a sub-directory of
// its own data volume with sibling containers when SLD itself runs in Docker.
type Mount struct {
	Type     string // MountTypeBind or MountTypeVolume
	Source   string // host path (bind) or volume name (volume)
	Target   string // path inside the container
	ReadOnly bool
	Subpath  string // volume only: sub-directory of the volume to mount (Docker Engine >= 26)
}

// DesiredContainer defines the required configuration of a container.
type DesiredContainer struct {
	Name        string
	Service     string
	Image       string
	ConfigHash  string
	Cmd         []string
	WorkingDir  string
	User        string
	Environment map[string]string
	Ports       []string
	Networks    []string
	Volumes     []string // "source:target[:mode]" bind / named-volume strings
	Mounts      []Mount  // structured mounts (host-path translated, volume subpaths)
	ExtraHosts  []string
	Labels      map[string]string
	Healthcheck *HealthcheckConfig
	// SkipHealthWait starts the container without blocking on its healthcheck
	// (dev servers may spend minutes installing dependencies first).
	SkipHealthWait bool
	// RestartPolicy is Docker's restart policy. Empty means "no": project
	// containers start only when asked to, never on their own when Docker
	// starts. SLD's own router uses "unless-stopped".
	RestartPolicy string
}

// ActualContainer represents the current state of a container in Docker.
type ActualContainer struct {
	ID         string
	Name       string
	Service    string
	Image      string
	ConfigHash string
	State      string // "running", "exited", "created", etc.
	Labels     map[string]string
}

// DesiredResourcePlan encapsulates the action to be taken for a specific container.
type DesiredResourcePlan struct {
	ResourceName string
	Action       ActionType
	Reason       string
	Desired      *DesiredContainer
	Actual       *ActualContainer
}

// DiffContainer compares a desired container against its actual Docker state.
func DiffContainer(desired *DesiredContainer, actual *ActualContainer) DesiredResourcePlan {
	if actual == nil {
		return DesiredResourcePlan{
			ResourceName: desired.Name,
			Action:       ActionCreate,
			Reason:       "container does not exist",
			Desired:      desired,
		}
	}

	// If the container exists, check if immutable properties changed
	if actual.Image != desired.Image {
		return DesiredResourcePlan{
			ResourceName: desired.Name,
			Action:       ActionRecreate,
			Reason:       "image changed from " + actual.Image + " to " + desired.Image,
			Desired:      desired,
			Actual:       actual,
		}
	}

	// If configuration hash changed
	if actual.ConfigHash != desired.ConfigHash && desired.ConfigHash != "" {
		return DesiredResourcePlan{
			ResourceName: desired.Name,
			Action:       ActionRecreate,
			Reason:       "configuration hash changed",
			Desired:      desired,
			Actual:       actual,
		}
	}

	// If container is stopped but configuration matches
	if actual.State != "running" {
		return DesiredResourcePlan{
			ResourceName: desired.Name,
			Action:       ActionStart,
			Reason:       "container is stopped",
			Desired:      desired,
			Actual:       actual,
		}
	}

	// Configuration matches and container is running
	return DesiredResourcePlan{
		ResourceName: desired.Name,
		Action:       ActionNoOp,
		Reason:       "container is healthy and configuration matches",
		Desired:      desired,
		Actual:       actual,
	}
}

// ComputeContainerPlan diffs all desired containers against all actual containers.
func ComputeContainerPlan(desiredList []*DesiredContainer, actualMap map[string]*ActualContainer) []DesiredResourcePlan {
	var plan []DesiredResourcePlan
	seen := make(map[string]bool)

	for _, d := range desiredList {
		seen[d.Name] = true
		actual := actualMap[d.Name]
		plan = append(plan, DiffContainer(d, actual))
	}

	// Check for obsolete containers that exist in actual but are not in desired
	for name, a := range actualMap {
		if !seen[name] {
			plan = append(plan, DesiredResourcePlan{
				ResourceName: name,
				Action:       ActionRemove,
				Reason:       "container is no longer declared in desired configuration",
				Actual:       a,
			})
		}
	}

	return plan
}
