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
type DatabaseService struct {
	db      *sql.DB
	dsn     string
	SnapDir string
}

// TableInfo represents a database table with metadata
type TableInfo struct {
	Name      string `json:"name"`
	RowCount  int64  `json:"row_count"`
	Engine    string `json:"engine"`
	Collation string `json:"collation"`
	Size      int64  `json:"size"`
	Overhead  int64  `json:"overhead"`
}

// ForeignKeyInfo represents a foreign key relationship
type ForeignKeyInfo struct {
	Table  string `json:"table"`
	Column string `json:"column"`
}

// ColumnInfo represents a table column
type ColumnInfo struct {
	Name       string          `json:"name"`
	Type       string          `json:"type"`
	Nullable   bool            `json:"nullable"`
	Key        string          `json:"key"`
	Default    string          `json:"default"`
	ForeignKey *ForeignKeyInfo `json:"foreign_key,omitempty"`
}

// TableData represents paginated table data
type TableData struct {
	Columns    []ColumnInfo             `json:"columns"`
	Rows       []map[string]interface{} `json:"rows"`
	Total      int64                    `json:"total"`
	Page       int                      `json:"page"`
	PerPage    int                      `json:"per_page"`
	TotalPages int                      `json:"total_pages"`
	QueryTime  float64                  `json:"query_time,omitempty"` // Query execution time in seconds
}

// Snapshot represents a database snapshot
type Snapshot struct {
	ID        string    `json:"id"`
	Database  string    `json:"database"`
	Table     string    `json:"table,omitempty"`
	Filename  string    `json:"filename"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// QueryResult represents a SQL query result
type QueryResult struct {
	Columns         []string                 `json:"columns"`
	Rows            []map[string]interface{} `json:"rows"`
	RowCount        int                      `json:"row_count"`
	AffectedRows    int64                    `json:"affected_rows,omitempty"`
	ExecutionTimeMs int64                    `json:"execution_time_ms"`
}

// NewDatabaseService creates a new database service
func NewDatabaseService() *DatabaseService {
	return &DatabaseService{
		SnapDir: "/var/lib/sld/snapshots",
	}
}

// Connect establishes a connection to MySQL
func (d *DatabaseService) Connect() error {
	var err error

	// 1. Try Environment Variables
	envUser := os.Getenv("SLD_DB_USER")
	envPass := os.Getenv("SLD_DB_PASS")
	envHost := os.Getenv("SLD_DB_HOST")
	envPort := os.Getenv("SLD_DB_PORT")

	if envUser != "" {
		if envHost == "" {
			envHost = "127.0.0.1"
		}
		if envPort == "" {
			envPort = "3306"
		}

		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/", envUser, envPass, envHost, envPort)
		db, err := sql.Open("mysql", dsn)
		if err == nil {
			if err := db.Ping(); err == nil {
				d.db = db
				d.dsn = dsn
				return nil
			}
			db.Close()
		}
	}

	// 2. Try Current OS User (via socket)
	// Many devs have 'alice'@'localhost' with auth_socket or no pass
	currentUser := os.Getenv("USER")
	if currentUser != "" && currentUser != "root" {
		// Try common socket locations
		socketPaths := []string{
			"/var/run/mysqld/mysqld.sock",
			"/tmp/mysql.sock",
			"/var/lib/mysql/mysql.sock",
		}

		for _, sock := range socketPaths {
			if _, err := os.Stat(sock); err == nil {
				dsn := fmt.Sprintf("%s@unix(%s)/", currentUser, sock)
				db, err := sql.Open("mysql", dsn)
				if err == nil {
					if err := db.Ping(); err == nil {
						d.db = db
						d.dsn = dsn
						return nil
					}
					db.Close()
				}
			}
		}
	}

	// 3. Try Root via Socket (Default for system installs)
	socketPaths := []string{
		"/var/run/mysqld/mysqld.sock",
		"/tmp/mysql.sock",
		"/var/lib/mysql/mysql.sock",
	}

	for _, sock := range socketPaths {
		if _, err := os.Stat(sock); err == nil {
			dsn := fmt.Sprintf("root@unix(%s)/", sock)
			db, err := sql.Open("mysql", dsn)
			if err == nil {
				if err := db.Ping(); err == nil {
					d.db = db
					d.dsn = dsn
					return nil
				}
				db.Close()
			}
		}
	}

	// 4. Try Root via TCP (Default for old MySQL/Homebrew?)
	dsn := "root@tcp(127.0.0.1:3306)/"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		// Return a helpful error message with context
		return fmt.Errorf("failed to connect to MySQL. Tried env vars, user '%s', and root. Error: %w. Try setting SLD_DB_USER environment variable", currentUser, err)
	}

	d.db = db
	d.dsn = dsn
	return nil
}

// Close closes the database connection
func (d *DatabaseService) Close() {
	if d.db != nil {
		d.db.Close()
	}
}

// ensureConnected reconnects if needed
func (d *DatabaseService) ensureConnected() error {
	if d.db == nil {
		return d.Connect()
	}
	if err := d.db.Ping(); err != nil {
		d.db.Close()
		return d.Connect()
	}
	return nil
}

// ListDatabases returns all user databases (excludes system DBs)
func (d *DatabaseService) ListDatabases() ([]string, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}

	rows, err := d.db.Query("SHOW DATABASES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	systemDBs := map[string]bool{
		"information_schema": true,
		"mysql":              true,
		"performance_schema": true,
		"sys":                true,
	}

	var databases []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		if !systemDBs[name] {
			databases = append(databases, name)
		}
	}

	return databases, nil
}

// ListTables returns tables with metadata for a database
func (d *DatabaseService) ListTables(database string) ([]TableInfo, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}

	// Switch to database
	if _, err := d.db.Exec("USE " + database); err != nil {
		return nil, fmt.Errorf("failed to use database %s: %w", database, err)
	}

	query := `
		SELECT 
			TABLE_NAME, 
			COALESCE(TABLE_ROWS, 0) as row_count,
			COALESCE(ENGINE, '') as engine,
			COALESCE(TABLE_COLLATION, '') as collation,
			COALESCE(DATA_LENGTH + INDEX_LENGTH, 0) as size,
			COALESCE(DATA_FREE, 0) as overhead
		FROM information_schema.TABLES 
		WHERE TABLE_SCHEMA = ? AND TABLE_TYPE = 'BASE TABLE'
		ORDER BY TABLE_NAME
	`

	rows, err := d.db.Query(query, database)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []TableInfo
	for rows.Next() {
		var t TableInfo
		if err := rows.Scan(&t.Name, &t.RowCount, &t.Engine, &t.Collation, &t.Size, &t.Overhead); err != nil {
			continue
		}
		tables = append(tables, t)
	}

	return tables, nil
}

// GetTableColumns returns column info for a table
func (d *DatabaseService) GetTableColumns(database, table string) ([]ColumnInfo, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}

	if _, err := d.db.Exec("USE " + database); err != nil {
		return nil, err
	}

	// 1. Get Foreign Keys
	fks := make(map[string]ForeignKeyInfo)
	fkQuery := `
		SELECT COLUMN_NAME, REFERENCED_TABLE_NAME, REFERENCED_COLUMN_NAME 
		FROM information_schema.KEY_COLUMN_USAGE 
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND REFERENCED_TABLE_NAME IS NOT NULL
	`
	fkRows, err := d.db.Query(fkQuery, database, table)
	if err == nil {
		defer fkRows.Close()
		for fkRows.Next() {
			var colName, refTable, refCol string
			if err := fkRows.Scan(&colName, &refTable, &refCol); err == nil {
				fks[colName] = ForeignKeyInfo{
					Table:  refTable,
					Column: refCol,
				}
			}
		}
	}

	// 2. Get Column Details
	// Quote table name to handle special characters/keywords
	rows, err := d.db.Query(fmt.Sprintf("DESCRIBE `%s`", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var field, colType, null, key string
		var defaultVal, extra sql.NullString
		if err := rows.Scan(&field, &colType, &null, &key, &defaultVal, &extra); err != nil {
			continue
		}

		colInfo := ColumnInfo{
			Name:     field,
			Type:     colType,
			Nullable: null == "YES",
			Key:      key,
			Default:  defaultVal.String,
		}

		if fk, ok := fks[field]; ok {
			colInfo.ForeignKey = &fk
		}

		columns = append(columns, colInfo)
	}

	return columns, nil
}

// GetTableData returns paginated data from a table
func (d *DatabaseService) GetTableData(database, table string, page, perPage int) (*TableData, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}

	if _, err := d.db.Exec("USE " + database); err != nil {
		return nil, err
	}

	// Get columns
	columns, err := d.GetTableColumns(database, table)
	if err != nil {
		return nil, err
	}

	// Get total count
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM `%s`", table)
	if err := d.db.QueryRow(countQuery).Scan(&total); err != nil {
		return nil, err
	}

	// Calculate pagination
	if perPage <= 0 {
		perPage = 50
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * perPage
	totalPages := int((total + int64(perPage) - 1) / int64(perPage))

	// Fetch rows
	dataQuery := fmt.Sprintf("SELECT * FROM `%s` LIMIT %d OFFSET %d", table, perPage, offset)
	rows, err := d.db.Query(dataQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Get column names
	colNames, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	// Scan rows into maps
	var data []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(colNames))
		valuePtrs := make([]interface{}, len(colNames))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		row := make(map[string]interface{})
		for i, col := range colNames {
			// Use schema column name if available to ensure consistency with frontend
			key := col
			if i < len(columns) {
				key = columns[i].Name
			}

			val := values[i]
			// Convert byte slices to strings for JSON
			if b, ok := val.([]byte); ok {
				row[key] = string(b)
			} else {
				row[key] = val
			}
		}
		data = append(data, row)
	}

	return &TableData{
		Columns:    columns,
		Rows:       data,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}, nil
}

// GetTableDataEx returns paginated data with sorting and optional profiling
func (d *DatabaseService) GetTableDataEx(database, table string, page, perPage int, sortCol, sortOrder string, profile bool) (*TableData, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}

	if _, err := d.db.Exec("USE " + database); err != nil {
		return nil, err
	}

	// Get columns for validation and response
	columns, err := d.GetTableColumns(database, table)
	if err != nil {
		return nil, err
	}

	// Validate sort column against schema to prevent SQL injection
	validSortCol := ""
	if sortCol != "" {
		for _, col := range columns {
			if col.Name == sortCol {
				validSortCol = sortCol
				break
			}
		}
	}

	// Validate sort order
	if sortOrder != "DESC" {
		sortOrder = "ASC"
	}

	// Get total count
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM `%s`", table)
	if err := d.db.QueryRow(countQuery).Scan(&total); err != nil {
		return nil, err
	}

	// Calculate pagination
	if perPage <= 0 {
		perPage = 50
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * perPage
	totalPages := int((total + int64(perPage) - 1) / int64(perPage))

	// Build data query
	dataQuery := fmt.Sprintf("SELECT * FROM `%s`", table)
	if validSortCol != "" {
		dataQuery += fmt.Sprintf(" ORDER BY `%s` %s", validSortCol, sortOrder)
	}
	dataQuery += fmt.Sprintf(" LIMIT %d OFFSET %d", perPage, offset)

	var queryTime float64

	// Enable profiling if requested
	if profile {
		d.db.Exec("SET profiling = 1")
	}

	// Execute query
	rows, err := d.db.Query(dataQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Get profiling data
	if profile {
		var profileRows *sql.Rows
		profileRows, err = d.db.Query("SHOW PROFILE")
		if err == nil {
			defer profileRows.Close()
			for profileRows.Next() {
				var status string
				var duration float64
				if err := profileRows.Scan(&status, &duration); err == nil {
					queryTime += duration
				}
			}
		}
		d.db.Exec("SET profiling = 0")
	}

	// Get column names
	colNames, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	// Scan rows into maps
	var data []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(colNames))
		valuePtrs := make([]interface{}, len(colNames))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		row := make(map[string]interface{})
		for i, col := range colNames {
			key := col
			if i < len(columns) {
				key = columns[i].Name
			}

			val := values[i]
			if b, ok := val.([]byte); ok {
				row[key] = string(b)
			} else {
				row[key] = val
			}
		}
		data = append(data, row)
	}

	return &TableData{
		Columns:    columns,
		Rows:       data,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
		QueryTime:  queryTime,
	}, nil
}

// ExecuteQuery executes a SQL query (read or write)
func (d *DatabaseService) ExecuteQuery(database, query string) (*QueryResult, error) {
	if err := d.ensureConnected(); err != nil {
		return nil, err
	}

	if _, err := d.db.Exec("USE " + database); err != nil {
		return nil, err
	}

	// Start timing
	startTime := time.Now()

	// Determine usage
	trimmed := strings.TrimSpace(strings.ToUpper(query))
	isSelect := strings.HasPrefix(trimmed, "SELECT") || strings.HasPrefix(trimmed, "SHOW") || strings.HasPrefix(trimmed, "DESCRIBE") || strings.HasPrefix(trimmed, "EXPLAIN")

	if !isSelect {
		res, err := d.db.Exec(query)
		elapsedMs := time.Since(startTime).Milliseconds()
		if err != nil {
			return nil, err
		}
		affected, _ := res.RowsAffected()
		return &QueryResult{
			RowCount:        int(affected),
			Rows:            []map[string]interface{}{},
			Columns:         []string{},
			AffectedRows:    affected,
			ExecutionTimeMs: elapsedMs,
		}, nil
	}

	// Handle SELECT-like queries
	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	colNames, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var data []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(colNames))
		valuePtrs := make([]interface{}, len(colNames))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		row := make(map[string]interface{})
		for i, col := range colNames {
			val := values[i]
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		data = append(data, row)
	}

	elapsedMs := time.Since(startTime).Milliseconds()
	return &QueryResult{
		Columns:         colNames,
		Rows:            data,
		RowCount:        len(data),
		ExecutionTimeMs: elapsedMs,
	}, nil
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
