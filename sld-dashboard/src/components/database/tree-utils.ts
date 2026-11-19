export interface TreeDatabaseInput {
  name: string;
  tables?: number;
  source_id?: string;
  source_name?: string;
  source_type?: "default" | "docker";
  driver?: "mysql" | "postgres";
}

export interface TreeNode {
  key: string;
  name: string;
  fullName: string;
  isDatabase: boolean;
  tables?: number;
  sourceId?: string;
  sourceName?: string;
  sourceType?: "default" | "docker";
  driver?: "mysql" | "postgres";
  children: Record<string, TreeNode>;
}

/**
 * Builds a grouped prefix-alias tree from all databases across active/Docker servers.
 * Example:
 *   cecadoe_ride_db + cecadoe_test       -> cecadoe/ [ride_db, test]
 *   insightify_lms  + insightify_pos_db  -> insightify/ [lms, pos_db]
 *   tailormeasure   + tailormeasure_test -> tailormeasure (DB) + child test (DB)
 *   single_standalone_db                 -> single_standalone_db (compressed to root)
 *
 * When the exact same database name exists on multiple Docker servers (e.g. "app" on two
 * containers), both entries are preserved as distinct leaf children labeled by their server alias.
 */
export function buildDatabaseTree(databases: TreeDatabaseInput[]): TreeNode[] {
  const rootObj: Record<string, TreeNode> = {};

  // Detect duplicate database names across different servers so neither overwrites the other
  const nameCounts = new Map<string, number>();
  for (const db of databases) {
    nameCounts.set(db.name, (nameCounts.get(db.name) || 0) + 1);
  }

  for (const db of databases) {
    const isDuplicateName = (nameCounts.get(db.name) || 0) > 1;
    const parts = db.name.split("_").filter(Boolean);
    if (parts.length === 0) continue;

    let currentLevel = rootObj;
    let currentFullName = "";

    parts.forEach((part, index) => {
      const isLast = index === parts.length - 1;
      currentFullName = currentFullName ? `${currentFullName}_${part}` : part;

      if (!currentLevel[part]) {
        currentLevel[part] = {
          key: `group:${currentFullName}`,
          name: part,
          fullName: currentFullName,
          isDatabase: false,
          children: {},
        };
      }

      if (isLast) {
        if (isDuplicateName) {
          const aliasKey = db.source_id || db.source_name || "default";
          const displayAlias = db.source_name || aliasKey;
          currentLevel[part].children[aliasKey] = {
            key: `${aliasKey}::${db.name}`,
            name: displayAlias,
            fullName: db.name,
            isDatabase: true,
            tables: db.tables,
            sourceId: db.source_id,
            sourceName: db.source_name,
            sourceType: db.source_type,
            driver: db.driver,
            children: {},
          };
        } else {
          currentLevel[part].key = `${db.source_id || "default"}::${db.name}`;
          currentLevel[part].isDatabase = true;
          currentLevel[part].tables = db.tables;
          currentLevel[part].sourceId = db.source_id;
          currentLevel[part].sourceName = db.source_name;
          currentLevel[part].sourceType = db.source_type;
          currentLevel[part].driver = db.driver;
        }
      }

      currentLevel = currentLevel[part].children;
    });
  }

  // Recursively compress single-child non-database folder nodes so standalone DBs stay flat
  const compressNode = (node: TreeNode): TreeNode => {
    const compressedChildren: Record<string, TreeNode> = {};
    for (const [k, child] of Object.entries(node.children)) {
      compressedChildren[k] = compressNode(child);
    }
    node.children = compressedChildren;

    const childKeys = Object.keys(node.children);
    if (!node.isDatabase && childKeys.length === 1) {
      const child = node.children[childKeys[0]];
      return {
        ...child,
        name: `${node.name}_${child.name}`,
      };
    }

    return node;
  };

  return Object.values(rootObj)
    .map(compressNode)
    .sort((a, b) => a.name.localeCompare(b.name));
}
