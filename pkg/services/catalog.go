package services

import (
	"fmt"
	"regexp"
	"strings"
)

// SystemDatabases are engine/tool catalogs that must not appear as project
// databases in SLD or phpMyAdmin, and must not be created, dropped, or renamed
// through the management APIs.
var SystemDatabases = []string{
	"information_schema",
	"performance_schema",
	"mysql",
	"sys",
	"phpmyadmin",
	"test",
	"postgres",
	"template0",
	"template1",
}

var (
	identPattern     = regexp.MustCompile(`(?i)^[\x60"'\[\]]*([A-Za-z0-9_]+)[\x60"'\[\]]*$`)
	dropDatabaseRE   = regexp.MustCompile(`(?is)(?:^|;)\s*(?:DROP)\s+(?:DATABASE|SCHEMA)\s+(?:IF\s+EXISTS\s+)?([^\s;]+)`)
	createDatabaseRE = regexp.MustCompile(`(?is)(?:^|;)\s*(?:CREATE)\s+(?:DATABASE|SCHEMA)\s+(?:IF\s+NOT\s+EXISTS\s+)?([^\s;]+)`)
	alterDatabaseRE  = regexp.MustCompile(`(?is)(?:^|;)\s*ALTER\s+(?:DATABASE|SCHEMA)\s+([^\s;]+)`)
	renameToRE       = regexp.MustCompile(`(?is)\bRENAME\s+TO\s+([^\s;]+)`)
)

// IsSystemDatabase reports whether name is a reserved catalog.
func IsSystemDatabase(name string) bool {
	n := normalizeCatalogName(name)
	if n == "" {
		return false
	}
	for _, sys := range SystemDatabases {
		if n == sys {
			return true
		}
	}
	return false
}

// FilterUserDatabases drops reserved catalogs from a server listing.
func FilterUserDatabases(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		if IsSystemDatabase(name) {
			continue
		}
		out = append(out, name)
	}
	return out
}

// RejectSystemDatabase returns an error if name is reserved.
func RejectSystemDatabase(name string) error {
	if IsSystemDatabase(name) {
		return fmt.Errorf("database %q is a system catalog and cannot be managed here", normalizeCatalogName(name))
	}
	return nil
}

// GuardCatalogSQL blocks CREATE/DROP/ALTER DATABASE against reserved names.
func GuardCatalogSQL(query string) error {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	for _, re := range []*regexp.Regexp{dropDatabaseRE, createDatabaseRE, alterDatabaseRE} {
		matches := re.FindAllStringSubmatch(query, -1)
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			if err := RejectSystemDatabase(m[1]); err != nil {
				return err
			}
		}
	}
	renameMatches := renameToRE.FindAllStringSubmatch(query, -1)
	for _, m := range renameMatches {
		if len(m) < 2 {
			continue
		}
		if err := RejectSystemDatabase(m[1]); err != nil {
			return err
		}
	}
	return nil
}

// PhpMyAdminHideDbPattern is a PMA hide_db regex matching SystemDatabases.
func PhpMyAdminHideDbPattern() string {
	parts := make([]string, 0, len(SystemDatabases))
	for _, name := range SystemDatabases {
		parts = append(parts, regexp.QuoteMeta(name))
	}
	return "^(?:" + strings.Join(parts, "|") + ")$"
}

func normalizeCatalogName(name string) string {
	name = strings.TrimSpace(name)
	if m := identPattern.FindStringSubmatch(name); len(m) == 2 {
		return strings.ToLower(m[1])
	}
	name = strings.Trim(name, "`\"'[]")
	return strings.ToLower(strings.TrimSpace(name))
}
