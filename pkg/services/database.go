package services

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// DatabaseService manages MySQL/MariaDB connections
// DatabaseService manages database connections via drivers
type DatabaseService struct {
	db      *sql.DB // Legacy, to be replaced by driver
	driver  DatabaseDriver
	dsn     string
	SnapDir string
}

// NewDatabaseService creates a new database service
func NewDatabaseService() *DatabaseService {
	// Default to MySQL for now
	return &DatabaseService{
		driver:  NewMySQLDriver(),
		SnapDir: "/var/lib/sld/snapshots",
	}
}

// SetDriver switches the database driver (mysql or postgres)
func (d *DatabaseService) SetDriver(driverName string) {
	// Close existing
	if d.driver != nil {
		d.driver.Close()
	}

	switch driverName {
	case "postgres":
		d.driver = NewPostgresDriver()
	default:
		d.driver = NewMySQLDriver()
	}
}

// Connect establishes a connection
func (d *DatabaseService) Connect() error {
	// Pass empty config to trigger auto-discovery in driver
	return d.driver.Connect(ConnectionConfig{})
}

// Close closes the database connection
func (d *DatabaseService) Close() {
	d.driver.Close()
}

// ensureConnected reconnects if needed
func (d *DatabaseService) ensureConnected() error {
	if !d.driver.IsConnected() {
		return d.Connect()
	}
	return nil
}

// ListDatabases returns all user databases
func (d *DatabaseService) ListDatabases() ([]string, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.driver.ListDatabases()
}

func (d *DatabaseService) CreateDatabase(name string) error {
	if err := d.ensureConnected(); err != nil {
		return err
	}
	return d.driver.CreateDatabase(name)
}

func (d *DatabaseService) DeleteDatabase(name string) error {
	if err := d.ensureConnected(); err != nil {
		return err
	}
	return d.driver.DeleteDatabase(name)
}

// ListTables returns tables with metadata
func (d *DatabaseService) ListTables(database string) ([]TableInfo, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.driver.ListTables(database)
}

// GetTableColumns returns column info for a table
func (d *DatabaseService) GetTableColumns(database, table string) ([]ColumnInfo, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.driver.GetTableColumns(database, table)
}

// GetTableData returns paginated data from a table
func (d *DatabaseService) GetTableData(database, table string, page, perPage int) (*TableData, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.driver.GetTableData(database, table, page, perPage)
}

// GetTableDataEx returns paginated data with sorting and profiling
func (d *DatabaseService) GetTableDataEx(database, table string, page, perPage int, sortCol, sortOrder string, profile bool) (*TableData, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.driver.GetTableDataEx(database, table, page, perPage, sortCol, sortOrder, profile)
}

// ExecuteQuery executes a SQL query
func (d *DatabaseService) ExecuteQuery(database, query string) (*QueryResult, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}
	return d.driver.ExecuteQuery(database, query)
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

	// Run mysql import
	cmd := exec.Command("mysql", "-u", "root", dbName)
	file, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer file.Close()
	cmd.Stdin = file

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("restore failed: %s", string(output))
	}

	return nil
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

// CloneDatabase creates a copy of a database using mysqldump piped directly to mysql
func (d *DatabaseService) CloneDatabase(source, target string) error {
	if err := d.ensureConnected(); err != nil {
		return err
	}

	// Validate source exists
	var exists int
	err := d.db.QueryRow("SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?", source).Scan(&exists)
	if err != nil || exists == 0 {
		return fmt.Errorf("source database '%s' not found", source)
	}

	// Check target doesn't exist
	err = d.db.QueryRow("SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?", target).Scan(&exists)
	if err == nil && exists > 0 {
		return fmt.Errorf("target database '%s' already exists", target)
	}

	// Create target database
	_, err = d.db.Exec(fmt.Sprintf("CREATE DATABASE `%s`", target))
	if err != nil {
		return fmt.Errorf("failed to create target database: %w", err)
	}

	// Use pipe: mysqldump source | mysql target
	dumpCmd := exec.Command("mysqldump", "-u", "root", source)
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
	return d.driver.GetTableRelationships(database)
}
