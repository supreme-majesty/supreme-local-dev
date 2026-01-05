interface DatabaseTreeProps {
  databases: string[];
  selectedDB: string | null;
  selectedTable: string | null;
  onSelectDb: (db: string) => void;
  onSelectTable: (db: string, table: string) => void;
  onCreateTable: (db: string) => void;
  onCloneDatabase: (db: string) => void;
}

export function DatabaseTree({
  databases,
  selectedDB,
  selectedTable,
  onSelectDb,
  onCreateTable,
  onCloneDatabase,
}: DatabaseTreeProps) {
  return (
    <div className="p-2">
      <h3>Database Tree</h3>
      <div className="mb-2">
        <button
          className="text-xs bg-blue-500 text-white px-2 py-1 rounded"
          onClick={() => onCreateTable(selectedDB || "")}
          disabled={!selectedDB}
        >
          Create Table
        </button>
      </div>
      <ul className="space-y-1">
        {databases.map((db) => (
          <li key={db} className="cursor-pointer">
            <div
              className={`flex items-center justify-between p-1 rounded ${
                selectedDB === db ? "bg-accent text-accent-foreground" : ""
              }`}
              onClick={() => onSelectDb(db)}
            >
              <span>{db}</span>
              {selectedDB === db && (
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    onCloneDatabase(db);
                  }}
                  className="text-xs text-muted-foreground hover:text-foreground"
                >
                  Clone
                </button>
              )}
            </div>
            {selectedDB === db && selectedTable && (
              <div className="pl-4 text-sm text-muted-foreground">
                └ {selectedTable}
              </div>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}
