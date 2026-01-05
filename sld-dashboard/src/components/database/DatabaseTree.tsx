import { type DatabaseInfo } from "../../api/daemon";

interface DatabaseTreeProps {
  databases: DatabaseInfo[];
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
          <li key={db.name} className="cursor-pointer">
            <div
              className={`flex items-center justify-between p-1 rounded ${
                selectedDB === db.name ? "bg-accent text-accent-foreground" : ""
              }`}
              onClick={() => onSelectDb(db.name)}
            >
              <span>{db.name}</span>
              {selectedDB === db.name && (
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    onCloneDatabase(db.name);
                  }}
                  className="text-xs text-muted-foreground hover:text-foreground"
                >
                  Clone
                </button>
              )}
            </div>
            {selectedDB === db.name && selectedTable && (
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
