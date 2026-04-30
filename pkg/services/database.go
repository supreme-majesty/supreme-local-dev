package services

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/brianvoe/gofakeit/v6"
)

type ConnectionProfile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        string `json:"port"`
	User        string `json:"user"`
	Password    string `json:"password"`
	Environment string `json:"environment"` // DEVELOPMENT, STAGING, PRODUCTION
	Color       string `json:"color"`       // UI accent color for this env
}

type WebhookConfig struct {
	URL      string `json:"url"`
	Type     string `json:"type"`    // SLACK, DISCORD, GENERIC
	Enabled  bool   `json:"enabled"`
	Events   []string `json:"events"` // AUDIT, HEALTH, PERFORMANCE
}

type MaintenanceTask struct {
	ID        string    `json:"id"`
	Database  string    `json:"database"`
	Type      string    `json:"type"` // BACKUP, OPTIMIZE, PII_SCAN
	Schedule  string    `json:"schedule"` // CRON string
	LastRun   time.Time `json:"last_run"`
	Enabled   bool      `json:"enabled"`
}

type PIIResult struct {
	Column   string   `json:"column"`
	Pattern  string   `json:"pattern"`
	Risk     string   `json:"risk"` // LOW, MEDIUM, HIGH
	Examples []string `json:"examples"`
}

type MaskingConfig struct {
	Database string            `json:"database"`
	Table    string            `json:"table"`
	Columns  map[string]string `json:"columns"` // Column -> MaskType (EMAIL, PHONE, NAME, RANDOM)
}

type TableDoc struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Columns     []ColumnDoc `json:"columns"`
	RowCount    int64 `json:"row_count"`
	Size        string `json:"size"`
}

type ColumnDoc struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	IsNullable  bool `json:"is_nullable"`
	Default     string `json:"default"`
	Key         string `json:"key"`
}

type SchemaAuditEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"` // CREATE, ALTER, DROP
	SQL       string    `json:"sql"`
	Target    string    `json:"target"` // Table name
}

type QuerySnippet struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	SQL       string    `json:"sql"`
	Database  string    `json:"database"`
	Tags      []string  `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
}

// DatabaseService manages MySQL/MariaDB connections
type Migration struct {
	ID        int       `json:"id"`
	Version   string    `json:"version"`
	Name      string    `json:"name"`
	AppliedAt time.Time `json:"applied_at"`
	Status    string    `json:"status"` // 'pending', 'applied'
	Content   string    `json:"content,omitempty"`
}

type SchemaDiff struct {
	TablesToCreate []string `json:"tables_to_create"`
	TablesToDrop   []string `json:"tables_to_drop"`
	TableDiffs     []TableDiff `json:"table_diffs"`
	SyncSQL        string `json:"sync_sql"`
}

type TableDiff struct {
	TableName      string   `json:"table_name"`
	ColumnsToAdd   []string `json:"columns_to_add"`
	ColumnsToDrop  []string `json:"columns_to_drop"`
	ColumnsToAlter []string `json:"columns_to_alter"`
	IndexesToAdd   []string `json:"indexes_to_add"`
	IndexesToDrop  []string `json:"indexes_to_drop"`
}

// DatabaseService manages database connections via drivers
type DatabaseService struct {
	db      *sql.DB // Legacy, to be replaced by driver
	Driver  DatabaseDriver
	dsn     string
	SnapDir string
}

// NewDatabaseService creates a new database service
func NewDatabaseService() *DatabaseService {
	// Default to MySQL for now
	return &DatabaseService{
		Driver:  NewMySQLDriver(),
		SnapDir: "/var/lib/sld/snapshots",
	}
}

// SetDriver switches the database driver (mysql or postgres)
func (d *DatabaseService) SetDriver(driverName string) {
	// Close existing
	if d.Driver != nil {
		d.Driver.Close()
	}

	switch driverName {
	case "postgres":
		d.Driver = NewPostgresDriver()
	default:
		d.Driver = NewMySQLDriver()
	}
}

// Connect establishes a connection
func (d *DatabaseService) Connect() error {
	// Pass empty config to trigger auto-discovery in driver
	return d.Driver.Connect(ConnectionConfig{})
}

// Close closes the database connection
func (d *DatabaseService) Close() {
	d.Driver.Close()
}

// ensureConnected reconnects if needed
func (d *DatabaseService) ensureConnected() error {
	if !d.Driver.IsConnected() {
		return d.Connect()
	}
	return nil
}

// ListDatabases returns all user databases
func (d *DatabaseService) ListDatabases() ([]string, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.Driver.ListDatabases()
}

func (d *DatabaseService) CreateDatabase(name string) error {
	if err := d.ensureConnected(); err != nil {
		return err
	}
	return d.Driver.CreateDatabase(name)
}

func (d *DatabaseService) DeleteDatabase(name string) error {
	if err := d.ensureConnected(); err != nil {
		return err
	}
	return d.Driver.DeleteDatabase(name)
}

func (d *DatabaseService) RenameDatabase(oldName, newName string) error {
	if err := d.ensureConnected(); err != nil {
		return err
	}
	return d.Driver.RenameDatabase(oldName, newName)
}

func (d *DatabaseService) Maintenance(database string, tables []string, operation string) ([]MaintenanceResult, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.Driver.Maintenance(database, tables, operation)
}

func (d *DatabaseService) GlobalSearch(database string, query string) ([]SearchResult, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.Driver.GlobalSearch(database, query)
}
func (d *DatabaseService) ListTables(database string) ([]TableInfo, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.Driver.ListTables(database)
}

func (d *DatabaseService) LogAudit(entry SchemaAuditEntry) error {
	logPath := filepath.Join(".sld", "schema_audit.json")
	var logs []SchemaAuditEntry
	
	if data, err := os.ReadFile(logPath); err == nil {
		json.Unmarshal(data, &logs)
	}
	
	logs = append([]SchemaAuditEntry{entry}, logs...)
	if len(logs) > 100 { logs = logs[:100] }
	
	data, _ := json.MarshalIndent(logs, "", "  ")
	os.MkdirAll(".sld", 0755)
	return os.WriteFile(logPath, data, 0644)
}

func (d *DatabaseService) GetAuditLog() ([]SchemaAuditEntry, error) {
	logPath := filepath.Join(".sld", "schema_audit.json")
	var logs []SchemaAuditEntry
	data, err := os.ReadFile(logPath)
	if err != nil { return []SchemaAuditEntry{}, nil }
	json.Unmarshal(data, &logs)
	return logs, nil
}

func (d *DatabaseService) SaveSnippet(snippet QuerySnippet) error {
	path := filepath.Join(".sld", "query_library.json")
	var snippets []QuerySnippet
	
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &snippets)
	}
	
	snippets = append(snippets, snippet)
	data, _ := json.MarshalIndent(snippets, "", "  ")
	os.MkdirAll(".sld", 0755)
	return os.WriteFile(path, data, 0644)
}

func (d *DatabaseService) GetSnippets() ([]QuerySnippet, error) {
	path := filepath.Join(".sld", "query_library.json")
	var snippets []QuerySnippet
	data, err := os.ReadFile(path)
	if err != nil { return []QuerySnippet{}, nil }
	json.Unmarshal(data, &snippets)
	return snippets, nil
}

// GetTableColumns returns column info for a table
func (d *DatabaseService) GetTableColumns(database, table string) ([]ColumnInfo, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.Driver.GetTableColumns(database, table)
}

func (d *DatabaseService) ImportData(database, table string, mapping map[string]string, data []map[string]interface{}) error {
	if err := d.ensureConnected(); err != nil { return err }
	
	// Create a transaction
	txId, err := d.BeginTransaction(database)
	if err != nil { return err }
	defer d.RollbackTransaction(txId)

	for _, row := range data {
		cols := []string{}
		vals := []interface{}{}
		placeholders := []string{}

		for fileCol, dbCol := range mapping {
			cols = append(cols, fmt.Sprintf("`%s`", dbCol))
			vals = append(vals, row[fileCol])
			placeholders = append(placeholders, "?")
		}

		query := fmt.Sprintf("INSERT INTO `%s` (%s) VALUES (%s)", 
			table, 
			strings.Join(cols, ", "), 
			strings.Join(placeholders, ", "),
		)
		
		_, err := d.ExecuteQuery(database, query, txId)
		if err != nil { return err }
	}

	return d.CommitTransaction(txId)
}

// GetTableData returns paginated data from a table
func (d *DatabaseService) GetTableData(database, table string, page, perPage int) (*TableData, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.Driver.GetTableData(database, table, page, perPage)
}

// GetTableDataEx returns paginated data with sorting and profiling
func (d *DatabaseService) GetTableDataEx(database, table string, page, perPage int, sortCol, sortOrder string, profile bool) (*TableData, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.Driver.GetTableDataEx(database, table, page, perPage, sortCol, sortOrder, profile)
}

// ExecuteQuery executes a SQL query
func (d *DatabaseService) ExecuteQuery(database, query string, txId string) (*QueryResult, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.Driver.ExecuteQuery(database, query, txId)
}

func (d *DatabaseService) BeginTransaction(database string) (string, error) {
	if err := d.ensureConnected(); err != nil {
		return "", err
	}
	return d.Driver.BeginTransaction(database)
}

func (d *DatabaseService) CommitTransaction(txId string) error {
	if err := d.ensureConnected(); err != nil {
		return err
	}
	return d.Driver.CommitTransaction(txId)
}

func (d *DatabaseService) RollbackTransaction(txId string) error {
	if err := d.ensureConnected(); err != nil {
		return err
	}
	return d.Driver.RollbackTransaction(txId)
}

func (d *DatabaseService) Query(database, query string, args []interface{}) ([]map[string]interface{}, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.Driver.Query(database, query, args)
}

func (d *DatabaseService) GetTables(database string) ([]TableInfo, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.Driver.ListTables(database)
}

func (d *DatabaseService) GenerateDocumentation(database string) ([]TableDoc, error) {
	tables, err := d.GetTables(database)
	if err != nil { return nil, err }

	var docs []TableDoc
	for _, t := range tables {
		info, _ := d.GetTableInfo(database, t.Name)
		cols, _ := d.GetTableColumns(database, t.Name)

		var colDocs []ColumnDoc
		for _, c := range cols {
			colDocs = append(colDocs, ColumnDoc{
				Name: c.Name,
				Type: c.Type,
				IsNullable: c.Nullable,
				Default: c.Default,
				Description: fmt.Sprintf("Storage for %s information.", c.Name),
			})
		}

		docs = append(docs, TableDoc{
			Name: t.Name,
			Description: fmt.Sprintf("Data entity for %s records.", t.Name),
			Columns: colDocs,
			RowCount: info.RowCount,
			Size: fmt.Sprintf("%d bytes", info.Size),
		})
	}
	return docs, nil
}

func (d *DatabaseService) GetTableInfo(database, table string) (*TableInfo, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.Driver.GetTableInfo(database, table)
}

func (d *DatabaseService) BatchInsert(database, table string, columns []string, data [][]interface{}, txId string) error {
	if err := d.ensureConnected(); err != nil {
		return err
	}
	return d.Driver.BatchInsert(database, table, columns, data, txId)
}

func (d *DatabaseService) GetStats(database string) (map[string]interface{}, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.Driver.GetStats(database)
}

func (d *DatabaseService) InitializeMigrations(database string) error {
	if err := d.ensureConnected(); err != nil {
		return err
	}

	query := `
		CREATE TABLE IF NOT EXISTS _sld_migrations (
			id INT AUTO_INCREMENT PRIMARY KEY,
			version VARCHAR(255) NOT NULL UNIQUE,
			name VARCHAR(255) NOT NULL,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB;
	`
	// Adjust for Postgres if needed, but for now we'll stick to MySQL compatible or use Driver specific logic
	// In a real multi-db app, we'd put this in the driver.
	
	_, err := d.Query(database, query, nil)
	return err
}

func (d *DatabaseService) GetAppliedMigrations(database string) ([]Migration, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}

	rows, err := d.Query(database, "SELECT id, version, name, applied_at FROM _sld_migrations ORDER BY version DESC", nil)
	if err != nil {
		return nil, err
	}

	var migrations []Migration
	for _, row := range rows {
		m := Migration{
			ID:      int(row["id"].(int64)),
			Version: row["version"].(string),
			Name:    row["name"].(string),
		}
		if t, ok := row["applied_at"].(time.Time); ok {
			m.AppliedAt = t
		}
		m.Status = "applied"
		migrations = append(migrations, m)
	}
	return migrations, nil
}

func (d *DatabaseService) GetPendingMigrations(database string) ([]Migration, error) {
	applied, err := d.GetAppliedMigrations(database)
	if err != nil {
		return nil, err
	}
	
	appliedMap := make(map[string]bool)
	for _, m := range applied {
		appliedMap[m.Version] = true
	}
	
	dir := filepath.Join("migrations", database)
	files, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Migration{}, nil
		}
		return nil, err
	}
	
	var pending []Migration
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".sql") {
			continue
		}
		
		version := strings.Split(f.Name(), "_")[0]
		if !appliedMap[version] {
			name := strings.TrimSuffix(strings.TrimPrefix(f.Name(), version+"_"), ".sql")
			pending = append(pending, Migration{
				Version: version,
				Name:    name,
				Status:  "pending",
			})
		}
	}
	
	return pending, nil
}

func (d *DatabaseService) CreateMigration(database, name string) (string, error) {
	version := time.Now().Format("20060102150405")
	filename := fmt.Sprintf("%s_%s.sql", version, name)
	
	// Create migrations dir if not exists
	dir := filepath.Join("migrations", database)
	os.MkdirAll(dir, 0755)
	
	path := filepath.Join(dir, filename)
	content := "-- UP\n\n\n-- DOWN\n\n"
	
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	
	return filename, nil
}

func (d *DatabaseService) RunMigration(database, filename string) error {
	dir := filepath.Join("migrations", database)
	path := filepath.Join(dir, filename)
	
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	
	// Parse UP section
	parts := strings.Split(string(content), "-- DOWN")
	upSection := parts[0]
	upSection = strings.TrimPrefix(upSection, "-- UP")
	
	// Split version from filename
	version := strings.Split(filename, "_")[0]
	name := strings.TrimSuffix(strings.TrimPrefix(filename, version+"_"), ".sql")
	
	// Run in transaction
	txId, err := d.BeginTransaction(database)
	if err != nil {
		return err
	}
	
	// Split queries by semicolon and execute
	queries := strings.Split(upSection, ";")
	for _, q := range queries {
		trimmed := strings.TrimSpace(q)
		if trimmed == "" {
			continue
		}
		if _, err := d.ExecuteQuery(database, trimmed, txId); err != nil {
			d.RollbackTransaction(txId)
			return err
		}
	}
	
	// Record migration
	recordQuery := fmt.Sprintf("INSERT INTO _sld_migrations (version, name) VALUES ('%s', '%s')", version, name)
	if _, err := d.ExecuteQuery(database, recordQuery, txId); err != nil {
		d.RollbackTransaction(txId)
		return err
	}
	
	return d.CommitTransaction(txId)
}

func (d *DatabaseService) CompareSchemas(sourceDB, targetDB string) (*SchemaDiff, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}

	diff := &SchemaDiff{
		TablesToCreate: []string{},
		TablesToDrop:   []string{},
		TableDiffs:     []TableDiff{},
	}
	var syncSQL strings.Builder

	sourceTables, _ := d.Driver.ListTables(sourceDB)
	targetTables, _ := d.Driver.ListTables(targetDB)

	sourceTableMap := make(map[string]bool)
	for _, t := range sourceTables {
		sourceTableMap[t.Name] = true
	}

	targetTableMap := make(map[string]bool)
	for _, t := range targetTables {
		targetTableMap[t.Name] = true
	}

	// Tables to create
	for _, t := range targetTables {
		if !sourceTableMap[t.Name] {
			diff.TablesToCreate = append(diff.TablesToCreate, t.Name)
			// For simplicity, we'd need GetCreateTable SQL from target and run on source
			// But for now we just record it.
			syncSQL.WriteString(fmt.Sprintf("-- Missing table in source: %s\n", t.Name))
		}
	}

	// Tables to drop
	for _, t := range sourceTables {
		if !targetTableMap[t.Name] {
			diff.TablesToDrop = append(diff.TablesToDrop, t.Name)
			syncSQL.WriteString(fmt.Sprintf("DROP TABLE `%s`;\n", t.Name))
		}
	}

	// Compare existing tables
	for _, t := range targetTables {
		if sourceTableMap[t.Name] {
			tableDiff, err := d.compareTableStructure(sourceDB, targetDB, t.Name)
			if err == nil && (len(tableDiff.ColumnsToAdd) > 0 || len(tableDiff.ColumnsToDrop) > 0 || len(tableDiff.ColumnsToAlter) > 0) {
				diff.TableDiffs = append(diff.TableDiffs, tableDiff)
				
				// Generate ALTER SQL
				for _, col := range tableDiff.ColumnsToAdd {
					syncSQL.WriteString(fmt.Sprintf("ALTER TABLE `%s` ADD %s;\n", t.Name, col))
				}
				for _, col := range tableDiff.ColumnsToDrop {
					syncSQL.WriteString(fmt.Sprintf("ALTER TABLE `%s` DROP COLUMN `%s`;\n", t.Name, col))
				}
				for _, col := range tableDiff.ColumnsToAlter {
					syncSQL.WriteString(fmt.Sprintf("ALTER TABLE `%s` MODIFY %s;\n", t.Name, col))
				}
			}
		}
	}

	diff.SyncSQL = syncSQL.String()
	return diff, nil
}

func (d *DatabaseService) compareTableStructure(sourceDB, targetDB, table string) (TableDiff, error) {
	diff := TableDiff{
		TableName:      table,
		ColumnsToAdd:   []string{},
		ColumnsToDrop:  []string{},
		ColumnsToAlter: []string{},
		IndexesToAdd:   []string{},
		IndexesToDrop:  []string{},
	}
	
	sourceCols, _ := d.GetTableColumns(sourceDB, table)
	targetCols, _ := d.GetTableColumns(targetDB, table)
	
	sourceColMap := make(map[string]ColumnInfo)
	for _, c := range sourceCols {
		sourceColMap[c.Name] = c
	}
	
	targetColMap := make(map[string]ColumnInfo)
	for _, c := range targetCols {
		targetColMap[c.Name] = c
	}
	
	// Columns to add
	for _, tc := range targetCols {
		if _, exists := sourceColMap[tc.Name]; !exists {
			colDef := fmt.Sprintf("`%s` %s", tc.Name, tc.Type)
			if !tc.Nullable { colDef += " NOT NULL" }
			if tc.Default != "" { colDef += " DEFAULT " + tc.Default }
			diff.ColumnsToAdd = append(diff.ColumnsToAdd, colDef)
		} else {
			// Compare definition
			sc := sourceColMap[tc.Name]
			if sc.Type != tc.Type || sc.Nullable != tc.Nullable {
				colDef := fmt.Sprintf("`%s` %s", tc.Name, tc.Type)
				if !tc.Nullable { colDef += " NOT NULL" }
				if tc.Default != "" { colDef += " DEFAULT " + tc.Default }
				diff.ColumnsToAlter = append(diff.ColumnsToAlter, colDef)
			}
		}
	}
	
	// Columns to drop
	for _, sc := range sourceCols {
		if _, exists := targetColMap[sc.Name]; !exists {
			diff.ColumnsToDrop = append(diff.ColumnsToDrop, sc.Name)
		}
	}
	
	return diff, nil
}

func (d *DatabaseService) ScanPII(database, table string) ([]PIIResult, error) {
	cols, err := d.GetTableColumns(database, table)
	if err != nil { return nil, err }

	patterns := map[string]struct{regex *regexp.Regexp; risk string}{
		"EMAIL": {regexp.MustCompile(`(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}`), "HIGH"},
		"PHONE": {regexp.MustCompile(`(?i)\+?[\d\s\-()]{7,}`), "MEDIUM"},
		"CC":    {regexp.MustCompile(`\d{4}[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}`), "HIGH"},
		"IP":    {regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`), "MEDIUM"},
	}

	var results []PIIResult
	for _, col := range cols {
		// Fetch sample data
		query := fmt.Sprintf("SELECT `%s` FROM `%s` WHERE `%s` IS NOT NULL LIMIT 50", col.Name, table, col.Name)
		rows, err := d.Query(database, query, nil)
		if err != nil { continue }

		for name, p := range patterns {
			matchCount := 0
			var examples []string
			for _, row := range rows {
				val := fmt.Sprintf("%v", row[col.Name])
				if p.regex.MatchString(val) {
					matchCount++
					if len(examples) < 3 { examples = append(examples, val) }
				}
			}

			if matchCount > 5 { // Threshold for detection
				results = append(results, PIIResult{
					Column: col.Name,
					Pattern: name,
					Risk: p.risk,
					Examples: examples,
				})
				break
			}
		}
	}
	return results, nil
}

func (d *DatabaseService) AnonymizeTable(config MaskingConfig) error {
	// For each row in the table, update the values with fakes
	// In a real app, we'd use a more efficient batch update
	rows, err := d.Query(config.Database, "SELECT * FROM `"+config.Table+"`", nil)
	if err != nil { return err }

	txId, err := d.BeginTransaction(config.Database)
	if err != nil { return err }

	for _, row := range rows {
		pkCol := "" // Simple assumption for now: first column is ID
		// Find PK...
		for k := range row { pkCol = k; break }
		
		var updates []string
		var args []interface{}
		for col, maskType := range config.Columns {
			var fakeVal interface{}
			switch maskType {
			case "EMAIL": fakeVal = gofakeit.Email()
			case "PHONE": fakeVal = gofakeit.Phone()
			case "NAME":  fakeVal = gofakeit.Name()
			default:      fakeVal = gofakeit.Word()
			}
			updates = append(updates, fmt.Sprintf("`%s` = ?", col))
			args = append(args, fakeVal)
		}
		
		if len(updates) > 0 {
			sql := fmt.Sprintf("UPDATE `%s` SET %s WHERE `%s` = ?", config.Table, strings.Join(updates, ", "), pkCol)
			args = append(args, row[pkCol])
			if _, err := d.ExecuteQuery(config.Database, sql, txId); err != nil {
				d.RollbackTransaction(txId)
				return err
			}
		}
	}

	return d.CommitTransaction(txId)
}
func (d *DatabaseService) AnalyzeQueryPlan(database, query string) (*QueryResult, error) {
	explainQuery := "EXPLAIN " + query
	return d.ExecuteQuery(database, explainQuery, "")
}

func (d *DatabaseService) GetOptimizationSuggestions(database, table string) ([]map[string]string, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}

	var suggestions []map[string]string
	
	cols, _ := d.GetTableColumns(database, table)
	indexes, _ := d.GetTableIndexes(database, table)
	
	// 1. Check for Foreign Keys without Indexes
	for _, col := range cols {
		if col.ForeignKey != nil {
			hasIndex := false
			for _, idx := range indexes {
				for _, idxCol := range idx.Columns {
					if idxCol == col.Name {
						hasIndex = true
						break
					}
				}
				if hasIndex { break }
			}
			
			if !hasIndex {
				suggestions = append(suggestions, map[string]string{
					"type": "index",
					"title": "Missing Index on Foreign Key",
					"description": fmt.Sprintf("The column `%s` is a foreign key but lacks an index. This can severely degrade JOIN performance.", col.Name),
					"sql": fmt.Sprintf("CREATE INDEX idx_%s_%s ON `%s`(`%s`);", table, col.Name, table, col.Name),
				})
			}
		}
	}
	
	// 2. Check for Table Overhead (MySQL only mostly)
	info, _ := d.GetTableInfo(database, table)
	if info != nil && info.Overhead > 1024*1024*10 { // > 10MB overhead
		suggestions = append(suggestions, map[string]string{
			"type": "maintenance",
			"title": "High Fragmentation",
			"description": fmt.Sprintf("Table `%s` has %d bytes of overhead. Running OPTIMIZE will reclaim space and improve performance.", table, info.Overhead),
			"sql": fmt.Sprintf("OPTIMIZE TABLE `%s`;", table),
		})
	}

	return suggestions, nil
}

func (d *DatabaseService) ExplainQuery(database, query string) (*QueryExplanation, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.Driver.ExplainQuery(database, query)
}

// CreateSnapshot creates a database snapshot using mysqldump
func (d *DatabaseService) CreateSnapshot(database, table string) (*Snapshot, error) {
	// Ensure snapshots directory exists
	if err := os.MkdirAll(d.SnapDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create snapshots directory: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.sql", database, timestamp)
	if table != "" {
		// Use a double underscore to separate db and table more clearly
		filename = fmt.Sprintf("%s__%s_%s.sql", database, table, timestamp)
	}
	filepath := filepath.Join(d.SnapDir, filename)

	// Run mysqldump
	args := []string{"-u", "root", database}
	if table != "" {
		args = append(args, table)
	}
	cmd := exec.Command("mysqldump", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("mysqldump failed: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filepath, output, 0644); err != nil {
		return nil, fmt.Errorf("failed to write snapshot: %w", err)
	}

	info, _ := os.Stat(filepath)

	return &Snapshot{
		ID:        timestamp,
		Database:  database,
		Filename:  filename,
		Size:      info.Size(),
		CreatedAt: time.Now(),
	}, nil
}

// ListSnapshots returns all available snapshots
func (d *DatabaseService) ListSnapshots() ([]Snapshot, error) {
	entries, err := os.ReadDir(d.SnapDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Snapshot{}, nil
		}
		return nil, err
	}

	var snapshots []Snapshot
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Parse filename: dbname_timestamp.sql or dbname__tablename_timestamp.sql
		name := strings.TrimSuffix(entry.Name(), ".sql")

		var dbName, tableName, timestamp string

		if strings.Contains(name, "__") {
			// New format: db__table_date_time
			parts := strings.Split(name, "__")
			dbName = parts[0]
			remaining := parts[1]
			remainingParts := strings.Split(remaining, "_")
			if len(remainingParts) >= 2 {
				tableName = strings.Join(remainingParts[:len(remainingParts)-2], "_")
				timestamp = remainingParts[len(remainingParts)-2] + "_" + remainingParts[len(remainingParts)-1]
			}
		} else {
			// Old format or simple db snapshot: db_date_time
			parts := strings.Split(name, "_")
			if len(parts) >= 2 {
				dbName = strings.Join(parts[:len(parts)-2], "_")
				timestamp = parts[len(parts)-2] + "_" + parts[len(parts)-1]
			}
		}

		snapshots = append(snapshots, Snapshot{
			ID:        timestamp,
			Database:  dbName,
			Table:     tableName,
			Filename:  entry.Name(),
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
		})
	}

	return snapshots, nil
}

// RestoreSnapshot restores a database from a snapshot
func (d *DatabaseService) RestoreSnapshot(filename string) error {
	filepath := filepath.Join(d.SnapDir, filename)

	if _, err := os.Stat(filepath); os.IsNotExist(err) {
		return fmt.Errorf("snapshot not found: %s", filename)
	}

	// Parse database name from filename
	name := strings.TrimSuffix(filename, ".sql")
	parts := strings.Split(name, "_")
	if len(parts) < 3 {
		return fmt.Errorf("invalid snapshot filename")
	}
	dbName := strings.Join(parts[:len(parts)-2], "_")

	return d.Driver.RestoreSnapshot(dbName, filepath)
}

// DeleteSnapshot deletes a snapshot file
func (d *DatabaseService) DeleteSnapshot(filename string) error {
	filepath := filepath.Join(d.SnapDir, filename)
	return os.Remove(filepath)
}

// RewindDatabase is a "Time-Travel" restore that first creates a safety backup
// before restoring the target snapshot. This allows users to "undo the undo".
func (d *DatabaseService) RewindDatabase(snapshotFilename string) (*Snapshot, error) {
	// 1. Parse the database name from the snapshot filename
	name := strings.TrimSuffix(snapshotFilename, ".sql")

	var dbName string
	if strings.Contains(name, "__") {
		// Table export: db__table_timestamp
		parts := strings.Split(name, "__")
		dbName = parts[0]
	} else {
		// Full DB export: db_timestamp
		parts := strings.Split(name, "_")
		if len(parts) < 3 {
			return nil, fmt.Errorf("invalid snapshot filename format")
		}
		dbName = strings.Join(parts[:len(parts)-2], "_")
	}

	// 2. Create an auto-backup BEFORE restoring (for undo capability)
	autoBackup, err := d.CreateSnapshot(dbName, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create safety backup before rewind: %w", err)
	}
	fmt.Printf("[TIME-TRAVEL] Created safety backup: %s\n", autoBackup.Filename)

	// 3. Restore the target snapshot
	if err := d.RestoreSnapshot(snapshotFilename); err != nil {
		return nil, fmt.Errorf("rewind failed: %w", err)
	}

	fmt.Printf("[TIME-TRAVEL] Rewound %s to snapshot: %s\n", dbName, snapshotFilename)
	return autoBackup, nil
}

// ImportSQL imports a SQL file into a specific database
func (d *DatabaseService) ImportSQL(database, sqlFilePath string) error {
	file, err := os.Open(sqlFilePath)
	if err != nil {
		return fmt.Errorf("failed to open SQL file: %w", err)
	}
	defer file.Close()

	cmd := exec.Command("mysql", "-u", "root", database)
	cmd.Stdin = file

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mysql import failed: %s", string(output))
	}

	return nil
}

type CloneOptions struct {
	Mode             string
	CreateDatabase   bool
	AddDropTable     bool
	AddAutoIncrement bool
	AddConstraints   bool
}

// CloneDatabase creates a copy of a database using mysqldump piped directly to mysql
func (d *DatabaseService) CloneDatabase(source, target string, opts CloneOptions) error {
	if err := d.ensureConnected(); err != nil {
		return err
	}

	mode := opts.Mode
	if mode == "" {
		mode = "both"
	}

	// Validate source exists
	var exists int
	err := d.db.QueryRow("SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?", source).Scan(&exists)
	if err != nil || exists == 0 {
		return fmt.Errorf("source database '%s' not found", source)
	}

	// Check target if we are creating it
	if opts.CreateDatabase {
		err = d.db.QueryRow("SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?", target).Scan(&exists)
		if err == nil && exists > 0 {
			return fmt.Errorf("target database '%s' already exists", target)
		}

		// Create target database
		_, err = d.db.Exec(fmt.Sprintf("CREATE DATABASE `%s`", target))
		if err != nil {
			return fmt.Errorf("failed to create target database: %w", err)
		}
	}

	// Prepare mysqldump args
	dumpArgs := []string{"-u", "root"}
	switch mode {
	case "structure":
		dumpArgs = append(dumpArgs, "--no-data")
	case "data":
		dumpArgs = append(dumpArgs, "--no-create-info")
	}

	if opts.AddDropTable {
		dumpArgs = append(dumpArgs, "--add-drop-table")
	} else {
		dumpArgs = append(dumpArgs, "--skip-add-drop-table")
	}

	if opts.AddConstraints {
		dumpArgs = append(dumpArgs, "--routines", "--triggers", "--events")
	}

	dumpArgs = append(dumpArgs, source)

	// Use pipe: mysqldump source | mysql target
	dumpCmd := exec.Command("mysqldump", dumpArgs...)
	importCmd := exec.Command("mysql", "-u", "root", target)

	// Create pipe
	pipe, err := dumpCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create pipe: %w", err)
	}
	importCmd.Stdin = pipe

	// Capture stderr for error reporting
	var dumpStderr, importStderr strings.Builder
	dumpCmd.Stderr = &dumpStderr
	importCmd.Stderr = &importStderr

	// Start both commands
	if err := dumpCmd.Start(); err != nil {
		return fmt.Errorf("failed to start mysqldump: %w", err)
	}
	if err := importCmd.Start(); err != nil {
		dumpCmd.Process.Kill()
		return fmt.Errorf("failed to start mysql import: %w", err)
	}

	// Wait for dump to complete
	if err := dumpCmd.Wait(); err != nil {
		importCmd.Process.Kill()
		return fmt.Errorf("mysqldump failed: %s", dumpStderr.String())
	}

	// Wait for import to complete
	if err := importCmd.Wait(); err != nil {
		return fmt.Errorf("mysql import failed: %s", importStderr.String())
	}

	return nil
}

// ForeignValue represents a value-label pair for foreign keys
type ForeignValue struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// GetForeignValues returns distinct values from a referenced table with labels
func (d *DatabaseService) GetForeignValues(database, table, column string) ([]ForeignValue, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	if _, err := d.db.Exec("USE " + database); err != nil {
		return nil, err
	}

	// 1. Get columns to find a likely label
	cols, err := d.GetTableColumns(database, table)
	if err != nil {
		return nil, err
	}

	labelCol := column // Default to ID itself

	// Heuristic: Look for name, title, email, slug, code
	candidates := []string{"name", "title", "label", "email", "username", "slug", "code"}
	found := false

	// First pass: exact match
	for _, cand := range candidates {
		for _, c := range cols {
			if strings.EqualFold(c.Name, cand) {
				labelCol = c.Name
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	// Second pass: contains match (e.g., full_name, article_title)
	if !found {
		for _, cand := range candidates {
			for _, c := range cols {
				if strings.Contains(strings.ToLower(c.Name), cand) {
					labelCol = c.Name
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}

	// Safety check: quote identifiers
	query := fmt.Sprintf("SELECT DISTINCT `%s`, `%s` FROM `%s` ORDER BY `%s` LIMIT 100", column, labelCol, table, labelCol)
	// If labelCol is same as column, we only select once to avoid ambiguity in scan?
	// Actually SQL handles `SELECT id, id ...` fine, but let's be clean.
	if labelCol == column {
		query = fmt.Sprintf("SELECT DISTINCT `%s` FROM `%s` ORDER BY `%s` LIMIT 100", column, table, column)
	}

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ForeignValue
	for rows.Next() {
		var val string
		var label string

		if labelCol == column {
			if err := rows.Scan(&val); err != nil {
				return nil, err
			}
			label = val
		} else {
			if err := rows.Scan(&val, &label); err != nil {
				return nil, err
			}
		}

		// Create composite label if different
		displayLabel := val
		if label != val {
			displayLabel = fmt.Sprintf("%s - %s", val, label)
		}

		results = append(results, ForeignValue{
			Value: val,
			Label: displayLabel,
		})
	}

	return results, nil
}

// GetTableRelationships returns all foreign key relationships in a database
func (d *DatabaseService) GetTableRelationships(database string) ([]TableRelationship, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.Driver.GetTableRelationships(database)
}
// ImportAnalysis represents the result of analyzing an import file
type ImportAnalysis struct {
	Columns []string                 `json:"columns"`
	Preview []map[string]interface{} `json:"preview"`
	Format  string                   `json:"format"`
}

// AnalyzeImport analyzes a file for import
func (d *DatabaseService) AnalyzeImport(filePath, format string) (*ImportAnalysis, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	analysis := &ImportAnalysis{
		Format:  format,
		Columns: []string{},
		Preview: []map[string]interface{}{},
	}

	if format == "csv" {
		reader := csv.NewReader(file)
		// Read header
		header, err := reader.Read()
		if err != nil {
			return nil, err
		}
		analysis.Columns = header

		// Read preview (up to 5 rows)
		for i := 0; i < 5; i++ {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				continue
			}
			row := make(map[string]interface{})
			for j, col := range header {
				if j < len(record) {
					row[col] = record[j]
				}
			}
			analysis.Preview = append(analysis.Preview, row)
		}
	} else if format == "json" {
		var data []map[string]interface{}
		if err := json.NewDecoder(file).Decode(&data); err != nil {
			return nil, err
		}

		if len(data) > 0 {
			// Get columns from first object
			for col := range data[0] {
				analysis.Columns = append(analysis.Columns, col)
			}
			// Sort columns for consistency
			sort.Strings(analysis.Columns)

			// Preview up to 5 rows
			for i := 0; i < len(data) && i < 5; i++ {
				analysis.Preview = append(analysis.Preview, data[i])
			}
		}
	}

	return analysis, nil
}

// ExecuteImport performs the actual import
func (d *DatabaseService) ExecuteImport(database, table, filePath, format string, mapping map[string]string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	var data [][]interface{}
	var targetCols []string
	for _, target := range mapping {
		if target != "" {
			targetCols = append(targetCols, target)
		}
	}

	if format == "csv" {
		reader := csv.NewReader(file)
		header, err := reader.Read()
		if err != nil {
			return err
		}

		// Find indices for mapped columns
		colIndices := make(map[string]int)
		for i, h := range header {
			if target, ok := mapping[h]; ok && target != "" {
				colIndices[target] = i
			}
		}

		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				continue
			}

			row := make([]interface{}, len(targetCols))
			for i, target := range targetCols {
				idx := colIndices[target]
				if idx < len(record) {
					row[i] = record[idx]
				} else {
					row[i] = nil
				}
			}
			data = append(data, row)

			// Batch insert every 1000 rows to save memory
			if len(data) >= 1000 {
				if err := d.BatchInsert(database, table, targetCols, data, ""); err != nil {
					return err
				}
				data = [][]interface{}{}
			}
		}
	} else if format == "json" {
		var rawData []map[string]interface{}
		if err := json.NewDecoder(file).Decode(&rawData); err != nil {
			return err
		}

		for _, item := range rawData {
			row := make([]interface{}, len(targetCols))
			for i, target := range targetCols {
				// Find source key for this target
				var sourceKey string
				for s, t := range mapping {
					if t == target {
						sourceKey = s
						break
					}
				}
				row[i] = item[sourceKey]
			}
			data = append(data, row)

			if len(data) >= 1000 {
				if err := d.BatchInsert(database, table, targetCols, data, ""); err != nil {
					return err
				}
				data = [][]interface{}{}
			}
		}
	}

	// Final batch
	if len(data) > 0 {
		return d.BatchInsert(database, table, targetCols, data, "")
	}

	return nil
}

func (d *DatabaseService) GetTableIndexes(database, table string) ([]IndexInfo, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.Driver.GetTableIndexes(database, table)
}

// SeedData generates and inserts mock data
func (d *DatabaseService) SeedData(database, table string, count int, fakers map[string]string) error {
	if err := d.ensureConnected(); err != nil {
		return err
	}

	var columns []string
	for col := range fakers {
		columns = append(columns, col)
	}

	var data [][]interface{}
	for i := 0; i < count; i++ {
		row := make([]interface{}, len(columns))
		for j, col := range columns {
			fakerType := fakers[col]
			row[j] = generateFakeValue(fakerType)
		}
		data = append(data, row)

		if len(data) >= 1000 {
			if err := d.BatchInsert(database, table, columns, data, ""); err != nil {
				return err
			}
			data = [][]interface{}{}
		}
	}

	if len(data) > 0 {
		return d.BatchInsert(database, table, columns, data, "")
	}

	return nil
}

func generateFakeValue(fakerType string) interface{} {
	switch fakerType {
	case "name":
		return gofakeit.Name()
	case "email":
		return gofakeit.Email()
	case "phone":
		return gofakeit.Phone()
	case "address":
		return gofakeit.Address().Address
	case "city":
		return gofakeit.City()
	case "country":
		return gofakeit.Country()
	case "company":
		return gofakeit.Company()
	case "job_title":
		return gofakeit.JobTitle()
	case "date":
		return gofakeit.Date().Format("2006-01-02")
	case "datetime":
		return gofakeit.Date().Format("2006-01-02 15:04:05")
	case "sentence":
		return gofakeit.Sentence(5)
	case "paragraph":
		return gofakeit.Paragraph(3, 5, 10, "\n")
	case "number":
		return gofakeit.Number(1, 1000)
	case "float":
		return gofakeit.Float64Range(1, 1000)
	case "bool":
		return gofakeit.Bool()
	case "uuid":
		return gofakeit.UUID()
	case "color":
		return gofakeit.Color()
	case "username":
		return gofakeit.Username()
	case "password":
		return gofakeit.Password(true, true, true, true, false, 12)
	default:
		return nil
	}
}

func (d *DatabaseService) SaveProfile(profile ConnectionProfile) error {
	profiles, _ := d.ListProfiles()
	found := false
	for i, p := range profiles {
		if p.ID == profile.ID {
			profiles[i] = profile
			found = true
			break
		}
	}
	if !found {
		profiles = append(profiles, profile)
	}
	os.MkdirAll(".sld", 0755)
	data, _ := json.MarshalIndent(profiles, "", "  ")
	return os.WriteFile(filepath.Join(".sld", "connections.json"), data, 0644)
}

func (d *DatabaseService) ListProfiles() ([]ConnectionProfile, error) {
	data, err := os.ReadFile(filepath.Join(".sld", "connections.json"))
	if err != nil { return []ConnectionProfile{}, nil }
	var profiles []ConnectionProfile
	json.Unmarshal(data, &profiles)
	return profiles, nil
}

func (d *DatabaseService) SendWebhook(config WebhookConfig, title, message string) error {
	if !config.Enabled { return nil }
	
	payload := map[string]interface{}{
		"text": fmt.Sprintf("*%s*\n%s", title, message),
	}
	if config.Type == "DISCORD" {
		payload = map[string]interface{}{
			"content": fmt.Sprintf("**%s**\n%s", title, message),
		}
	}

	body, _ := json.Marshal(payload)
	// In a real app, use http.Post. For now, we simulate.
	fmt.Printf("WEBHOOK SENT TO %s: %s\n", config.URL, string(body))
	return nil
}

func (d *DatabaseService) ExecuteMaintenance(task MaintenanceTask) error {
	fmt.Printf("EXECUTING MAINTENANCE: %s on %s\n", task.Type, task.Database)
	switch task.Type {
	case "BACKUP":
		_, err := d.CreateSnapshot(task.Database, "")
		return err
	case "OPTIMIZE":
		tables, _ := d.ListTables(task.Database)
		tableNames := []string{}
		for _, t := range tables { tableNames = append(tableNames, t.Name) }
		_, err := d.Maintenance(task.Database, tableNames, "OPTIMIZE")
		return err
	}
	return nil
}
