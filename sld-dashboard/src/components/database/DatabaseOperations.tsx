import { useState, useEffect } from "react";
import { 
  Plus, 
  RefreshCw, 
  Copy, 
  Trash2, 
  Table, 
  AlertTriangle,
  Globe,
  ShieldCheck
} from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Card, CardHeader, CardTitle, CardContent, CardDescription } from "@/components/ui/Card";
import { 
  useRenameDatabaseMutation, 
  useCloneDatabaseMutation, 
  useDeleteDatabaseMutation, 
  useExecuteQueryMutation,
  useCollations,
  useDatabaseSettings,
  useTables
} from "@/hooks/use-database";
import { useAppStore } from "@/stores/useAppStore";

interface DatabaseOperationsProps {
  database: string;
  onSelectDb: (db: string) => void;
  onCreateTable: () => void;
}

export function DatabaseOperations({ database, onSelectDb, onCreateTable }: DatabaseOperationsProps) {
  const [newName, setNewName] = useState(database);
  const [copyName, setCopyName] = useState(`${database}_copy`);
  const [copyOption, setCopyOption] = useState<"structure" | "both" | "data">("both");
  const [selectedCollation, setSelectedCollation] = useState("");
  const [changeAllTables, setChangeAllTables] = useState(false);

  const addToast = useAppStore((s: any) => s.addToast);

  const renameMutation = useRenameDatabaseMutation();
  const cloneMutation = useCloneDatabaseMutation();
  const deleteMutation = useDeleteDatabaseMutation();
  const executeMutation = useExecuteQueryMutation();
  
  const { data: collations, isLoading: loadingCollations } = useCollations();
  const { data: currentSettings } = useDatabaseSettings(database);
  const { data: tables } = useTables(database);

  useEffect(() => {
    if (currentSettings?.collation) {
      setSelectedCollation(currentSettings.collation);
    }
  }, [currentSettings]);

  const handleRename = () => {
    if (!newName || newName === database) return;
    renameMutation.mutate({ oldName: database, newName }, {
      onSuccess: () => onSelectDb(newName)
    });
  };

  const handleCopy = () => {
    if (!copyName) return;
    cloneMutation.mutate({ source: database, target: copyName, mode: copyOption }, {
      onSuccess: () => onSelectDb(copyName)
    });
  };

  const handleSetCollation = async () => {
    if (!selectedCollation) return;
    
    const collationObj = collations?.find(c => c.name === selectedCollation);
    if (!collationObj) return;

    try {
      // 1. Alter Database
      await executeMutation.mutateAsync({
        database,
        query: `ALTER DATABASE \`${database}\` CHARACTER SET ${collationObj.charset} COLLATE ${selectedCollation}`
      });

      // 2. Alter All Tables if checked
      if (changeAllTables && tables) {
        addToast({ 
          type: "info", 
          title: "Updating Tables", 
          description: `Updating collation for ${tables.length} tables...` 
        });

        for (const table of tables) {
          await executeMutation.mutateAsync({
            database,
            query: `ALTER TABLE \`${table.name}\` CONVERT TO CHARACTER SET ${collationObj.charset} COLLATE ${selectedCollation}`
          });
        }
      }

      addToast({ 
        type: "success", 
        title: "Collation updated", 
        description: `Database ${database} ${changeAllTables ? "and all tables " : ""}updated to ${selectedCollation}` 
      });
    } catch (err: any) {
      addToast({
        type: "error",
        title: "Failed to update collation",
        description: err.message
      });
    }
  };

  return (
    <div className="space-y-6 pb-12 mt-4 px-6">
      {/* Create New Table Quick Access */}
      <Card className="border-emerald-500/20 bg-emerald-500/5">
        <CardHeader className="pb-3">
          <CardTitle className="text-base flex items-center gap-2 text-emerald-600">
            <Plus size={18} /> Create new table
          </CardTitle>
          <CardDescription>Quickly add a new table to <strong>{database}</strong></CardDescription>
        </CardHeader>
        <CardContent className="flex gap-4">
          <Button onClick={onCreateTable} className="bg-emerald-600 hover:bg-emerald-700">
            <Table size={16} className="mr-2" /> Start Table Designer
          </Button>
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        {/* Rename Database */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <RefreshCw size={18} className="text-blue-500" /> Rename database to
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <Input 
              value={newName} 
              onChange={(e) => setNewName(e.target.value)} 
              placeholder="New database name..."
            />
            <div className="flex items-center gap-2 text-xs text-[var(--muted-foreground)]">
              <ShieldCheck size={14} className="text-emerald-500" />
              <span>Adjusts privileges automatically</span>
            </div>
            <Button 
              onClick={handleRename} 
              disabled={renameMutation.isPending || !newName || newName === database}
              className="w-full"
            >
              {renameMutation.isPending ? "Renaming..." : "Go"}
            </Button>
          </CardContent>
        </Card>

        {/* Copy Database */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Copy size={18} className="text-purple-500" /> Copy database to
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <Input 
              value={copyName} 
              onChange={(e) => setCopyName(e.target.value)} 
              placeholder="Copy to..."
            />
            
            <div className="space-y-2">
              <label className="flex items-center gap-2 text-sm cursor-pointer group">
                <input 
                  type="radio" 
                  name="copy_opt" 
                  checked={copyOption === "both"} 
                  onChange={() => setCopyOption("both")}
                  className="accent-[var(--primary)]"
                />
                <span className="group-hover:text-[var(--primary)]">Structure and data</span>
              </label>
              <label className="flex items-center gap-2 text-sm cursor-pointer group">
                <input 
                  type="radio" 
                  name="copy_opt" 
                  checked={copyOption === "structure"} 
                  onChange={() => setCopyOption("structure")}
                  className="accent-[var(--primary)]"
                />
                <span className="group-hover:text-[var(--primary)]">Structure only</span>
              </label>
              <label className="flex items-center gap-2 text-sm cursor-pointer group">
                <input 
                  type="radio" 
                  name="copy_opt" 
                  checked={copyOption === "data"} 
                  onChange={() => setCopyOption("data")}
                  className="accent-[var(--primary)]"
                />
                <span className="group-hover:text-[var(--primary)]">Data only</span>
              </label>
            </div>

            <Button 
              onClick={handleCopy} 
              disabled={cloneMutation.isPending || !copyName}
              variant="secondary"
              className="w-full"
            >
              {cloneMutation.isPending ? "Copying..." : "Go"}
            </Button>
          </CardContent>
        </Card>
      </div>

      {/* Collation */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <Globe size={18} className="text-orange-500" /> Collation
          </CardTitle>
          <CardDescription>
            Change the default character set and collation for <strong>{database}</strong>.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-col md:flex-row items-end gap-4">
            <div className="flex-1 w-full space-y-2">
              <label className="text-xs font-medium text-[var(--muted-foreground)]">Select Collation</label>
              <div className="relative">
                <select 
                  value={selectedCollation} 
                  onChange={(e) => setSelectedCollation(e.target.value)}
                  className="w-full bg-[var(--background)] border border-[var(--border)] rounded-md h-10 px-3 text-sm focus:outline-none focus:ring-2 focus:ring-[var(--primary)]/50 appearance-none"
                  disabled={loadingCollations}
                >
                  <option value="">{loadingCollations ? "Loading collations..." : "Select a collation..."}</option>
                  {collations?.map(c => (
                    <option key={c.name} value={c.name}>{c.name} ({c.charset})</option>
                  ))}
                </select>
                <div className="absolute inset-y-0 right-0 flex items-center px-2 pointer-events-none text-[var(--muted-foreground)]">
                  <RefreshCw size={14} className={loadingCollations ? "animate-spin" : ""} />
                </div>
              </div>
            </div>
            <Button 
              onClick={handleSetCollation} 
              disabled={executeMutation.isPending || !selectedCollation}
              className="w-full md:w-auto"
            >
              {executeMutation.isPending ? "Applying..." : "Apply Collation"}
            </Button>
          </div>

          <div className="pt-2">
            <label className="flex items-center gap-3 p-3 border border-[var(--border)] rounded-lg hover:bg-[var(--muted)]/20 cursor-pointer transition-colors group">
              <input 
                type="checkbox" 
                checked={changeAllTables} 
                onChange={(e) => setChangeAllTables(e.target.checked)}
                className="w-4 h-4 accent-[var(--primary)]"
              />
              <div className="flex-1">
                <div className="text-sm font-medium group-hover:text-[var(--primary)] transition-colors">Change all tables collations</div>
                <div className="text-xs text-[var(--muted-foreground)]">This will recursively update every table in this database to the new collation.</div>
              </div>
            </label>
          </div>
        </CardContent>
      </Card>

      {/* Danger Zone */}
      <Card className="border-red-500/20 bg-red-500/5">
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2 text-red-600">
            <AlertTriangle size={18} /> Danger Zone
          </CardTitle>
          <CardDescription>Actions here cannot be undone. Be careful.</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-between p-4 border border-red-500/20 rounded-lg bg-white/50">
            <div>
              <h4 className="text-sm font-semibold text-red-700">Drop the database</h4>
              <p className="text-xs text-red-600/70">Permanently delete the database and all its tables.</p>
            </div>
            <Button 
              variant="danger" 
              onClick={() => {
                if(confirm(`Are you sure you want to PERMANENTLY DROP database "${database}"?`)) {
                  deleteMutation.mutate(database, {
                    onSuccess: () => onSelectDb("")
                  });
                }
              }}
              disabled={deleteMutation.isPending}
            >
              <Trash2 size={14} className="mr-2" /> Drop Database
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
