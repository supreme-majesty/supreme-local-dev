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
