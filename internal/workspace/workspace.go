// Package workspace reads JavaScript monorepo workspaces: the package folders
// a pnpm-workspace.yaml or a package.json "workspaces" field lists.
package workspace

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// maxDepth bounds "**" globs (packages/**/x) so a huge tree is not walked.
const maxDepth = 4

// Patterns returns the workspace globs declared at root, or nil when root is
// not a workspace root.
func Patterns(root string) []string {
	if data, err := os.ReadFile(filepath.Join(root, "pnpm-workspace.yaml")); err == nil {
		var ws struct {
			Packages []string `yaml:"packages"`
		}
		if yaml.Unmarshal(data, &ws) == nil && len(ws.Packages) > 0 {
			return ws.Packages
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return nil
	}
	var pkg struct {
		Workspaces json.RawMessage `json:"workspaces"`
	}
	if json.Unmarshal(data, &pkg) != nil || len(pkg.Workspaces) == 0 {
		return nil
	}
	var list []string
	if json.Unmarshal(pkg.Workspaces, &list) == nil {
		return list
	}
	var obj struct {
		Packages []string `json:"packages"` // yarn classic's {packages, nohoist}
	}
	if json.Unmarshal(pkg.Workspaces, &obj) == nil {
		return obj.Packages
	}
	return nil
}

// Packages returns the workspace package folders under root (slash-separated,
// relative to root, sorted) that contain a package.json. It is empty when
// root is not a workspace root.
func Packages(root string) []string {
	patterns := Patterns(root)
	if len(patterns) == 0 {
		return nil
	}
	found := map[string]bool{}
	var excludes []string
	for _, p := range patterns {
		p = strings.TrimPrefix(strings.TrimSpace(p), "./")
		if p == "" {
			continue
		}
		if ex, ok := strings.CutPrefix(p, "!"); ok {
			excludes = append(excludes, strings.TrimPrefix(ex, "./"))
			continue
		}
		for _, dir := range expand(root, p) {
			found[dir] = true
		}
	}
	var out []string
	for dir := range found {
		if dir == "." || excluded(dir, excludes) || !valid(dir) {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir), "package.json")); err == nil {
			out = append(out, dir)
		}
	}
	sort.Strings(out)
	return out
}

// expand turns one glob into existing directories. "**" matches any depth up
// to maxDepth; other segments use path.Match.
func expand(root, pattern string) []string {
	segments := strings.Split(strings.TrimSuffix(pattern, "/"), "/")
	var walk func(dir string, segs []string, depth int) []string
	walk = func(dir string, segs []string, depth int) []string {
		if len(segs) == 0 {
			return []string{dir}
		}
		if depth > maxDepth {
			return nil
		}
		seg := segs[0]
		var out []string
		if seg == "**" {
			out = append(out, walk(dir, segs[1:], depth)...) // zero directories
			for _, child := range subdirs(root, dir) {
				out = append(out, walk(child, segs, depth+1)...)
			}
			return out
		}
		if !strings.ContainsAny(seg, "*?[") {
			child := join(dir, seg)
			if isDir(root, child) {
				out = walk(child, segs[1:], depth+1)
			}
			return out
		}
		for _, child := range subdirs(root, dir) {
			if ok, _ := path.Match(seg, path.Base(child)); ok {
				out = append(out, walk(child, segs[1:], depth+1)...)
			}
		}
		return out
	}
	return walk(".", segments, 0)
}

func subdirs(root, dir string) []string {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != "node_modules" && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, join(dir, e.Name()))
		}
	}
	return out
}

func join(dir, name string) string {
	if dir == "." {
		return name
	}
	return dir + "/" + name
}

func isDir(root, dir string) bool {
	st, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir)))
	return err == nil && st.IsDir()
}

func excluded(dir string, patterns []string) bool {
	for _, p := range patterns {
		p = strings.TrimSuffix(p, "/**")
		if ok, _ := path.Match(p, dir); ok || dir == p || strings.HasPrefix(dir, p+"/") {
			return true
		}
	}
	return false
}

// valid rejects folders whose paths would not survive being used in shell
// variables and volume targets (spaces, quotes...).
func valid(dir string) bool {
	for _, r := range dir {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '/' || r == '-' || r == '_' || r == '.' || r == '@':
		default:
			return false
		}
	}
	return !strings.Contains(dir, "..")
}
