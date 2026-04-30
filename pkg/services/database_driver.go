package services

import "time"

// DatabaseDriver defines the interface for database interactions
type DatabaseDriver interface {
	Connect(config ConnectionConfig) error
	Close() error
	IsConnected() bool

	ListDatabases() ([]string, error)
	CreateDatabase(name string) error
	DeleteDatabase(name string) error
	RenameDatabase(oldName, newName string) error
	ListTables(database string) ([]TableInfo, error)
	GetTableColumns(database, table string) ([]ColumnInfo, error)
	GetTableData(database, table string, page, perPage int) (*TableData, error)
	GetTableDataEx(database, table string, page, perPage int, sortCol, sortOrder string, profile bool) (*TableData, error)

	ExecuteQuery(database, query string, txId string) (*QueryResult, error)
	BeginTransaction(database string) (string, error)
	CommitTransaction(txId string) error
	RollbackTransaction(txId string) error
	BatchInsert(database, table string, columns []string, data [][]interface{}, txId string) error
	GetStats(database string) (map[string]interface{}, error)
	GetForeignValues(database, table, column string) ([]string, error)
	GetTableRelationships(database string) ([]TableRelationship, error)
	GetTableInfo(database, table string) (*TableInfo, error)
	Query(database, query string, args []interface{}) ([]map[string]interface{}, error)

	// Backup/Restore
	CreateSnapshot(database, table string, filepath string) error
	RestoreSnapshot(database, filepath string) error

	// Advanced Schema
	GetTableIndexes(database, table string) ([]IndexInfo, error)

	// Maintenance & Search
	Maintenance(database string, tables []string, operation string) ([]MaintenanceResult, error)
	GlobalSearch(database string, query string) ([]SearchResult, error)
	GetCollations() ([]CollationInfo, error)
	GetDatabaseSettings(database string) (*DatabaseSettings, error)
	ExplainQuery(database, query string) (*QueryExplanation, error)
}

type MaintenanceResult struct {
	Table     string `json:"table"`
	Operation string `json:"operation"`
	MsgType   string `json:"msg_type"`
	MsgText   string `json:"msg_text"`
}

type SearchResult struct {
	Table       string                   `json:"table"`
	ColumnCount int                      `json:"column_count"`
	RowCount    int                      `json:"row_count"`
	Matches     []map[string]interface{} `json:"matches,omitempty"`
}

type CollationInfo struct {
	Name    string `json:"name"`
	Charset string `json:"charset"`
}

type DatabaseSettings struct {
	Collation string `json:"collation"`
	Charset   string `json:"charset"`
}

type ConnectionConfig struct {
	User     string
	Password string
	Host     string
	Port     string
	Socket   string
}

// Metadata Structs (moved from database.go)
type Snapshot struct {
	ID        string    `json:"id"`
	Database  string    `json:"database"`
	Table     string    `json:"table,omitempty"`
	Filename  string    `json:"filename"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

type TableInfo struct {
	Name      string `json:"name"`
	RowCount  int64  `json:"row_count"`
	Engine    string `json:"engine"`
	Collation string `json:"collation"`
	Size      int64  `json:"size"`
	Overhead  int64  `json:"overhead"`
}

type ForeignKeyInfo struct {
	Table  string `json:"table"`
	Column string `json:"column"`
}

type TableRelationship struct {
	FromTable  string `json:"from_table"`
	FromColumn string `json:"from_column"`
	ToTable    string `json:"to_table"`
	ToColumn   string `json:"to_column"`
}

type ColumnInfo struct {
	Name       string          `json:"name"`
	Type       string          `json:"type"`
	Nullable   bool            `json:"nullable"`
	Key        string          `json:"key"`
	Default    string          `json:"default"`
	ForeignKey *ForeignKeyInfo `json:"foreign_key,omitempty"`
}

type TableData struct {
	Columns    []ColumnInfo             `json:"columns"`
	Rows       []map[string]interface{} `json:"rows"`
	Total      int64                    `json:"total"`
	Page       int                      `json:"page"`
	PerPage    int                      `json:"per_page"`
	TotalPages int                      `json:"total_pages"`
	QueryTime  float64                  `json:"query_time,omitempty"`
}

type QueryResult struct {
	Columns         []string                 `json:"columns"`
	Rows            []map[string]interface{} `json:"rows"`
	RowCount        int                      `json:"row_count"`
	AffectedRows    int64                    `json:"affected_rows,omitempty"`
	ExecutionTimeMs int64                    `json:"execution_time_ms"`
}

type IndexInfo struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
	Primary bool     `json:"primary"`
	Type    string   `json:"type"`
}

type QueryExplanation struct {
	Query           string                   `json:"query"`
	ExecutionPlan   []map[string]interface{} `json:"execution_plan"`
	Analysis        []string                 `json:"analysis"`
	Recommendations []string                 `json:"recommendations"`
	EstimatedRows   int64                    `json:"estimated_rows"`
	Complexity      string                   `json:"complexity"` // Simple, Moderate, Complex
}
