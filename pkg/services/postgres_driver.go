package services

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type PostgresDriver struct {
	db  *sql.DB
	dsn string
}

func NewPostgresDriver() *PostgresDriver {
	return &PostgresDriver{}
}

func (d *PostgresDriver) Connect(config ConnectionConfig) error {
	var dsn string

	if config.User != "" {
		host := config.Host
		if host == "" {
			host = "127.0.0.1"
		}
		port := config.Port
		if port == "" {
			port = "5432"
		}
		// Default postgres DSN
		dsn = fmt.Sprintf("postgres://%s:%s@%s:%s/postgres?sslmode=disable", config.User, config.Password, host, port)
	} else {
		// Auto-discovery from Environment
		envUser := os.Getenv("SLD_DB_USER")
		envPass := os.Getenv("SLD_DB_PASS")
		envHost := os.Getenv("SLD_DB_HOST")
		envPort := os.Getenv("SLD_DB_PORT")

		if envUser == "" {
			envUser = "postgres" // default superuser often
		}
		if envHost == "" {
			envHost = "127.0.0.1"
		}
		if envPort == "" {
			envPort = "5432"
		}

		dsn = fmt.Sprintf("postgres://%s:%s@%s:%s/postgres?sslmode=disable", envUser, envPass, envHost, envPort)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	d.db = db
	d.dsn = dsn
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
		// Exclude 'postgres' if desired, but it's often user-accessible
		databases = append(databases, name)
	}
	return databases, nil
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

	// Create a temporary connection string
	baseDSN := d.dsn
	// Replace /postgres? with /database?
	// This is hacky. Better to rebuild DSN from config if we had it stored.
	// Assuming DSN structure: postgres://user:pass@host:port/dbname?args

	targetDSN := strings.Replace(baseDSN, "/postgres?", "/"+database+"?", 1)
	if !strings.Contains(targetDSN, "/"+database+"?") {
		// maybe no query params
		if strings.HasSuffix(baseDSN, "/postgres") {
			targetDSN = strings.TrimSuffix(baseDSN, "/postgres") + "/" + database
		}
	}

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
	targetDSN := strings.Replace(d.dsn, "/postgres?", "/"+database+"?", 1)
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
	targetDSN := strings.Replace(d.dsn, "/postgres?", "/"+database+"?", 1)
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

func (d *PostgresDriver) ExecuteQuery(database, query string) (*QueryResult, error) {
	targetDSN := strings.Replace(d.dsn, "/postgres?", "/"+database+"?", 1)
	tempDB, err := sql.Open("postgres", targetDSN)
	if err != nil {
		return nil, err
	}
	defer tempDB.Close()

	start := time.Now()
	// Detect SELECT
	trimmed := strings.ToUpper(strings.TrimSpace(query))
	if strings.HasPrefix(trimmed, "SELECT") {
		rows, err := tempDB.Query(query)
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
		res, err := tempDB.Exec(query)
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

func (d *PostgresDriver) GetForeignValues(database, table, column string) ([]string, error) {
	return []string{}, nil
}

func (d *PostgresDriver) GetTableRelationships(database string) ([]TableRelationship, error) {
	targetDSN := strings.Replace(d.dsn, "/postgres?", "/"+database+"?", 1)
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
	// pg_dump
	args := []string{"-h", "localhost", "-U", "postgres", database}
	if table != "" {
		args = append(args, "-t", table)
	}
	cmd := exec.Command("pg_dump", args...)
	// Set PGPASSWORD if needed env var
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}
	return os.WriteFile(filepath, output, 0644)
}

func (d *PostgresDriver) RestoreSnapshot(database, filepath string) error {
	cmd := exec.Command("psql", "-h", "localhost", "-U", "postgres", database)
	file, err := os.Open(filepath)
	if err != nil {
		return err
	}
	defer file.Close()
	cmd.Stdin = file
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("restore failed: %s", string(out))
	}
	return nil
}
func (d *PostgresDriver) GetTableIndexes(database, table string) ([]IndexInfo, error) {
	targetDSN := strings.Replace(d.dsn, "/postgres?", "/"+database+"?", 1)
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
	targetDSN := strings.Replace(d.dsn, "/postgres?", "/"+database+"?", 1)
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
	targetDSN := strings.Replace(d.dsn, "/postgres?", "/"+database+"?", 1)
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
