import {
  ReactFlow,
  Background,
  Controls,
  Handle,
  Position,
  useNodesState,
  useEdgesState,
  type NodeProps,
  type Edge,
  type Node,
  MarkerType,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Table as TableIcon, Key, Link as LinkIcon, Database, Edit2 } from "lucide-react";
import { useTables, useExecuteQueryMutation } from "@/hooks/use-database";
import { api } from "@/api/daemon";
import { useEffect, useState } from "react";
import { AlterTableDesigner } from "./AlterTableDesigner";

type TableNodeData = {
  label: string;
  columns: any[];
  database: string;
  onEdit?: (table: string) => void;
};

type CustomNode = Node<TableNodeData, "table">;

// --- Custom Table Node ---
const TableNode = ({ data }: NodeProps<CustomNode>) => {
  return (
    <div className="bg-[var(--card)] border border-[var(--border)] rounded-lg shadow-xl min-w-[200px] overflow-hidden">
      <div className="bg-[var(--primary)]/10 px-3 py-2 border-b border-[var(--border)] flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <TableIcon size={14} className="text-[var(--primary)]" />
          <span className="font-bold text-sm text-[var(--foreground)]">{data.label}</span>
        </div>
        {data.onEdit && (
          <button
            onClick={() => data.onEdit!(data.label)}
            className="text-[var(--muted-foreground)] hover:text-[var(--primary)] p-1 rounded hover:bg-[var(--primary)]/20 transition-colors"
            title="Alter Table"
          >
            <Edit2 size={14} />
          </button>
        )}
      </div>
      <div className="p-0">
        {data.columns?.map((col: any) => (
          <div
            key={col.name}
            id={`${data.label}-${col.name}`}
            className="px-3 py-1.5 flex items-center justify-between group hover:bg-[var(--muted)]/30 relative border-b border-[var(--border)] last:border-0"
          >
            {/* Foreign Key Handle (Left) */}
            <Handle
              type="target"
              position={Position.Left}
              id={col.name}
              style={{ background: 'var(--primary)', width: 6, height: 6, left: -4 }}
            />
            
            <div className="flex items-center gap-2">
              {col.key === "PRI" && <Key size={10} className="text-yellow-500" />}
              {col.foreign_key && <LinkIcon size={10} className="text-blue-400" />}
              <span className={`text-xs ${col.key === "PRI" ? "font-bold text-[var(--foreground)]" : "text-[var(--muted-foreground)]"}`}>
                {col.name}
              </span>
            </div>
            
            <span className="text-[10px] opacity-40 font-mono uppercase ml-4">
              {col.type.split('(')[0]}
            </span>

            {/* Foreign Key Handle (Right) */}
            <Handle
              type="source"
              position={Position.Right}
              id={col.name}
              style={{ background: 'var(--primary)', width: 6, height: 6, right: -4 }}
            />
          </div>
        ))}
      </div>
    </div>
  );
};

const nodeTypes = {
  table: TableNode,
};

interface SchemaCanvasProps {
  database: string;
}

export function SchemaCanvas({ database }: SchemaCanvasProps) {
  const { data: tables = [], isLoading: loadingTables, refetch } = useTables(database);
  const [nodes, setNodes, onNodesChange] = useNodesState<CustomNode>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]);
  const [editingTable, setEditingTable] = useState<string | null>(null);
  
  const executeQueryMutation = useExecuteQueryMutation();

  const handleEditTable = (table: string) => {
    setEditingTable(table);
  };

  const handleSaveAlter = (sql: string) => {
    executeQueryMutation.mutate(
      { database, query: sql },
      {
        onSuccess: () => {
          setEditingTable(null);
          refetch(); // Refresh tables and therefore the graph
        },
      }
    );
  };

  // Fetch all table columns to build the graph
  // Note: In a real app, we might want a single endpoint for full schema
  // But we'll follow the existing patterns.
  
  useEffect(() => {
    if (tables.length === 0) return;

    const buildGraph = async () => {
      const newNodes: CustomNode[] = [];
      const newEdges: Edge[] = [];
      
      // Layout parameters
      const spacingX = 300;
      const spacingY = 400;
      const cols = Math.ceil(Math.sqrt(tables.length));

      for (let i = 0; i < tables.length; i++) {
        const table = tables[i];
        
        // Fetch columns for this table
        try {
            const columns = await api.getTableSchema(database, table.name);
            
            const x = (i % cols) * spacingX;
            const y = Math.floor(i / cols) * spacingY;

            newNodes.push({
              id: table.name,
              type: 'table',
              position: { x, y },
              data: { 
                label: table.name, 
                columns,
                database,
                onEdit: handleEditTable
              },
            });

            // Extract edges from foreign keys
            columns.forEach((col: any) => {
              if (col.foreign_key) {
                newEdges.push({
                  id: `e-${table.name}-${col.name}-${col.foreign_key.table}`,
                  source: table.name,
                  sourceHandle: col.name,
                  target: col.foreign_key.table,
                  targetHandle: col.foreign_key.column,
                  animated: true,
                  style: { stroke: 'var(--primary)', strokeWidth: 2 },
                  markerEnd: {
                    type: MarkerType.ArrowClosed,
                    color: 'var(--primary)',
                  },
                });
              }
            });
        } catch (e) {
            console.error(`Failed to fetch schema for ${table.name}`, e);
        }
      }

      setNodes(newNodes);
      setEdges(newEdges);
    };

    buildGraph();
  }, [tables, database, setNodes, setEdges]);

  if (loadingTables) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-[var(--muted-foreground)]">
        <Loader2 className="w-8 h-8 animate-spin mb-2" />
        <p>Architecting schema...</p>
      </div>
    );
  }

  return (
    <div className="w-full h-full bg-[var(--muted)]/5 min-h-[600px] border border-[var(--border)] rounded-lg overflow-hidden relative">
      <div className="absolute top-4 left-4 z-10 flex items-center gap-2 bg-[var(--card)]/80 backdrop-blur px-3 py-1.5 rounded-full border border-[var(--border)] shadow-sm">
        <Database size={14} className="text-[var(--primary)]" />
        <span className="text-xs font-medium">{database}</span>
        <div className="w-px h-3 bg-[var(--border)] mx-1" />
        <span className="text-[10px] text-[var(--muted-foreground)] uppercase tracking-wider">{tables.length} Tables</span>
      </div>
      
      <ReactFlow
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        nodeTypes={nodeTypes}
        fitView
        colorMode="dark"
      >
        <Background gap={20} size={1} color="var(--border)" />
        <Controls />
      </ReactFlow>

      {/* Alter Table Modal */}
      {editingTable && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm">
          <div className="w-full max-w-6xl max-h-[90vh] overflow-y-auto rounded-lg shadow-2xl">
            <AlterTableDesigner
              database={database}
              table={editingTable}
              onCancel={() => setEditingTable(null)}
              onSave={handleSaveAlter}
              isLoading={executeQueryMutation.isPending}
            />
          </div>
        </div>
      )}
    </div>
  );
}

const Loader2 = ({ className }: { className?: string }) => (
  <svg
    xmlns="http://www.w3.org/2000/svg"
    width="24"
    height="24"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    className={className}
  >
    <path d="M21 12a9 9 0 1 1-6.219-8.56" />
  </svg>
);
