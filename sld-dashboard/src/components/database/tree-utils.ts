export interface TreeNode {
  name: string;
  fullName: string;
  isDatabase: boolean;
  children: Record<string, TreeNode>;
}

export function buildDatabaseTree(databases: { name: string }[]): TreeNode[] {
  return [...databases]
    .map((db) => ({
      name: db.name,
      fullName: db.name,
      isDatabase: true,
      children: {},
    }))
    .sort((a, b) => a.name.localeCompare(b.name));
}
