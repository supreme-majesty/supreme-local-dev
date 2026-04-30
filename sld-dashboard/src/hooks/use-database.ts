import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/daemon";
import { useAppStore } from "@/stores/useAppStore";

export const dbKeys = {
  all: ["db"] as const,
  list: () => [...dbKeys.all, "list"] as const,
  tables: (db: string) => [...dbKeys.all, "tables", db] as const,
  columns: (db: string, table: string) =>
    [...dbKeys.all, "columns", db, table] as const,
  indexes: (db: string, table: string) =>
    [...dbKeys.all, "indexes", db, table] as const,
  relationships: (db: string) =>
    [...dbKeys.all, "relationships", db] as const,
  data: (
    db: string,
    table: string,
    page: number,
    perPage: number,
    sortCol: string,
    sortOrder: string,
    profile: boolean,
  ) =>
    [
      ...dbKeys.all,
      "data",
      db,
      table,
      page,
      perPage,
      sortCol,
      sortOrder,
      profile,
    ] as const,
  snapshots: () => [...dbKeys.all, "snapshots"] as const,
};

export function useDatabases() {
  return useQuery({
    queryKey: dbKeys.list(),
    queryFn: () => api.getDatabases(),
  });
}

export function useTables(database: string | null) {
  return useQuery({
    queryKey: dbKeys.tables(database || ""),
    queryFn: () => api.getTables(database!),
    enabled: !!database,
  });
}

export function useCreateDatabaseMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (name: string) => api.createDatabase(name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: dbKeys.list() });
      addToast({ type: "success", title: "Database created" });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to create database",
        description: err.message,
      });
    },
  });
}

export function useDeleteDatabaseMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (name: string) => api.deleteDatabase(name),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: dbKeys.list() });
      addToast({ type: "success", title: "Database deleted" });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to delete database",
        description: err.message,
      });
    },
  });
}

export function useRenameDatabaseMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (vars: { oldName: string; newName: string }) =>
      api.renameDatabase(vars.oldName, vars.newName),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: dbKeys.list() });
      addToast({ type: "success", title: "Database renamed" });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Rename failed",
        description: err.message,
      });
    },
  });
}

export interface TableDataOptions {
  perPage?: number;
  sortCol?: string;
  sortOrder?: "ASC" | "DESC";
  profile?: boolean;
}

export function useTableData(
  database: string | null,
  table: string | null,
  page: number,
  options: TableDataOptions = {},
) {
  const {
    perPage = 50,
    sortCol = "",
    sortOrder = "ASC",
    profile = false,
  } = options;
  return useQuery({
    queryKey: dbKeys.data(
      database || "",
      table || "",
      page,
      perPage,
      sortCol,
      sortOrder,
      profile,
    ),
    queryFn: () =>
      api.getTableData(database!, table!, page, {
        perPage,
        sortCol,
        sortOrder,
        profile,
      }),
    enabled: !!database && !!table,
  });
}

export function useTableColumns(database: string | null, table: string | null) {
  return useQuery({
    queryKey: dbKeys.columns(database || "", table || ""),
    queryFn: () => api.getTableSchema(database!, table!),
    enabled: !!database && !!table,
  });
}

export function useTableIndexes(database: string | null, table: string | null) {
  return useQuery({
    queryKey: dbKeys.indexes(database || "", table || ""),
    queryFn: () => api.getTableIndexes(database!, table!),
    enabled: !!database && !!table,
  });
}

export function useTableRelationships(database: string | null) {
  return useQuery({
    queryKey: dbKeys.relationships(database || ""),
    queryFn: () => api.getDbRelationships(database!),
    enabled: !!database,
  });
}

export function useSnapshots() {
  return useQuery({
    queryKey: dbKeys.snapshots(),
    queryFn: () => api.getSnapshots(),
  });
}

export function useCreateSnapshotMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (vars: { database: string; table?: string }) =>
      api.createSnapshot(vars.database, vars.table),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: dbKeys.snapshots() });
      addToast({ type: "success", title: "Snapshot created" });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to create snapshot",
        description: err.message,
      });
    },
  });
}

export function useRestoreSnapshotMutation() {
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (filename: string) => api.restoreSnapshot(filename),
    onSuccess: () => {
      addToast({ type: "success", title: "Database restored" });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Restore failed",
        description: err.message,
      });
    },
  });
}

export function useDeleteSnapshotMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (filename: string) => api.deleteSnapshot(filename),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: dbKeys.snapshots() });
      addToast({ type: "success", title: "Snapshot deleted" });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Deletion failed",
        description: err.message,
      });
    },
  });
}

export function useExecuteQueryMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (vars: { database: string; query: string }) =>
      api.executeQuery(vars.database, vars.query),
    onSuccess: (data) => {
      // Invalidate relevant queries
      // We don't know exactly what changed, so invalidate tables and data for this DB
      queryClient.invalidateQueries({ queryKey: dbKeys.all });

      if (data.error) {
        addToast({
          type: "error",
          title: "Query Error",
          description: data.error,
        });
      } else {
        // Only show success if it wasn't a select (usually)
        // But here we use it for Delete/Insert so yes.
        addToast({ type: "success", title: "Action completed successfully" });
      }
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Action failed",
        description: err.message,
      });
    },
  });
}

export function useImportDatabaseMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (vars: { file: File; database: string; restore?: boolean }) =>
      api.importDatabase(vars.file, vars.database, vars.restore),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: dbKeys.all });
      addToast({ type: "success", title: "Database imported successfully" });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Import failed",
        description: err.message,
      });
    },
  });
}
export function useCloneDatabaseMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (vars: { 
      source: string; 
      target: string; 
      mode: string;
      create_db?: boolean;
      add_drop?: boolean;
      add_auto_inc?: boolean;
      add_constraints?: boolean;
    }) =>
      api.cloneDatabase(vars.source, vars.target, {
        mode: vars.mode,
        create_db: vars.create_db,
        add_drop: vars.add_drop,
        add_auto_inc: vars.add_auto_inc,
        add_constraints: vars.add_constraints
      }),
    onSuccess: (_, vars) => {
      queryClient.invalidateQueries({ queryKey: dbKeys.list() });
      addToast({ 
        type: "success", 
        title: "Database cloned", 
        description: `Successfully cloned ${vars.source} to ${vars.target}` 
      });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Failed to clone database",
        description: err.message,
      });
    },
  });
}


export function useMaintenanceMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (vars: { database: string; tables: string[]; operation: string }) =>
      api.maintenance(vars.database, vars.tables, vars.operation),
    onSuccess: (_, vars) => {
      queryClient.invalidateQueries({ queryKey: dbKeys.tables(vars.database) });
      addToast({
        type: "success",
        title: "Maintenance complete",
        description: `Successfully performed ${vars.operation} on ${vars.tables.length || 'all'} tables.`
      });
    }
  });
}

export function useDBSearchQuery(db: string, query: string) {
  return useQuery({
    queryKey: ["db", "search", db, query],
    enabled: !!db && query.length >= 2,
    queryFn: () => api.search(db, query),
  });
}

export function useCollations() {
  return useQuery({
    queryKey: [...dbKeys.all, "collations"],
    queryFn: () => api.getCollations(),
  });
}

export function useDatabaseSettings(db: string) {
  return useQuery({
    queryKey: [...dbKeys.all, "settings", db],
    queryFn: () => api.getDatabaseSettings(db),
    enabled: !!db,
  });
}

export function useAnalyzeImportMutation() {
  return useMutation({
    mutationFn: (vars: { file: File; format?: string }) =>
      api.analyzeImport(vars.file, vars.format),
  });
}

export function useExecuteImportMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (vars: {
      database: string;
      table: string;
      temp_path: string;
      format: string;
      mapping: Record<string, string>;
    }) => api.executeImport(vars),
    onSuccess: (_, vars) => {
      queryClient.invalidateQueries({ queryKey: dbKeys.data(vars.database, vars.table, 1, 50, "", "ASC", false) });
      addToast({ type: "success", title: "Import successful" });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Import failed",
        description: err.message,
      });
    },
  });
}

export function useSeedTableMutation() {
  const queryClient = useQueryClient();
  const addToast = useAppStore((s) => s.addToast);

  return useMutation({
    mutationFn: (vars: {
      database: string;
      table: string;
      count: number;
      fakers: Record<string, string>;
    }) => api.seedTable(vars),
    onSuccess: (_, vars) => {
      queryClient.invalidateQueries({ queryKey: dbKeys.data(vars.database, vars.table, 1, 50, "", "ASC", false) });
      addToast({ type: "success", title: `Successfully seeded ${vars.count} rows` });
    },
    onError: (err: Error) => {
      addToast({
        type: "error",
        title: "Seeding failed",
        description: err.message,
      });
    },
  });
}

export function useDatabaseStats(database: string) {
  return useQuery({
    queryKey: [...dbKeys.all, "stats", database],
    queryFn: () => api.getStats(database),
    enabled: !!database,
    refetchInterval: 5000, // Refresh every 5s
  });
}

export function useMigrations(database: string) {
  return useQuery({
    queryKey: [...dbKeys.all, "migrations", database],
    queryFn: () => api.getMigrations(database),
    enabled: !!database,
  });
}

export function useInitMigrationsMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (vars: { database: string }) => api.initMigrations(vars.database),
    onSuccess: (_, vars) => {
      queryClient.invalidateQueries({ queryKey: [...dbKeys.all, "migrations", vars.database] });
    },
  });
}

export function useCreateMigrationMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (vars: { database: string, name: string }) => api.createMigration(vars.database, vars.name),
    onSuccess: (_, vars) => {
      queryClient.invalidateQueries({ queryKey: [...dbKeys.all, "migrations", vars.database] });
    },
  });
}

export function useRunMigrationMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (vars: { database: string, filename: string }) => api.runMigration(vars.database, vars.filename),
    onSuccess: (_, vars) => {
      queryClient.invalidateQueries({ queryKey: [...dbKeys.all, "migrations", vars.database] });
      queryClient.invalidateQueries({ queryKey: [...dbKeys.all, "tables", vars.database] });
    },
  });
}

export function useSchemaDiff(source: string, target: string) {
  return useQuery({
    queryKey: [...dbKeys.all, "diff", source, target],
    queryFn: () => api.compareSchemas(source, target),
    enabled: !!source && !!target,
  });
}

export function useOptimizationSuggestions(database: string, table: string) {
  return useQuery({
    queryKey: [...dbKeys.all, "optimize", database, table],
    queryFn: () => api.getOptimizationSuggestions(database, table),
    enabled: !!database && !!table,
  });
}

export function usePIIScan(database: string, table: string) {
  return useQuery({
    queryKey: ['db-pii-scan', database, table],
    queryFn: () => api.scanPII(database, table),
    enabled: !!database && !!table,
  });
}

export function useMaskDataMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (config: unknown) => api.maskData(config),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['db-data'] });
    },
  });
}

export function useQueryPlanAnalysisMutation() {
  return useMutation({
    mutationFn: ({ database, query }: { database: string; query: string }) => 
      api.analyzeQueryPlan(database, query),
  });
}

export function useSchemaDocs(database: string) {
  return useQuery({
    queryKey: ['db-docs', database],
    queryFn: () => api.generateDocs(database),
    enabled: !!database,
  });
}

export function useSnippets() {
  return useQuery({
    queryKey: ['db-snippets'],
    queryFn: () => api.getSnippets(),
  });
}

export function useSaveSnippetMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (snippet: unknown) => api.saveSnippet(snippet),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['db-snippets'] });
    },
  });
}

export function useAuditLog() {
  return useQuery({
    queryKey: ['db-audit'],
    queryFn: () => api.getAuditLog(),
    refetchInterval: 10000,
  });
}

export function useImportDataMutation() {
  return useMutation({
    mutationFn: (config: unknown) => api.importData(config),
  });
}

export function useProfiles() {
  return useQuery({
    queryKey: ['db-profiles'],
    queryFn: () => api.getProfiles(),
  });
}

export function useSaveProfileMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (profile: unknown) => api.saveProfile(profile),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['db-profiles'] }),
  });
}

export function useSwitchEnvironmentMutation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.switchEnvironment(id),
    onSuccess: () => {
      queryClient.invalidateQueries();
      window.location.reload(); // Hard reload to clear all cached state for the new env
    },
  });
}
