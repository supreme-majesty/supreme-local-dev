package daemon

// Site represents a project/site served by SLD
type Site struct {
	Name       string   `json:"name"`
	Path       string   `json:"path"`
	Domain     string   `json:"domain"`
	PHPVersion string   `json:"phpVersion,omitempty"`
	Secure     bool     `json:"secure"`
	Type       string   `json:"type"`     // "parked" or "linked"
	Creating   bool     `json:"creating"` // true if project is still being created
	Tags       []string `json:"tags,omitempty"`
	Category   string   `json:"category,omitempty"`
}
