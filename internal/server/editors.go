package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Editors are opened through their URL schemes: the browser hands the link to
// the editor installed on the developer's machine. SLD runs in a container and
// cannot start programs on the host itself.

type editorScheme struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// build turns a host path (forward slashes, e.g. "C:/Users/me/app" or
	// "/home/me/app") into the editor's link.
	build func(path string) string
	// wsl builds a link for a folder inside a WSL distribution, if supported.
	wsl func(distro, path string) string
}

func fileURL(scheme string) func(string) string {
	return func(p string) string { return scheme + "://file" + slashPrefixed(p) }
}

func remoteWSL(scheme string) func(string, string) string {
	return func(distro, p string) string {
		return scheme + "://vscode-remote/wsl+" + url.PathEscape(distro) + slashPrefixed(p)
	}
}

func jetbrains(scheme string) func(string) string {
	return func(p string) string { return scheme + "://open?file=" + url.QueryEscape(p) }
}

var editorSchemes = []editorScheme{
	{ID: "vscode", Name: "Visual Studio Code", build: fileURL("vscode"), wsl: remoteWSL("vscode")},
	{ID: "cursor", Name: "Cursor", build: fileURL("cursor"), wsl: remoteWSL("cursor")},
	{ID: "windsurf", Name: "Windsurf", build: fileURL("windsurf"), wsl: remoteWSL("windsurf")},
	{ID: "vscodium", Name: "VSCodium", build: fileURL("vscodium")},
	{ID: "zed", Name: "Zed", build: fileURL("zed")},
	{ID: "phpstorm", Name: "PhpStorm", build: jetbrains("phpstorm")},
	{ID: "idea", Name: "IntelliJ IDEA", build: jetbrains("idea")},
	{ID: "webstorm", Name: "WebStorm", build: jetbrains("webstorm")},
	{ID: "pycharm", Name: "PyCharm", build: jetbrains("pycharm")},
	{ID: "sublime", Name: "Sublime Text", build: func(p string) string {
		return "subl://open?url=" + url.QueryEscape("file://"+slashPrefixed(p))
	}},
}

// slashPrefixed returns p as a URL path: a leading slash and each segment
// percent-encoded ("C:/Users/Supreme Majesty" -> "/C:/Users/Supreme%20Majesty").
func slashPrefixed(p string) string {
	segments := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for i, s := range segments {
		segments[i] = strings.ReplaceAll(url.PathEscape(s), "%3A", ":")
	}
	return "/" + strings.Join(segments, "/")
}

var (
	desktopMount = regexp.MustCompile(`^/run/desktop/mnt/host/([a-zA-Z])(/.*)?$`)
	wslShare     = regexp.MustCompile(`(?i)^//wsl(?:\$|\.localhost)/([^/]+)(/.*)?$`)
)

// editorLink returns the editor link for a host path, as the developer's OS
// names it: "C:\Users\me\app", "/run/desktop/mnt/host/c/Users/me/app",
// "\\wsl$\Ubuntu\home\me\app" or "/home/me/app".
func editorLink(ed editorScheme, hostPath string) string {
	p := strings.ReplaceAll(hostPath, `\`, "/")
	if m := wslShare.FindStringSubmatch(p); m != nil {
		rest := m[2]
		if rest == "" {
			rest = "/"
		}
		if ed.wsl != nil {
			return ed.wsl(m[1], rest)
		}
	}
	if m := desktopMount.FindStringSubmatch(p); m != nil {
		p = strings.ToUpper(m[1]) + ":" + m[2]
	}
	return ed.build(p)
}

// handleEditors lists editors: GET /api/system/editors.
func (s *Server) handleEditors(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, editorSchemes)
}

// handleOpenEditor returns the link that opens a project folder in an editor:
// POST /api/system/open-editor {path, editor} -> {success, url}. The dashboard
// navigates to it.
func (s *Server) handleOpenEditor(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path   string `json:"path"`
		Editor string `json:"editor"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&body); err != nil || body.Path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	path, err := s.inWorkspace(body.Path)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	ed := editorSchemes[0]
	for _, e := range editorSchemes {
		if e.ID == body.Editor {
			ed = e
		}
	}
	hostPath := path
	if s.opts.Env.InContainer() {
		hp, err := s.opts.Env.HostPath(path)
		if err != nil {
			writeError(w, http.StatusBadRequest, "cannot tell where this folder is on your machine: "+err.Error())
			return
		}
		hostPath = hp
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "url": editorLink(ed, hostPath), "host_path": hostPath})
}
