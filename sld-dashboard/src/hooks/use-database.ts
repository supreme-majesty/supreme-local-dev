import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/api/daemon";
import { useAppStore } from "@/stores/useAppStore";

export const dbKeys = {
  all: ["db"] as const,
  list: () => [...dbKeys.all, "list"] as const,
  tables: (db: string) => [...dbKeys.all, "tables", db] as const,
  columns: (db: string, table: string) =>
    [...dbKeys.all, "columns", db, table] as const,
  data: (db: string, table: string, page: number) =>
    [...dbKeys.all, "data", db, table, page] as const,
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

export function useTableData(
  database: string | null,
  table: string | null,
  page: number
) {
  return useQuery({
    queryKey: dbKeys.data(database || "", table || "", page),
    queryFn: () => api.getTableData(database!, table!, page),
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
