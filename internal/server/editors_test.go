package server

import "testing"

func TestEditorLink(t *testing.T) {
	byID := map[string]editorScheme{}
	for _, e := range editorSchemes {
		byID[e.ID] = e
	}
	cases := []struct {
		editor, hostPath, want string
	}{
		{"vscode", `C:\Users\me\Code\shop`, "vscode://file/C:/Users/me/Code/shop"},
		{"vscode", "/run/desktop/mnt/host/c/Users/me/shop", "vscode://file/C:/Users/me/shop"},
		{"vscode", "/run/desktop/mnt/host/c/Users/Supreme Majesty/shop", "vscode://file/C:/Users/Supreme%20Majesty/shop"},
		{"vscode", "/home/me/Projects/shop", "vscode://file/home/me/Projects/shop"},
		{"vscode", `\\wsl$\Ubuntu\home\me\shop`, "vscode://vscode-remote/wsl+Ubuntu/home/me/shop"},
		{"cursor", `\\wsl.localhost\Ubuntu-24.04\home\me\shop`, "cursor://vscode-remote/wsl+Ubuntu-24.04/home/me/shop"},
		{"phpstorm", "/Users/me/shop", "phpstorm://open?file=%2FUsers%2Fme%2Fshop"},
		{"zed", "/Users/me/shop", "zed://file/Users/me/shop"},
	}
	for _, c := range cases {
		if got := editorLink(byID[c.editor], c.hostPath); got != c.want {
			t.Errorf("editorLink(%s, %q) = %q, want %q", c.editor, c.hostPath, got, c.want)
		}
	}
}
