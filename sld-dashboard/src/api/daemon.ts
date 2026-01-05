export interface TableDataOptions {
  perPage?: number;
  sortCol?: string;
  sortOrder?: "ASC" | "DESC";
  profile?: boolean;
}

export interface QueryResult {
  rows?: Record<string, unknown>[];
  rowCount?: number;
  total?: number;
  total_pages?: number;
  query_time?: number;
  error?: string;
  columns?: string[]; // Added for explain data
}

export interface ColumnInfo {
  name: string;
  type: string;
  key: string;
  null: string;
  default: string | null;
  extra: string;
  foreign_key?: { table: string; column: string };
}

export interface Snapshot {
  id: string;
  database: string;
  table?: string;
  filename: string;
  created_at: string;
  size: number;
}

export interface DatabaseTrigger {
  Trigger: string;
  Event: string;
  Timing: string;
  Table: string;
  Statement: string;
}

export const api = {
  getDatabases: async (): Promise<string[]> => {
    return [];
  },
  getTables: async (_db: string): Promise<string[]> => {
    void _db;
    return [];
  },
  getTableData: async (
    _db: string,
    _table: string,
    _page: number,
    _options: TableDataOptions
  ): Promise<QueryResult> => {
    void _db;
    void _table;
    void _page;
    void _options;
    return { rows: [], rowCount: 0 };
  },
  getTableSchema: async (
    _db: string,
    _table: string
  ): Promise<ColumnInfo[]> => {
    void _db;
    void _table;
    return [];
  },
  getSnapshots: async (): Promise<Snapshot[]> => {
    return [];
  },
  createSnapshot: async (_database: string, _table?: string): Promise<void> => {
    void _database;
    void _table;
    // mock
  },
  restoreSnapshot: async (_filename: string): Promise<void> => {
    void _filename;
    // mock
  },
  deleteSnapshot: async (_filename: string): Promise<void> => {
    void _filename;
    // mock
  },
  executeQuery: async (
    _database: string,
    _query: string
  ): Promise<QueryResult> => {
    void _database;
    void _query;
    return { rows: [], rowCount: 0 };
  },
  importDatabase: async (
    _file: File,
    _database: string,
    _restore?: boolean
  ): Promise<void> => {
    void _file;
    void _database;
    void _restore;
    // mock
  },
  getForeignValues: async (
    _database: string,
    _table: string,
    _column: string
  ): Promise<{ value: string; label: string }[]> => {
    void _database;
    void _table;
    void _column;
    return [];
  },
};
