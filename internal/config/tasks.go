package config

import (
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

// Task is a named one-off command run inside one of the project's app
// containers from the dashboard, e.g. migrations, seeding or a build.
//
//	tasks:
//	  migrate: php artisan migrate
//	  build:
//	    command: npm run build
//	    app: vite
//	    description: Production build of the frontend
type Task struct {
	// Name is the key under `tasks:`. Not serialized as a field.
	Name string `yaml:"-"`

	Command     string `yaml:"command"`
	App         string `yaml:"app,omitempty"` // default: the project's main app
	Description string `yaml:"description,omitempty"`
}

var taskFields = map[string]bool{"command": true, "app": true, "description": true}

var taskNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9:_.-]{0,39}$`)

// Tasks is an ordered set of tasks (a mapping in YAML; order is kept so the
// dashboard lists them as written).
type Tasks []Task

// UnmarshalYAML accepts `name: command` or `name: {command, app, description}`.
func (t *Tasks) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: tasks must be a mapping of task name to command", node.Line)
	}
	seen := map[string]bool{}
	out := make(Tasks, 0, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if seen[key.Value] {
			return fmt.Errorf("line %d: duplicate task %q", key.Line, key.Value)
		}
		seen[key.Value] = true
		task := Task{Name: key.Value}
		switch value.Kind {
		case yaml.ScalarNode:
			task.Command = value.Value
		case yaml.MappingNode:
			for j := 0; j+1 < len(value.Content); j += 2 {
				if f := value.Content[j]; !taskFields[f.Value] {
					return fmt.Errorf("line %d: tasks.%s: unknown field %q", f.Line, key.Value, f.Value)
				}
			}
			if err := value.Decode(&task); err != nil {
				return fmt.Errorf("tasks.%s: %w", key.Value, err)
			}
			task.Name = key.Value
		default:
			return fmt.Errorf("line %d: tasks.%s: expected a command or a mapping", value.Line, key.Value)
		}
		out = append(out, task)
	}
	*t = out
	return nil
}

// MarshalYAML writes a task as a plain command when it has nothing else.
func (t Tasks) MarshalYAML() (any, error) {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, task := range t {
		var value yaml.Node
		if task.App == "" && task.Description == "" {
			value = yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: task.Command}
		} else if err := value.Encode(task); err != nil {
			return nil, fmt.Errorf("tasks.%s: %w", task.Name, err)
		}
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: task.Name},
			&value,
		)
	}
	return node, nil
}

func validateTasks(cfg *Config, fail func(string, ...any)) {
	apps := map[string]bool{}
	for _, a := range cfg.Apps {
		apps[a.Name] = true
	}
	for _, task := range cfg.Tasks {
		path := "tasks." + task.Name
		if !taskNamePattern.MatchString(task.Name) {
			fail("%s: task names use lowercase letters, digits and ':' '_' '.' '-' (max 40 chars)", path)
		}
		if task.Command == "" {
			fail("%s.command: required", path)
		}
		if task.App != "" && !apps[task.App] {
			fail("%s.app: no app named %q", path, task.App)
		}
	}
}
