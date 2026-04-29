package services

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type MySQLDriver struct {
	db  *sql.DB
	dsn string
}

func NewMySQLDriver() *MySQLDriver {
	return &MySQLDriver{}
}

func (d *MySQLDriver) Connect(config ConnectionConfig) error {
	var dsn string

	// If config is provided, use it
	if config.User != "" {
		host := config.Host
		if host == "" {
			host = "127.0.0.1"
		}
		port := config.Port
		if port == "" {
			port = "3306"
		}
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/", config.User, config.Password, host, port)
	} else {
		// Auto-discovery logic (copied from original database.go)

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
			dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/", envUser, envPass, envHost, envPort)
		} else {
			// 2. Try Current OS User (socket)
			currentUser := os.Getenv("USER")
			socketFound := false

			socketPaths := []string{
				"/var/run/mysqld/mysqld.sock",
				"/tmp/mysql.sock",
				"/var/lib/mysql/mysql.sock",
			}

			if currentUser != "" && currentUser != "root" {
				for _, sock := range socketPaths {
					if _, err := os.Stat(sock); err == nil {
						dsn = fmt.Sprintf("%s@unix(%s)/", currentUser, sock)
						socketFound = true
						break
					}
				}
			}

			// 3. Try Root via Socket
			if !socketFound {
				for _, sock := range socketPaths {
					if _, err := os.Stat(sock); err == nil {
						dsn = fmt.Sprintf("root@unix(%s)/", sock)
						socketFound = true
						break
					}
				}
			}

			// 4. Fallback TCP
			if !socketFound {
				dsn = "root@tcp(127.0.0.1:3306)/"
			}
		}
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return err
	}

	d.db = db
	d.dsn = dsn
	return nil
}

func (d *MySQLDriver) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

func (d *MySQLDriver) IsConnected() bool {
	return d.db != nil && d.db.Ping() == nil
}

func (d *MySQLDriver) ListDatabases() ([]string, error) {
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

func (d *MySQLDriver) CreateDatabase(name string) error {
	_, err := d.db.Exec(fmt.Sprintf("CREATE DATABASE `%s`", name))
	return err
}

func (d *MySQLDriver) DeleteDatabase(name string) error {
	_, err := d.db.Exec(fmt.Sprintf("DROP DATABASE `%s`", name))
	return err
}

func (d *MySQLDriver) RenameDatabase(oldName, newName string) error {
	// MySQL doesn't support RENAME DATABASE directly.
	// We must CREATE new, RENAME TABLEs, DROP old.
	if err := d.CreateDatabase(newName); err != nil {
		return fmt.Errorf("failed to create new database: %w", err)
	}

	tables, err := d.ListTables(oldName)
	if err != nil {
		return fmt.Errorf("failed to list tables in old database: %w", err)
	}

	for _, t := range tables {
		query := fmt.Sprintf("RENAME TABLE `%s`.`%s` TO `%s`.`%s`", oldName, t.Name, newName, t.Name)
		if _, err := d.db.Exec(query); err != nil {
			return fmt.Errorf("failed to move table %s: %w", t.Name, err)
		}
	}

	if err := d.DeleteDatabase(oldName); err != nil {
		return fmt.Errorf("failed to delete old database: %w", err)
	}

	return nil
}

func (d *MySQLDriver) ListTables(database string) ([]TableInfo, error) {
	// USE db
	if _, err := d.db.Exec("USE " + database); err != nil {
		return nil, err
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

	tables := make([]TableInfo, 0)
	for rows.Next() {
		var t TableInfo
		if err := rows.Scan(&t.Name, &t.RowCount, &t.Engine, &t.Collation, &t.Size, &t.Overhead); err != nil {
			continue
		}
		tables = append(tables, t)
	}
	return tables, nil
}

func (d *MySQLDriver) GetTableColumns(database, table string) ([]ColumnInfo, error) {
	if _, err := d.db.Exec("USE " + database); err != nil {
		return nil, err
	}

	// Foreign Keys
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
				fks[colName] = ForeignKeyInfo{Table: refTable, Column: refCol}
			}
		}
	}

	// Columns
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

func (d *MySQLDriver) GetTableData(database, table string, page, perPage int) (*TableData, error) {
	return d.GetTableDataEx(database, table, page, perPage, "", "", false)
}

func (d *MySQLDriver) GetTableDataEx(database, table string, page, perPage int, sortCol, sortOrder string, profile bool) (*TableData, error) {
	if _, err := d.db.Exec("USE " + database); err != nil {
		return nil, err
	}

	// Total count
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM `%s`", table)
	if err := d.db.QueryRow(countQuery).Scan(&total); err != nil {
		return nil, err
	}

	// Pagination
	if perPage <= 0 {
		perPage = 50
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * perPage
	totalPages := int((total + int64(perPage) - 1) / int64(perPage))

	// Validate sort
	if sortOrder != "DESC" {
		sortOrder = "ASC"
	}
	// Simplistic sort validation (driver should ideally validate against columns)

	// Query
	dataQuery := fmt.Sprintf("SELECT * FROM `%s`", table)
	if sortCol != "" {
		dataQuery += fmt.Sprintf(" ORDER BY `%s` %s", sortCol, sortOrder)
	}
	dataQuery += fmt.Sprintf(" LIMIT %d OFFSET %d", perPage, offset)

	var queryTime float64
	if profile {
		d.db.Exec("SET profiling = 1")
	}

	rows, err := d.db.Query(dataQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if profile {
		// Fetch profile... simplified for driver
		// Implementation similar to original
		d.db.Exec("SET profiling = 0")
	}

	colNames, _ := rows.Columns()

	// Fetch column info for frontend mapping
	columns, _ := d.GetTableColumns(database, table)

	var data []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(colNames))
		valuePtrs := make([]interface{}, len(colNames))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		rows.Scan(valuePtrs...)

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

func (d *MySQLDriver) ExecuteQuery(database, query string) (*QueryResult, error) {
	if _, err := d.db.Exec("USE " + database); err != nil {
		return nil, err
	}

	startTime := time.Now()
	trimmed := strings.TrimSpace(strings.ToUpper(query))
	isSelect := strings.HasPrefix(trimmed, "SELECT") || strings.HasPrefix(trimmed, "SHOW") || strings.HasPrefix(trimmed, "DESCRIBE") || strings.HasPrefix(trimmed, "EXPLAIN")

	if !isSelect {
		res, err := d.db.Exec(query)
		elapsed := time.Since(startTime).Milliseconds()
		if err != nil {
			return nil, err
		}
		affected, _ := res.RowsAffected()
		return &QueryResult{
			AffectedRows:    affected,
			ExecutionTimeMs: elapsed,
		}, nil
	}

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	var data []map[string]interface{}
	// Scan logic...
	// For brevity, similar strictly to GetTableData scan
	for rows.Next() {
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		rows.Scan(valuePtrs...)
		row := make(map[string]interface{})
		for i, col := range cols {
			val := values[i]
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		data = append(data, row)
	}

	return &QueryResult{
		Columns:         cols,
		Rows:            data,
		RowCount:        len(data),
		ExecutionTimeMs: time.Since(startTime).Milliseconds(),
	}, nil
}

func (d *MySQLDriver) GetForeignValues(database, table, column string) ([]string, error) {
	// Not implemented in original database.go, but interface requires it?
	// Original code didn't have this.
	return []string{}, nil
}

func (d *MySQLDriver) GetTableRelationships(database string) ([]TableRelationship, error) {
	query := `
		SELECT 
			TABLE_NAME as from_table, 
			COLUMN_NAME as from_column, 
			REFERENCED_TABLE_NAME as to_table, 
			REFERENCED_COLUMN_NAME as to_column
		FROM information_schema.KEY_COLUMN_USAGE 
		WHERE TABLE_SCHEMA = ? AND REFERENCED_TABLE_NAME IS NOT NULL
	`

	rows, err := d.db.Query(query, database)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var relationships []TableRelationship
	for rows.Next() {
		var r TableRelationship
		if err := rows.Scan(&r.FromTable, &r.FromColumn, &r.ToTable, &r.ToColumn); err != nil {
			continue
		}
		relationships = append(relationships, r)
	}
	return relationships, nil
}

func (d *MySQLDriver) CreateSnapshot(database, table string, filepath string) error {
	args := []string{"-u", "root", database}
	if table != "" {
		args = append(args, table)
	}
	cmd := exec.Command("mysqldump", args...)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("mysqldump failed: %w", err)
	}
	return os.WriteFile(filepath, output, 0644)
}

func (d *MySQLDriver) RestoreSnapshot(database string, filepath string) error {
	cmd := exec.Command("mysql", "-u", "root", database)
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
func (d *MySQLDriver) GetTableIndexes(database, table string) ([]IndexInfo, error) {
	if _, err := d.db.Exec("USE " + database); err != nil {
		return nil, err
	}

	rows, err := d.db.Query(fmt.Sprintf("SHOW INDEX FROM `%s`", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	indexMap := make(map[string]*IndexInfo)
	for rows.Next() {
		var table, nonUnique, keyName, seq, column, collation, cardinality, subPart, packed, null, indexType, comment, indexComment string
		if err := rows.Scan(&table, &nonUnique, &keyName, &seq, &column, &collation, &cardinality, &subPart, &packed, &null, &indexType, &comment, &indexComment); err != nil {
			continue
		}

		if _, ok := indexMap[keyName]; !ok {
			indexMap[keyName] = &IndexInfo{
				Name:    keyName,
				Columns: []string{},
				Unique:  nonUnique == "0",
				Primary: keyName == "PRIMARY",
				Type:    indexType,
			}
		}
		indexMap[keyName].Columns = append(indexMap[keyName].Columns, column)
	}

	var indexes []IndexInfo
	for _, idx := range indexMap {
		indexes = append(indexes, *idx)
	}
	return indexes, nil
}

func (m *MySQLDriver) Maintenance(database string, tables []string, operation string) ([]MaintenanceResult, error) {
	if !m.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	if len(tables) == 0 {
		return nil, nil
	}

	validOps := map[string]bool{
		"ANALYZE":  true,
		"CHECK":    true,
		"OPTIMIZE": true,
		"REPAIR":   true,
	}
	op := strings.ToUpper(operation)
	if !validOps[op] {
		return nil, fmt.Errorf("invalid maintenance operation: %s", operation)
	}

	// Prepare table list with backticks
	quotedTables := make([]string, len(tables))
	for i, t := range tables {
		quotedTables[i] = fmt.Sprintf("`%s`.`%s`", database, t)
	}

	query := fmt.Sprintf("%s TABLE %s", op, strings.Join(quotedTables, ", "))
	rows, err := m.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []MaintenanceResult
	for rows.Next() {
		var res MaintenanceResult
		var tableRaw string
		if err := rows.Scan(&tableRaw, &res.Operation, &res.MsgType, &res.MsgText); err != nil {
			return nil, err
		}
		// Table name might be fully qualified, extract base
		parts := strings.Split(tableRaw, ".")
		res.Table = parts[len(parts)-1]
		results = append(results, res)
	}

	return results, nil
}

func (m *MySQLDriver) GlobalSearch(database string, query string) ([]SearchResult, error) {
	if !m.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	// 1. Get all tables and their columns that are searchable (string/text)
	tables, err := m.ListTables(database)
	if err != nil {
		return nil, err
	}

	var searchResults []SearchResult

	for _, table := range tables {
		// Get columns for this table
		cols, err := m.GetTableColumns(database, table.Name)
		if err != nil {
			continue
		}

		var searchableCols []string
		for _, col := range cols {
			typ := strings.ToLower(col.Type)
			if strings.Contains(typ, "char") || strings.Contains(typ, "text") || strings.Contains(typ, "string") {
				searchableCols = append(searchableCols, col.Name)
			}
		}

		if len(searchableCols) == 0 {
			continue
		}

		// 2. Build search query for this table
		var conditions []string
		for _, col := range searchableCols {
			conditions = append(conditions, fmt.Sprintf("`%s` LIKE '%%%s%%'", col, query))
		}

		searchSQL := fmt.Sprintf("SELECT * FROM `%s`.`%s` WHERE %s LIMIT 10", database, table.Name, strings.Join(conditions, " OR "))
		
		rows, err := m.db.Query(searchSQL)
		if err != nil {
			continue
		}

		// Process results
		colsNames, _ := rows.Columns()
		count := 0
		var matches []map[string]interface{}

		for rows.Next() {
			count++
			row := make([]interface{}, len(colsNames))
			valPtrs := make([]interface{}, len(colsNames))
			for i := range row {
				valPtrs[i] = &row[i]
			}

			if err := rows.Scan(valPtrs...); err == nil {
				match := make(map[string]interface{})
				for i, colName := range colsNames {
					val := row[i]
					if b, ok := val.([]byte); ok {
						match[colName] = string(b)
					} else {
						match[colName] = val
					}
				}
				matches = append(matches, match)
			}
		}
		rows.Close()

		if count > 0 {
			searchResults = append(searchResults, SearchResult{
				Table:       table.Name,
				ColumnCount: len(searchableCols),
				RowCount:    count,
				Matches:     matches,
			})
		}
	}

	return searchResults, nil
}

func (m *MySQLDriver) GetCollations() ([]CollationInfo, error) {
	if !m.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	rows, err := m.db.Query("SHOW COLLATION")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var collations []CollationInfo
	for rows.Next() {
		var c CollationInfo
		var charset, id, isDefault, compiled, sortlen string
		if err := rows.Scan(&c.Name, &charset, &id, &isDefault, &compiled, &sortlen); err != nil {
			return nil, err
		}
		c.Charset = charset
		collations = append(collations, c)
	}

	return collations, nil
}

func (m *MySQLDriver) GetDatabaseSettings(database string) (*DatabaseSettings, error) {
	if !m.IsConnected() {
		return nil, fmt.Errorf("not connected")
	}

	query := fmt.Sprintf("SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = '%s'", database)
	var settings DatabaseSettings
	err := m.db.QueryRow(query).Scan(&settings.Charset, &settings.Collation)
	if err != nil {
		return nil, err
	}

	return &settings, nil
}

func (m *MySQLDriver) ExplainQuery(database, query string) (*QueryExplanation, error) {
	if _, err := m.db.Exec("USE " + database); err != nil {
		return nil, err
	}

	explainQuery := "EXPLAIN " + query
	rows, err := m.db.Query(explainQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	var plan []map[string]interface{}
	var estRows int64
	
	for rows.Next() {
		columns := make([]interface{}, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}

		if err := rows.Scan(columnPointers...); err != nil {
			return nil, err
		}

		m := make(map[string]interface{})
		for i, colName := range cols {
			val := columns[i]
			if b, ok := val.([]byte); ok {
				m[colName] = string(b)
			} else {
				m[colName] = val
			}
		}
		plan = append(plan, m)

		// Parse estimated rows
		if rowVal, ok := m["rows"]; ok {
			if r, ok := rowVal.(int64); ok {
				estRows += r
			} else if rs, ok := rowVal.(string); ok {
				var r int64
				fmt.Sscanf(rs, "%d", &r)
				estRows += r
			}
		}
	}

	// Simple heuristic analysis
	analysis := []string{}
	recommendations := []string{}
	complexity := "Simple"

	for _, row := range plan {
		accessType, _ := row["type"].(string)
		extra, _ := row["Extra"].(string)

		if accessType == "ALL" {
			analysis = append(analysis, fmt.Sprintf("Full Table Scan detected on %s.", row["table"]))
			recommendations = append(recommendations, fmt.Sprintf("Add an index to %s.", row["table"]))
			complexity = "Complex"
		}
		if strings.Contains(extra, "Using filesort") {
			analysis = append(analysis, "Sorting requires a temporary file (filesort).")
			recommendations = append(recommendations, "Ensure ORDER BY columns are indexed.")
			complexity = "Moderate"
		}
	}

	return &QueryExplanation{
		Query:           query,
		ExecutionPlan:   plan,
		Analysis:        analysis,
		Recommendations: recommendations,
		EstimatedRows:   estRows,
		Complexity:      complexity,
	}, nil
}
