package services

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
)

type MySQLDriver struct {
	db  *sql.DB
	dsn string
	cfg ConnectionConfig

	mu        sync.Mutex // guards activeTxs and perDB
	activeTxs map[string]*sql.Tx
	// perDB holds one small pool per database, opened with that database as
	// the connection default. Running "USE db" on the shared pool only
	// changes whichever connection it happens to get, so the next statement
	// could run against another database.
	perDB map[string]*sql.DB
}

func NewMySQLDriver() *MySQLDriver {
	return &MySQLDriver{
		activeTxs: make(map[string]*sql.Tx),
		perDB:     make(map[string]*sql.DB),
	}
}

// dbFor returns a pool whose connections all have database selected. An empty
// name returns the server-level pool.
func (d *MySQLDriver) dbFor(database string) (*sql.DB, error) {
	if d.db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	if database == "" {
		return d.db, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if db, ok := d.perDB[database]; ok {
		return db, nil
	}
	cfg, err := mysql.ParseDSN(d.dsn)
	if err != nil {
		return nil, err
	}
	cfg.DBName = database
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)
	d.perDB[database] = db
	return db, nil
}

// closePerDB closes the per-database pools (on reconnect or close).
func (d *MySQLDriver) closePerDB() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for name, db := range d.perDB {
		_ = db.Close()
		delete(d.perDB, name)
	}
}

func (d *MySQLDriver) tx(txId string) (*sql.Tx, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, ok := d.activeTxs[txId]
	return tx, ok
}

func (d *MySQLDriver) takeTx(txId string) (*sql.Tx, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, ok := d.activeTxs[txId]
	delete(d.activeTxs, txId)
	return tx, ok
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func stateDir() string {
	if dir := os.Getenv("SLD_STATE_DIR"); dir != "" {
		return dir
	}
	return "/var/lib/sld"
}

// MySQLHost is the database host the manager should use (compose sets SLD_DB_HOST).
func MySQLHost() string { return envOr("SLD_DB_HOST", "127.0.0.1") }

// MySQLPort is the database TCP port.
func MySQLPort() string { return envOr("SLD_DB_PORT", "3306") }

// MySQLUser is the database user.
func MySQLUser() string { return envOr("SLD_DB_USER", "root") }

// MySQLPassword is the database password (empty for local socket/root).
func MySQLPassword() string { return os.Getenv("SLD_DB_PASS") }

// MySQLDSN builds a go-sql-driver DSN from environment, used by the daemon in Docker.
func MySQLDSN() string {
	user := MySQLUser()
	pass := MySQLPassword()
	host := MySQLHost()
	port := MySQLPort()
	if pass == "" {
		return fmt.Sprintf("%s@tcp(%s:%s)/?timeout=5s&readTimeout=15s&writeTimeout=15s", user, host, port)
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/?timeout=5s&readTimeout=15s&writeTimeout=15s", user, pass, host, port)
}

func (d *MySQLDriver) Connect(config ConnectionConfig) error {
	var dsn string

	// If config is provided, use it
	if config.User != "" || config.Host != "" {
		user := config.User
		if user == "" {
			user = "root"
		}
		host := config.Host
		if host == "" {
			host = "127.0.0.1"
		}
		port := config.Port
		if port == "" {
			port = "3306"
		}
		if config.Password == "" {
			dsn = fmt.Sprintf("%s@tcp(%s:%s)/?timeout=5s&readTimeout=15s&writeTimeout=15s", user, host, port)
		} else {
			dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/?timeout=5s&readTimeout=15s&writeTimeout=15s", user, config.Password, host, port)
		}
	} else {
		// Auto-discovery logic (copied from original database.go)

		if os.Getenv("SLD_DB_HOST") != "" || os.Getenv("SLD_DB_USER") != "" || os.Getenv("SLD_DB_PASS") != "" {
			dsn = MySQLDSN()
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
						dsn = fmt.Sprintf("%s@unix(%s)/?timeout=5s&readTimeout=15s&writeTimeout=15s", currentUser, sock)
						socketFound = true
						break
					}
				}
			}

			// 3. Try Root via Socket
			if !socketFound {
				for _, sock := range socketPaths {
					if _, err := os.Stat(sock); err == nil {
						dsn = fmt.Sprintf("root@unix(%s)/?timeout=5s&readTimeout=15s&writeTimeout=15s", sock)
						socketFound = true
						break
					}
				}
			}

			// 4. Fallback TCP
			if !socketFound {
				dsn = "root@tcp(127.0.0.1:3306)/?timeout=5s&readTimeout=15s&writeTimeout=15s"
			}
		}
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return err
	}

	d.closePerDB()
	if d.db != nil {
		_ = d.db.Close()
	}
	d.db = db
	d.dsn = dsn
	d.cfg = config
	return nil
}

func (d *MySQLDriver) Close() error {
	d.closePerDB()
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

	var databases []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		databases = append(databases, name)
	}
	return FilterUserDatabases(databases), nil
}

func (d *MySQLDriver) TableCounts() (map[string]int, error) {
	if d.db == nil {
		return nil, nil
	}
	rows, err := d.db.Query(`
		SELECT TABLE_SCHEMA, COUNT(*)
		FROM information_schema.TABLES
		WHERE TABLE_TYPE = 'BASE TABLE'
		GROUP BY TABLE_SCHEMA
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var schema string
		var count int
		if err := rows.Scan(&schema, &count); err != nil {
			continue
		}
		counts[schema] = count
	}
	return counts, nil
}

func (d *MySQLDriver) CreateDatabase(name string) error {
	_, err := d.db.Exec("CREATE DATABASE " + quoteMy(name))
	return err
}

func (d *MySQLDriver) DeleteDatabase(name string) error {
	_, err := d.db.Exec("DROP DATABASE " + quoteMy(name))
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
		query := "RENAME TABLE " + quoteMy(oldName) + "." + quoteMy(t.Name) + " TO " + quoteMy(newName) + "." + quoteMy(t.Name)
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
	db, err := d.dbFor(database)
	if err != nil {
		return nil, err
	}

	// Foreign Keys
	fks := make(map[string]ForeignKeyInfo)
	fkQuery := `
		SELECT COLUMN_NAME, REFERENCED_TABLE_NAME, REFERENCED_COLUMN_NAME 
		FROM information_schema.KEY_COLUMN_USAGE 
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND REFERENCED_TABLE_NAME IS NOT NULL
	`
	fkRows, err := db.Query(fkQuery, database, table)
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
	rows, err := db.Query("DESCRIBE " + quoteMy(table))
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
	db, err := d.dbFor(database)
	if err != nil {
		return nil, err
	}

	// Total count
	var total int64
	countQuery := "SELECT COUNT(*) FROM " + quoteMy(table)
	if err := db.QueryRow(countQuery).Scan(&total); err != nil {
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
	dataQuery := "SELECT * FROM " + quoteMy(table)
	if sortCol != "" {
		dataQuery += " ORDER BY " + quoteMy(sortCol) + " " + sortOrder
	}
	dataQuery += fmt.Sprintf(" LIMIT %d OFFSET %d", perPage, offset)

	var queryTime float64
	start := time.Now()
	rows, err := db.Query(dataQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if profile {
		queryTime = float64(time.Since(start).Microseconds()) / 1000
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

// ExecuteQuery executes a SQL query
func (d *MySQLDriver) ExecuteQuery(database, query string, txId string) (*QueryResult, error) {
	if d.db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	// A transaction already runs on one connection with its database
	// selected; otherwise use the pool for this database.
	type runner interface {
		Query(query string, args ...interface{}) (*sql.Rows, error)
		Exec(query string, args ...interface{}) (sql.Result, error)
	}
	var run runner
	if txId != "" {
		tx, ok := d.tx(txId)
		if !ok {
			return nil, fmt.Errorf("transaction %s not found or expired", txId)
		}
		run = tx
	} else {
		db, err := d.dbFor(database)
		if err != nil {
			return nil, err
		}
		run = db
	}

	start := time.Now()

	var rows *sql.Rows
	var err error
	if returnsRows(query) {
		rows, err = run.Query(query)
	} else {
		var result sql.Result
		result, err = run.Exec(query)
		if err == nil {
			affected, _ := result.RowsAffected()
			return &QueryResult{
				AffectedRows:    affected,
				ExecutionTimeMs: time.Since(start).Milliseconds(),
			}, nil
		}
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, _ := rows.Columns()
	data := make([]map[string]interface{}, 0)
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
		ExecutionTimeMs: time.Since(start).Milliseconds(),
	}, nil
}

func (d *MySQLDriver) BeginTransaction(database string) (string, error) {
	if d.db == nil {
		return "", fmt.Errorf("database not connected")
	}

	db, err := d.dbFor(database)
	if err != nil {
		return "", err
	}
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}

	txId := fmt.Sprintf("tx_%d", time.Now().UnixNano())
	d.mu.Lock()
	d.activeTxs[txId] = tx
	d.mu.Unlock()
	return txId, nil
}

func (d *MySQLDriver) CommitTransaction(txId string) error {
	tx, ok := d.takeTx(txId)
	if !ok {
		return fmt.Errorf("transaction %s not found", txId)
	}
	return tx.Commit()
}

func (d *MySQLDriver) RollbackTransaction(txId string) error {
	tx, ok := d.takeTx(txId)
	if !ok {
		return fmt.Errorf("transaction %s not found", txId)
	}
	return tx.Rollback()
}

// ExecArgs runs one statement with bound parameters, inside txId when given.
func (d *MySQLDriver) ExecArgs(database, query string, args []interface{}, txId string) (int64, error) {
	var res sql.Result
	var err error
	if txId != "" {
		tx, ok := d.tx(txId)
		if !ok {
			return 0, fmt.Errorf("transaction %s not found", txId)
		}
		res, err = tx.Exec(query, args...)
	} else {
		db, dbErr := d.dbFor(database)
		if dbErr != nil {
			return 0, dbErr
		}
		res, err = db.Exec(query, args...)
	}
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (d *MySQLDriver) BatchInsert(database, table string, columns []string, data [][]interface{}, txId string) error {
	db, err := d.dbFor(database)
	if err != nil {
		return err
	}

	if len(data) == 0 {
		return nil
	}

	// Prepare column names
	colNames := make([]string, len(columns))
	for i, col := range columns {
		colNames[i] = quoteMy(col)
	}
	colsStr := strings.Join(colNames, ", ")

	// Use transaction if provided
	var tx *sql.Tx
	if txId != "" {
		var ok bool
		tx, ok = d.tx(txId)
		if !ok {
			return fmt.Errorf("transaction %s not found", txId)
		}
	}

	// Batch size
	const batchSize = 500
	for i := 0; i < len(data); i += batchSize {
		end := i + batchSize
		if end > len(data) {
			end = len(data)
		}
		batch := data[i:end]

		placeholders := make([]string, len(batch))
		values := make([]interface{}, 0, len(batch)*len(columns))
		for j, row := range batch {
			rowPlaceholders := make([]string, len(columns))
			for k := range columns {
				rowPlaceholders[k] = "?"
			}
			placeholders[j] = "(" + strings.Join(rowPlaceholders, ", ") + ")"
			values = append(values, row...)
		}

		query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", quoteMy(table), colsStr, strings.Join(placeholders, ", "))

		var err error
		if tx != nil {
			_, err = tx.Exec(query, values...)
		} else {
			_, err = db.Exec(query, values...)
		}

		if err != nil {
			return fmt.Errorf("batch insert failed at index %d: %w", i, err)
		}
	}

	return nil
}

func (d *MySQLDriver) Query(database, query string, args []interface{}) ([]map[string]interface{}, error) {
	db, err := d.dbFor(database)
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}
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
			b, ok := val.([]byte)
			if ok {
				m[colName] = string(b)
			} else {
				m[colName] = val
			}
		}
		results = append(results, m)
	}

	return results, nil
}

func (d *MySQLDriver) GetTableInfo(database, table string) (*TableInfo, error) {
	query := `
		SELECT 
			TABLE_NAME, 
			TABLE_ROWS, 
			ENGINE, 
			TABLE_COLLATION, 
			DATA_LENGTH + INDEX_LENGTH as SIZE,
			DATA_FREE as OVERHEAD
		FROM INFORMATION_SCHEMA.TABLES 
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
	`
	rows, err := d.Query("", query, []interface{}{database, table})
	if err != nil || len(rows) == 0 {
		return nil, fmt.Errorf("table info not found: %v", err)
	}

	row := rows[0]
	info := &TableInfo{
		Name:      row["TABLE_NAME"].(string),
		Engine:    row["ENGINE"].(string),
		Collation: row["TABLE_COLLATION"].(string),
	}

	// Handle numeric types safely
	if v, ok := row["TABLE_ROWS"].(int64); ok {
		info.RowCount = v
	}
	if v, ok := row["SIZE"].(int64); ok {
		info.Size = v
	}
	if v, ok := row["OVERHEAD"].(int64); ok {
		info.Overhead = v
	}

	return info, nil
}

func (d *MySQLDriver) GetStats(database string) (map[string]interface{}, error) {
	if d.db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	stats := make(map[string]interface{})

	// 1. Get Global Status
	rows, err := d.db.Query("SHOW GLOBAL STATUS")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	status := make(map[string]string)
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err == nil {
			status[name] = value
		}
	}
	stats["status"] = status

	// 2. Get Process List
	pRows, err := d.db.Query("SHOW FULL PROCESSLIST")
	if err != nil {
		return nil, err
	}
	defer pRows.Close()

	var processes []map[string]interface{}
	cols, _ := pRows.Columns()
	for pRows.Next() {
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		pRows.Scan(valuePtrs...)
		proc := make(map[string]interface{})
		for i, col := range cols {
			val := values[i]
			if b, ok := val.([]byte); ok {
				proc[col] = string(b)
			} else {
				proc[col] = val
			}
		}
		processes = append(processes, proc)
	}
	stats["processes"] = processes

	return stats, nil
}

func (d *MySQLDriver) GetForeignValues(database, table, column string) ([]string, error) {
	if d.db == nil || table == "" || column == "" {
		return []string{}, nil
	}
	quotedDB := quoteMy(database)
	quotedTable := quoteMy(table)
	quotedCol := quoteMy(column)

	var query string
	if database != "" {
		query = fmt.Sprintf("SELECT DISTINCT CAST(%s AS CHAR) FROM %s.%s WHERE %s IS NOT NULL ORDER BY 1 LIMIT 100", quotedCol, quotedDB, quotedTable, quotedCol)
	} else {
		query = fmt.Sprintf("SELECT DISTINCT CAST(%s AS CHAR) FROM %s WHERE %s IS NOT NULL ORDER BY 1 LIMIT 100", quotedCol, quotedTable, quotedCol)
	}

	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	vals := make([]string, 0)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err == nil {
			vals = append(vals, v)
		}
	}
	return vals, nil
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
	extra := []string{"--single-transaction", "--quick", database}
	if table != "" {
		extra = append(extra, table)
	}
	args, env := buildMySQLCLIArgs(d.cfg, extra...)

	bin := "mysqldump"
	if _, err := exec.LookPath(bin); err != nil {
		if _, err2 := exec.LookPath("mariadb-dump"); err2 == nil {
			bin = "mariadb-dump"
		} else {
			return fmt.Errorf("mysqldump not found in PATH: %w", err)
		}
	}

	outFile, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create snapshot file: %w", err)
	}

	var stderr strings.Builder
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	cmd.Stdout = outFile
	cmd.Stderr = &stderr

	err = cmd.Run()
	_ = outFile.Close()
	if err != nil {
		_ = os.Remove(filepath)
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("mysqldump failed: %s", msg)
		}
		return fmt.Errorf("mysqldump failed: %w", err)
	}
	return nil
}

func (d *MySQLDriver) RestoreSnapshot(database string, filepath string) error {
	// Prefer native mysql / mariadb CLI for full dump compatibility & speed
	bin := ""
	if _, err := exec.LookPath("mysql"); err == nil {
		bin = "mysql"
	} else if _, err := exec.LookPath("mariadb"); err == nil {
		bin = "mariadb"
	}

	if bin != "" {
		reader, err := openMaybeGzip(filepath)
		if err != nil {
			return fmt.Errorf("failed to open SQL file: %w", err)
		}
		defer reader.Close()

		args, env := buildMySQLCLIArgs(d.cfg, "--default-character-set=utf8mb4", database)
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		cmd.Stdin = reader
		if output, err := cmd.CombinedOutput(); err != nil {
			msg := strings.TrimSpace(string(output))
			if msg != "" {
				return fmt.Errorf("mysql import failed: %s", msg)
			}
			return fmt.Errorf("mysql import failed: %w", err)
		}
		return nil
	}

	// Fallback: execute SQL statements directly over the Go SQL connection when mysql CLI is unavailable
	if d.db == nil {
		return fmt.Errorf("database not connected and mysql CLI not found in PATH")
	}
	db, err := d.dbFor(database)
	if err != nil {
		return fmt.Errorf("failed to select database %s: %w", database, err)
	}
	return execSQLFileFallback(db, filepath)
}
func (d *MySQLDriver) GetTableIndexes(database, table string) ([]IndexInfo, error) {
	if d.db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	// information_schema has the same columns on MySQL 5.7/8 and MariaDB,
	// unlike SHOW INDEX (13, 14 or 15 columns, several of them NULL).
	rows, err := d.db.Query(`
		SELECT INDEX_NAME, NON_UNIQUE, COALESCE(COLUMN_NAME, ''), COALESCE(INDEX_TYPE, '')
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY INDEX_NAME = 'PRIMARY' DESC, INDEX_NAME, SEQ_IN_INDEX`, database, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []IndexInfo
	pos := make(map[string]int)
	for rows.Next() {
		var keyName, column, indexType string
		var nonUnique int
		if err := rows.Scan(&keyName, &nonUnique, &column, &indexType); err != nil {
			return nil, err
		}
		i, ok := pos[keyName]
		if !ok {
			i = len(indexes)
			pos[keyName] = i
			indexes = append(indexes, IndexInfo{
				Name:    keyName,
				Columns: []string{},
				Unique:  nonUnique == 0,
				Primary: keyName == "PRIMARY",
				Type:    indexType,
			})
		}
		indexes[i].Columns = append(indexes[i].Columns, column)
	}
	return indexes, rows.Err()
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
		quotedTables[i] = quoteMy(database) + "." + quoteMy(t)
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
		// The search text is a bound parameter; LIKE wildcards in it match literally.
		var conditions []string
		var args []interface{}
		pattern := "%" + escapeLike(query) + "%"
		for _, col := range searchableCols {
			conditions = append(conditions, quoteMy(col)+" LIKE ?") // backslash is MySQL's default LIKE escape
			args = append(args, pattern)
		}

		searchSQL := fmt.Sprintf("SELECT * FROM %s.%s WHERE %s LIMIT 10", quoteMy(database), quoteMy(table.Name), strings.Join(conditions, " OR "))

		rows, err := m.db.Query(searchSQL, args...)
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

	query := "SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?"
	var settings DatabaseSettings
	err := m.db.QueryRow(query, database).Scan(&settings.Charset, &settings.Collation)
	if err != nil {
		return nil, err
	}

	return &settings, nil
}

func (m *MySQLDriver) ExplainQuery(database, query string) (*QueryExplanation, error) {
	db, err := m.dbFor(database)
	if err != nil {
		return nil, err
	}

	explainQuery := "EXPLAIN " + query
	rows, err := db.Query(explainQuery)
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
