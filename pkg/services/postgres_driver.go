package services

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type PostgresDriver struct {
	db        *sql.DB
	dsn       string
	cfg       ConnectionConfig
	activeTxs map[string]*sql.Tx
}

func NewPostgresDriver() *PostgresDriver {
	return &PostgresDriver{
		activeTxs: make(map[string]*sql.Tx),
	}
}

// PostgresHost returns the PostgreSQL database host (compose sets SLD_PG_HOST or POSTGRES_HOST).
func PostgresHost() string {
	if v := os.Getenv("SLD_PG_HOST"); v != "" {
		return v
	}
	return envOr("POSTGRES_HOST", "127.0.0.1")
}

// PostgresPort returns the PostgreSQL database TCP port.
func PostgresPort() string {
	if v := os.Getenv("SLD_PG_PORT"); v != "" {
		return v
	}
	return envOr("POSTGRES_PORT", "5432")
}

// PostgresUser returns the PostgreSQL database user.
func PostgresUser() string {
	if v := os.Getenv("SLD_PG_USER"); v != "" {
		return v
	}
	return envOr("POSTGRES_USER", "postgres")
}

// PostgresPassword returns the PostgreSQL database password.
func PostgresPassword() string {
	if v := os.Getenv("SLD_PG_PASS"); v != "" {
		return v
	}
	return os.Getenv("POSTGRES_PASSWORD")
}

// PostgresDB returns default database name.
func PostgresDB() string {
	if v := os.Getenv("SLD_PG_DB"); v != "" {
		return v
	}
	return envOr("POSTGRES_DB", "postgres")
}

// PostgresDSN builds connection URL for PostgreSQL.
func PostgresDSN() string {
	user := PostgresUser()
	pass := PostgresPassword()
	host := PostgresHost()
	port := PostgresPort()
	db := PostgresDB()
	if pass == "" {
		return fmt.Sprintf("postgres://%s@%s:%s/%s?sslmode=disable&connect_timeout=5", user, host, port, db)
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable&connect_timeout=5", user, pass, host, port, db)
}

func (d *PostgresDriver) Connect(config ConnectionConfig) error {
	var dsn string

	if config.User != "" || config.Host != "" {
		user := config.User
		if user == "" {
			user = "postgres"
		}
		host := config.Host
		if host == "" {
			host = "127.0.0.1"
		}
		port := config.Port
		if port == "" {
			port = "5432"
		}
		dbName := "postgres"
		if config.Password != "" {
			dsn = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable&connect_timeout=5", user, config.Password, host, port, dbName)
		} else {
			dsn = fmt.Sprintf("postgres://%s@%s:%s/%s?sslmode=disable&connect_timeout=5", user, host, port, dbName)
		}
	} else {
		dsn = PostgresDSN()
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	if d.db != nil {
		_ = d.db.Close()
	}
	d.db = db
	d.dsn = dsn
	d.cfg = config
	return nil
}

func (d *PostgresDriver) Close() error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

func (d *PostgresDriver) IsConnected() bool {
	return d.db != nil && d.db.Ping() == nil
}

func (d *PostgresDriver) ListDatabases() ([]string, error) {
	rows, err := d.db.Query("SELECT datname FROM pg_database WHERE datistemplate = false;")
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

func (d *PostgresDriver) TableCounts() (map[string]int, error) {
	if d.db == nil {
		return nil, nil
	}
	rows, err := d.db.Query(`
		SELECT table_catalog, COUNT(*)
		FROM information_schema.tables
		WHERE table_type = 'BASE TABLE'
		  AND table_schema NOT IN ('pg_catalog', 'information_schema')
		GROUP BY table_catalog
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

func (d *PostgresDriver) CreateDatabase(name string) error {
	_, err := d.db.Exec(fmt.Sprintf("CREATE DATABASE \"%s\"", name))
	return err
}

func (d *PostgresDriver) DeleteDatabase(name string) error {
	_, err := d.db.Exec(fmt.Sprintf("DROP DATABASE \"%s\"", name))
	return err
}

func (d *PostgresDriver) RenameDatabase(oldName, newName string) error {
	// Terminate other connections to the database before renaming
	d.db.Exec(fmt.Sprintf(`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid()`, oldName))
	_, err := d.db.Exec(fmt.Sprintf("ALTER DATABASE \"%s\" RENAME TO \"%s\"", oldName, newName))
	return err
}

func (d *PostgresDriver) ListTables(database string) ([]TableInfo, error) {
	// Reconnect to specific database?
	// Postgres connection is to a specific DB. 'postgres' is default maintenance DB.
	// To list tables in 'target', we usually need to Connect to 'target'.
	// This implies we should swap connection or open a temp one.
	// For now, let's assume d.db is connected to maintenance DB, so we can't switch context easily via USE like MySQL.
	// We MUST open a new connection to 'database'.

	// Temporarily connect to the target database
	// Parse current DSN to replace dbname
	// This is tricky. Simplified approach: reuse credentials.

	// For robust implementation, we'll just open a new connection for this operation
	// But this is inefficient.
	// However, ListTables is infrequent.

	// ... actually, we can query information_schema.tables of the connected DB.
	// But d.db is connected to 'postgres' initially.
	// So we DO need to switch.

	targetDSN := replacePostgresDB(d.dsn, database)

	tempDB, err := sql.Open("postgres", targetDSN)
	if err != nil {
		return nil, err
	}
	defer tempDB.Close()

	// Query
	query := `
		SELECT 
			table_name,
			(SELECT n_live_tup FROM pg_stat_user_tables WHERE relname = table_name) as row_count,
			'heap' as engine,
			'default' as collation,
			pg_total_relation_size(quote_ident(table_name)) as size,
			0 as overhead
		FROM information_schema.tables 
		WHERE table_schema = 'public' 
		ORDER BY table_name
	`

	rows, err := tempDB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tables := make([]TableInfo, 0)
	for rows.Next() {
		var t TableInfo
		var rowCount sql.NullInt64
		if err := rows.Scan(&t.Name, &rowCount, &t.Engine, &t.Collation, &t.Size, &t.Overhead); err != nil {
			continue
		}
		t.RowCount = rowCount.Int64
		tables = append(tables, t)
	}
	return tables, nil
}

func (d *PostgresDriver) GetTableColumns(database, table string) ([]ColumnInfo, error) {
	// Connect to target DB
	targetDSN := replacePostgresDB(d.dsn, database)
	tempDB, err := sql.Open("postgres", targetDSN)
	if err != nil {
		return nil, err
	}
	defer tempDB.Close()

	// FKs
	fks := make(map[string]ForeignKeyInfo)
	fkQuery := `
		SELECT
			kcu.column_name,
			ccu.table_name AS foreign_table_name,
			ccu.column_name AS foreign_column_name
		FROM 
			information_schema.key_column_usage AS kcu
		JOIN 
			information_schema.constraint_column_usage AS ccu
			ON ccu.constraint_name = kcu.constraint_name
		JOIN 
			information_schema.table_constraints AS tc
			ON tc.constraint_name = kcu.constraint_name
		WHERE 
			tc.constraint_type = 'FOREIGN KEY' 
			AND tc.table_name = $1
	`
	fkRows, err := tempDB.Query(fkQuery, table)
	if err == nil {
		defer fkRows.Close()
		for fkRows.Next() {
			var col, refT, refC string
			if err := fkRows.Scan(&col, &refT, &refC); err == nil {
				fks[col] = ForeignKeyInfo{Table: refT, Column: refC}
			}
		}
	}

	// Columns
	query := `
		SELECT 
			column_name, 
			data_type, 
			is_nullable, 
			column_default 
		FROM information_schema.columns 
		WHERE table_schema = 'public' AND table_name = $1
	`
	rows, err := tempDB.Query(query, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var name, dtype, isoNull string
		var defVal sql.NullString
		if err := rows.Scan(&name, &dtype, &isoNull, &defVal); err != nil {
			continue
		}

		col := ColumnInfo{
			Name:     name,
			Type:     dtype,
			Nullable: isoNull == "YES",
			Default:  defVal.String,
		}
		if fk, ok := fks[name]; ok {
			col.ForeignKey = &fk
		}
		columns = append(columns, col)
	}
	return columns, nil
}

func (d *PostgresDriver) GetTableData(database, table string, page, perPage int) (*TableData, error) {
	return d.GetTableDataEx(database, table, page, perPage, "", "", false)
}

func (d *PostgresDriver) GetTableDataEx(database, table string, page, perPage int, sortCol, sortOrder string, profile bool) (*TableData, error) {
	targetDSN := replacePostgresDB(d.dsn, database)
	tempDB, err := sql.Open("postgres", targetDSN)
	if err != nil {
		return nil, err
	}
	defer tempDB.Close()

	// Count
	var total int64
	tempDB.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM \"%s\"", table)).Scan(&total)

	if perPage <= 0 {
		perPage = 50
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * perPage
	totalPages := int((total + int64(perPage) - 1) / int64(perPage))

	if sortOrder != "DESC" {
		sortOrder = "ASC"
	}

	query := fmt.Sprintf("SELECT * FROM \"%s\"", table)
	if sortCol != "" {
		query += fmt.Sprintf(" ORDER BY \"%s\" %s", sortCol, sortOrder)
	}
	query += fmt.Sprintf(" LIMIT %d OFFSET %d", perPage, offset)

	if profile {
		tempDB.Exec("EXPLAIN ANALYZE " + query) // Just trigger usage, parsing output is complex
	}

	rows, err := tempDB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	colNames, _ := rows.Columns()

	// Fetch column info for metadata
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
	}, nil
}

func (d *PostgresDriver) ExecuteQuery(database, query string, txId string) (*QueryResult, error) {
	var targetDB *sql.DB
	var tx *sql.Tx
	var err error

	if txId != "" {
		var ok bool
		tx, ok = d.activeTxs[txId]
		if !ok {
			return nil, fmt.Errorf("transaction %s not found", txId)
		}
	} else {
		targetDSN := replacePostgresDB(d.dsn, database)
		targetDB, err = sql.Open("postgres", targetDSN)
		if err != nil {
			return nil, err
		}
		defer targetDB.Close()
	}

	start := time.Now()
	trimmed := strings.ToUpper(strings.TrimSpace(query))
	isSelect := strings.HasPrefix(trimmed, "SELECT") || strings.HasPrefix(trimmed, "SHOW") || strings.HasPrefix(trimmed, "DESC") || strings.HasPrefix(trimmed, "EXPLAIN")

	if isSelect {
		var rows *sql.Rows
		if tx != nil {
			rows, err = tx.Query(query)
		} else {
			rows, err = targetDB.Query(query)
		}

		if err != nil {
			return nil, err
		}
		defer rows.Close()

		cols, _ := rows.Columns()
		var data []map[string]interface{}
		// Scan ...
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
	} else {
		var res sql.Result
		if tx != nil {
			res, err = tx.Exec(query)
		} else {
			res, err = targetDB.Exec(query)
		}
		if err != nil {
			return nil, err
		}
		aff, _ := res.RowsAffected()
		return &QueryResult{
			AffectedRows:    aff,
			ExecutionTimeMs: time.Since(start).Milliseconds(),
		}, nil
	}
}

func (d *PostgresDriver) BeginTransaction(database string) (string, error) {
	if d.db == nil {
		return "", fmt.Errorf("database not connected")
	}

	tx, err := d.db.Begin()
	if err != nil {
		return "", err
	}

	txId := fmt.Sprintf("tx_%d", time.Now().UnixNano())
	d.activeTxs[txId] = tx
	return txId, nil
}

func (d *PostgresDriver) CommitTransaction(txId string) error {
	tx, ok := d.activeTxs[txId]
	if !ok {
		return fmt.Errorf("transaction %s not found", txId)
	}

	err := tx.Commit()
	delete(d.activeTxs, txId)
	return err
}

func (d *PostgresDriver) RollbackTransaction(txId string) error {
	tx, ok := d.activeTxs[txId]
	if !ok {
		return fmt.Errorf("transaction %s not found", txId)
	}

	err := tx.Rollback()
	delete(d.activeTxs, txId)
	return err
}

func (d *PostgresDriver) BatchInsert(database, table string, columns []string, data [][]interface{}, txId string) error {
	if d.db == nil {
		return fmt.Errorf("database not connected")
	}

	if len(data) == 0 {
		return nil
	}

	// Prepare column names
	colNames := make([]string, len(columns))
	for i, col := range columns {
		colNames[i] = "\"" + col + "\""
	}
	colsStr := strings.Join(colNames, ", ")

	// Use transaction if provided
	var tx *sql.Tx
	if txId != "" {
		var ok bool
		tx, ok = d.activeTxs[txId]
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
				// PostgreSQL uses $1, $2, etc.
				rowPlaceholders[k] = fmt.Sprintf("$%d", j*len(columns)+k+1)
			}
			placeholders[j] = "(" + strings.Join(rowPlaceholders, ", ") + ")"
			values = append(values, row...)
		}

		query := fmt.Sprintf("INSERT INTO \"%s\" (%s) VALUES %s", table, colsStr, strings.Join(placeholders, ", "))

		var err error
		if tx != nil {
			_, err = tx.Exec(query, values...)
		} else {
			// If not in a persistent transaction, we still need the right database connection.
			// Reusing the logic from ExecuteQuery for simple connection per query.
			targetDSN := replacePostgresDB(d.dsn, database)
			tempDB, err := sql.Open("postgres", targetDSN)
			if err != nil {
				return err
			}
			_, err = tempDB.Exec(query, values...)
			tempDB.Close()
		}

		if err != nil {
			return fmt.Errorf("batch insert failed at index %d: %w", i, err)
		}
	}

	return nil
}

// replacePostgresDB returns a copy of a postgres:// DSN targeting the specified database.
func replacePostgresDB(dsn, database string) string {
	if database == "" {
		return dsn
	}
	u, err := url.Parse(dsn)
	if err == nil && u.Scheme != "" {
		u.Path = "/" + database
		return u.String()
	}
	return strings.Replace(dsn, "/postgres?", "/"+database+"?", 1)
}

func (d *PostgresDriver) Query(database, query string, args []interface{}) ([]map[string]interface{}, error) {
	if d.db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	conn := d.db
	var tempDB *sql.DB
	if database != "" {
		var err error
		tempDB, err = sql.Open("postgres", replacePostgresDB(d.dsn, database))
		if err != nil {
			return nil, err
		}
		defer tempDB.Close()
		conn = tempDB
	}

	rows, err := conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	results := make([]map[string]interface{}, 0)
	for rows.Next() {
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}
		row := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			if b, ok := values[i].([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = values[i]
			}
		}
		results = append(results, row)
	}
	return results, rows.Err()
}

func (d *PostgresDriver) GetTableInfo(database, table string) (*TableInfo, error) {
	if d.db == nil {
		return nil, fmt.Errorf("database not connected")
	}
	tempDB, err := sql.Open("postgres", replacePostgresDB(d.dsn, database))
	if err != nil {
		return nil, err
	}
	defer tempDB.Close()

	query := `
		SELECT
			table_name,
			COALESCE((SELECT n_live_tup FROM pg_stat_user_tables WHERE relname = table_name LIMIT 1), 0) AS row_count,
			'heap' AS engine,
			'default' AS collation,
			pg_total_relation_size(quote_ident(table_name)) AS size,
			0 AS overhead
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = $1
		LIMIT 1
	`
	var info TableInfo
	err = tempDB.QueryRow(query, table).Scan(&info.Name, &info.RowCount, &info.Engine, &info.Collation, &info.Size, &info.Overhead)
	if err != nil {
		return nil, fmt.Errorf("table info not found: %w", err)
	}
	return &info, nil
}

func (d *PostgresDriver) GetStats(database string) (map[string]interface{}, error) {
	if d.db == nil {
		return nil, fmt.Errorf("database not connected")
	}

	stats := make(map[string]interface{})

	// 1. Get Database Stats
	query := fmt.Sprintf("SELECT * FROM pg_stat_database WHERE datname = '%s'", database)
	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if rows.Next() {
		cols, _ := rows.Columns()
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		rows.Scan(valuePtrs...)
		dbStats := make(map[string]interface{})
		for i, col := range cols {
			dbStats[col] = values[i]
		}
		stats["db_stats"] = dbStats
	}

	// 2. Get Activity
	aRows, err := d.db.Query("SELECT * FROM pg_stat_activity")
	if err != nil {
		return nil, err
	}
	defer aRows.Close()

	var processes []map[string]interface{}
	cols, _ := aRows.Columns()
	for aRows.Next() {
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		aRows.Scan(valuePtrs...)
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

func (d *PostgresDriver) GetForeignValues(database, table, column string) ([]string, error) {
	if d.db == nil || table == "" || column == "" {
		return []string{}, nil
	}
	tempDB, err := sql.Open("postgres", replacePostgresDB(d.dsn, database))
	if err != nil {
		return nil, err
	}
	defer tempDB.Close()

	quotedTable := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
	quotedCol := `"` + strings.ReplaceAll(column, `"`, `""`) + `"`
	query := fmt.Sprintf("SELECT DISTINCT %s::text FROM %s WHERE %s IS NOT NULL ORDER BY 1 LIMIT 100", quotedCol, quotedTable, quotedCol)

	rows, err := tempDB.Query(query)
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

func (d *PostgresDriver) GetTableRelationships(database string) ([]TableRelationship, error) {
	targetDSN := replacePostgresDB(d.dsn, database)
	tempDB, err := sql.Open("postgres", targetDSN)
	if err != nil {
		return nil, err
	}
	defer tempDB.Close()

	query := `
		SELECT
			kcu.table_name AS from_table,
			kcu.column_name AS from_column,
			ccu.table_name AS to_table,
			ccu.column_name AS to_column
		FROM 
			information_schema.key_column_usage AS kcu
		JOIN 
			information_schema.constraint_column_usage AS ccu
			ON ccu.constraint_name = kcu.constraint_name
		JOIN 
			information_schema.table_constraints AS tc
			ON tc.constraint_name = kcu.constraint_name
		WHERE 
			tc.constraint_type = 'FOREIGN KEY'
	`

	rows, err := tempDB.Query(query)
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

func (d *PostgresDriver) CreateSnapshot(database, table string, filepath string) error {
	extra := []string{"-d", database}
	if table != "" {
		extra = append(extra, "-t", table)
	}
	args, env := buildPostgresCLIArgs(d.cfg, extra...)

	outFile, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create snapshot file: %w", err)
	}

	var stderr strings.Builder
	cmd := exec.Command("pg_dump", args...)
	cmd.Env = env
	cmd.Stdout = outFile
	cmd.Stderr = &stderr

	err = cmd.Run()
	_ = outFile.Close()
	if err != nil {
		_ = os.Remove(filepath)
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("pg_dump failed: %s", msg)
		}
		return fmt.Errorf("pg_dump failed: %w", err)
	}
	return nil
}

func (d *PostgresDriver) RestoreSnapshot(database, filepath string) error {
	if _, err := exec.LookPath("psql"); err == nil {
		reader, err := openMaybeGzip(filepath)
		if err != nil {
			return fmt.Errorf("failed to open SQL file: %w", err)
		}
		defer reader.Close()

		args, env := buildPostgresCLIArgs(d.cfg, "-v", "ON_ERROR_STOP=1", "-d", database)
		cmd := exec.Command("psql", args...)
		cmd.Env = env
		cmd.Stdin = reader
		if out, err := cmd.CombinedOutput(); err != nil {
			msg := strings.TrimSpace(string(out))
			if msg != "" {
				return fmt.Errorf("psql import failed: %s", msg)
			}
			return fmt.Errorf("psql import failed: %w", err)
		}
		return nil
	}

	// Fallback: execute SQL statements directly over Go SQL connection when psql CLI is unavailable
	if d.db == nil {
		return fmt.Errorf("database not connected and psql CLI not found in PATH")
	}
	tempDB, err := sql.Open("postgres", replacePostgresDB(d.dsn, database))
	if err != nil {
		return err
	}
	defer tempDB.Close()
	return execSQLFileFallback(tempDB, filepath)
}
func (d *PostgresDriver) GetTableIndexes(database, table string) ([]IndexInfo, error) {
	targetDSN := replacePostgresDB(d.dsn, database)
	tempDB, err := sql.Open("postgres", targetDSN)
	if err != nil {
		return nil, err
	}
	defer tempDB.Close()

	query := `
		SELECT
			indexname as index_name,
			indexdef as index_def
		FROM
			pg_indexes
		WHERE
			schemaname = 'public'
			AND tablename = $1
	`
	rows, err := tempDB.Query(query, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []IndexInfo
	for rows.Next() {
		var name, def string
		if err := rows.Scan(&name, &def); err != nil {
			continue
		}

		unique := strings.Contains(strings.ToUpper(def), "UNIQUE")
		primary := strings.HasSuffix(name, "_pkey")

		// Extract columns from def: CREATE INDEX ... ON ... USING ... (col1, col2)
		start := strings.Index(def, "(")
		end := strings.LastIndex(def, ")")
		var cols []string
		if start != -1 && end != -1 && end > start {
			colStr := def[start+1 : end]
			parts := strings.Split(colStr, ",")
			for _, p := range parts {
				cols = append(cols, strings.TrimSpace(p))
			}
		}

		indexes = append(indexes, IndexInfo{
			Name:    name,
			Columns: cols,
			Unique:  unique,
			Primary: primary,
			Type:    "btree", // default for pg_indexes
		})
	}
	return indexes, nil
}

func (d *PostgresDriver) Maintenance(database string, tables []string, operation string) ([]MaintenanceResult, error) {
	targetDSN := replacePostgresDB(d.dsn, database)
	tempDB, err := sql.Open("postgres", targetDSN)
	if err != nil {
		return nil, err
	}
	defer tempDB.Close()

	var results []MaintenanceResult

	// Default to all tables if none specified
	if len(tables) == 0 {
		rows, err := tempDB.Query("SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname = 'public'")
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var t string
				if err := rows.Scan(&t); err == nil {
					tables = append(tables, t)
				}
			}
		}
	}

	for _, table := range tables {
		var sqlOp string
		switch strings.ToUpper(operation) {
		case "ANALYZE":
			sqlOp = fmt.Sprintf("ANALYZE %s", table)
		case "CHECK":
			// PG doesn't have CHECK TABLE, but we can use ANALYZE for stats
			sqlOp = fmt.Sprintf("ANALYZE %s", table)
		case "OPTIMIZE":
			sqlOp = fmt.Sprintf("VACUUM FULL %s", table)
		case "REPAIR":
			sqlOp = fmt.Sprintf("REINDEX TABLE %s", table)
		default:
			continue
		}

		_, err := tempDB.Exec(sqlOp)
		status := "OK"
		msg := "Success"
		if err != nil {
			status = "Error"
			msg = err.Error()
		}

		results = append(results, MaintenanceResult{
			Table:     table,
			Operation: operation,
			MsgType:   status,
			MsgText:   msg,
		})
	}

	return results, nil
}

func (d *PostgresDriver) GlobalSearch(database string, query string) ([]SearchResult, error) {
	targetDSN := replacePostgresDB(d.dsn, database)
	tempDB, err := sql.Open("postgres", targetDSN)
	if err != nil {
		return nil, err
	}
	defer tempDB.Close()

	// 1. Find all searchable columns
	colQuery := `
        SELECT table_name, column_name
        FROM information_schema.columns
        WHERE table_schema = 'public'
        AND data_type IN ('character varying', 'text', 'character', 'uuid')
    `
	rows, err := tempDB.Query(colQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tableCols := make(map[string][]string)
	for rows.Next() {
		var t, c string
		if err := rows.Scan(&t, &c); err == nil {
			tableCols[t] = append(tableCols[t], c)
		}
	}

	var results []SearchResult
	for table, cols := range tableCols {
		var whereParts []string
		for _, col := range cols {
			whereParts = append(whereParts, fmt.Sprintf("%s::text ILIKE '%%%s%%'", col, query))
		}

		searchQuery := fmt.Sprintf("SELECT * FROM %s WHERE %s LIMIT 10", table, strings.Join(whereParts, " OR "))
		rows, err := tempDB.Query(searchQuery)
		if err != nil {
			continue
		}

		columnNames, _ := rows.Columns()
		var matches []map[string]interface{}
		for rows.Next() {
			values := make([]interface{}, len(columnNames))
			valuePtrs := make([]interface{}, len(columnNames))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			if err := rows.Scan(valuePtrs...); err == nil {
				row := make(map[string]interface{})
				for i, col := range columnNames {
					val := values[i]
					if b, ok := val.([]byte); ok {
						row[col] = string(b)
					} else {
						row[col] = val
					}
				}
				matches = append(matches, row)
			}
		}
		rows.Close()

		if len(matches) > 0 {
			results = append(results, SearchResult{
				Table:       table,
				ColumnCount: len(cols),
				RowCount:    len(matches),
				Matches:     matches,
			})
		}
	}

	return results, nil
}

func (d *PostgresDriver) GetCollations() ([]CollationInfo, error) {
	// Postgres collations are a bit complex, but we can list them from pg_collation
	rows, err := d.db.Query("SELECT collname FROM pg_collation WHERE collnamespace = (SELECT oid FROM pg_namespace WHERE nspname = 'pg_catalog')")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var collations []CollationInfo
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			collations = append(collations, CollationInfo{Name: name, Charset: "UTF8"})
		}
	}
	return collations, nil
}

func (d *PostgresDriver) GetDatabaseSettings(database string) (*DatabaseSettings, error) {
	// In PG, collation is set at DB creation (datcollate, datctype)
	var coll, ctype string
	err := d.db.QueryRow("SELECT datcollate, datctype FROM pg_database WHERE datname = $1", database).Scan(&coll, &ctype)
	if err != nil {
		return nil, err
	}
	return &DatabaseSettings{Collation: coll, Charset: "UTF8"}, nil
}

func (d *PostgresDriver) ExplainQuery(database, query string) (*QueryExplanation, error) {
	// Connect to target DB
	targetDSN := replacePostgresDB(d.dsn, database)
	tempDB, err := sql.Open("postgres", targetDSN)
	if err != nil {
		return nil, err
	}
	defer tempDB.Close()

	// 1. Run EXPLAIN (VERBOSE, FORMAT JSON)
	explainQuery := "EXPLAIN (FORMAT JSON) " + query
	var jsonPlan string
	err = tempDB.QueryRow(explainQuery).Scan(&jsonPlan)
	if err != nil {
		return nil, err
	}

	// 2. Simple output for now
	plan := []map[string]interface{}{
		{"raw_plan": jsonPlan},
	}

	return &QueryExplanation{
		Query:         query,
		ExecutionPlan: plan,
		Analysis:      []string{"PostgreSQL plan captured in JSON format."},
		Complexity:    "Moderate",
	}, nil
}
