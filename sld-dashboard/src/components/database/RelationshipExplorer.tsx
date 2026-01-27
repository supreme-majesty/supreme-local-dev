import { useState, useEffect } from "react";
import {
  Network,
  Table,
  ArrowRight,
  RefreshCw,
  Loader2,
  Database,
} from "lucide-react";
import { api } from "@/api/daemon";

interface TableRelationship {
  from_table: string;
  from_column: string;
  to_table: string;
  to_column: string;
}

interface RelationshipExplorerProps {
  selectedDb: string | null;
}

export function RelationshipExplorer({
  selectedDb,
}: RelationshipExplorerProps) {
  const [relationships, setRelationships] = useState<TableRelationship[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedTable, setSelectedTable] = useState<string | null>(null);

  const fetchRelationships = async () => {
    if (!selectedDb) return;
    setLoading(true);
    setError(null);
    try {
      const res = await api.getDbRelationships(selectedDb);
      setRelationships(res);
    } catch (e: any) {
      setError(e.message || "Failed to fetch relationships");
      setRelationships([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (selectedDb) {
      fetchRelationships();
    }
  }, [selectedDb]);

  // Get unique tables involved in relationships
  const tables = [
    ...new Set([
      ...relationships.map((r) => r.from_table),
      ...relationships.map((r) => r.to_table),
    ]),
  ].sort();

  // Filter relationships based on selected table
  const filteredRelationships = selectedTable
    ? relationships.filter(
        (r) => r.from_table === selectedTable || r.to_table === selectedTable,
      )
    : relationships;

  if (!selectedDb) {
    return (
      <div className="flex flex-col items-center justify-center h-64 text-gray-500">
        <Database className="w-12 h-12 mb-4 opacity-50" />
        <p>Select a database to explore relationships</p>
      </div>
    );
  }

  return (
    <div className="p-4 space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold flex items-center gap-2">
          <Network className="w-5 h-5" />
          Table Relationships
        </h2>
        <button
          onClick={fetchRelationships}
          disabled={loading}
          className="flex items-center gap-2 px-3 py-1.5 text-sm bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
        >
          {loading ? (
            <Loader2 className="w-4 h-4 animate-spin" />
          ) : (
            <RefreshCw className="w-4 h-4" />
          )}
          Refresh
        </button>
      </div>

      {error && (
        <div className="p-3 text-sm bg-red-100 border border-red-200 text-red-700 rounded">
          {error}
        </div>
      )}

      {/* Table Filter */}
      {tables.length > 0 && (
        <div className="flex gap-2 flex-wrap">
          <button
            className={`px-3 py-1 text-sm rounded ${!selectedTable ? "bg-blue-600 text-white" : "bg-gray-100 hover:bg-gray-200"}`}
            onClick={() => setSelectedTable(null)}
          >
            All Tables
          </button>
          {tables.map((table) => (
            <button
              key={table}
              className={`px-3 py-1 text-sm rounded ${selectedTable === table ? "bg-blue-600 text-white" : "bg-gray-100 hover:bg-gray-200"}`}
              onClick={() => setSelectedTable(table)}
            >
              {table}
            </button>
          ))}
        </div>
      )}

      {/* Relationship Cards */}
      {filteredRelationships.length > 0 ? (
        <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
          {filteredRelationships.map((rel, idx) => (
            <div
              key={idx}
              className="p-4 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-sm hover:shadow-md transition-shadow"
            >
              <div className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-2">
                  <Table className="w-4 h-4 text-blue-500" />
                  <span className="font-medium">{rel.from_table}</span>
                  <span className="text-xs text-gray-500">
                    .{rel.from_column}
                  </span>
                </div>
                <ArrowRight className="w-4 h-4 text-gray-400" />
                <div className="flex items-center gap-2">
                  <Table className="w-4 h-4 text-green-500" />
                  <span className="font-medium">{rel.to_table}</span>
                  <span className="text-xs text-gray-500">
                    .{rel.to_column}
                  </span>
                </div>
              </div>
            </div>
          ))}
        </div>
      ) : (
        !loading && (
          <div className="flex flex-col items-center justify-center h-40 text-gray-500">
            <p>No foreign key relationships found in this database</p>
            <p className="text-sm mt-1">
              Tables using InnoDB with FK constraints will appear here
            </p>
          </div>
        )
      )}

      {loading && (
        <div className="flex items-center justify-center h-40">
          <Loader2 className="w-8 h-8 animate-spin text-blue-500" />
        </div>
      )}
    </div>
  );
}
