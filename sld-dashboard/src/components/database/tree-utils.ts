export interface TreeNode {
  name: string;
  fullName: string;
  isDatabase: boolean;
  children: Record<string, TreeNode>;
}

export function buildDatabaseTree(databases: { name: string }[]): TreeNode[] {
  const rootObj: Record<string, TreeNode> = {};

  databases.forEach((db) => {
    const parts = db.name.split("_");
    let currentLevel = rootObj;
    let currentFullName = "";

    parts.forEach((part, index) => {
      const isLast = index === parts.length - 1;
      // Reconstruct the fullname at this level
      currentFullName = currentFullName ? `${currentFullName}_${part}` : part;

      if (!currentLevel[part]) {
        currentLevel[part] = {
          name: part,
          fullName: currentFullName,
          isDatabase: false,
          children: {},
        };
      }

      if (isLast) {
        currentLevel[part].isDatabase = true;
      }

      currentLevel = currentLevel[part].children;
    });
  });

  // Helper to compress the tree recursively
  const compressNode = (node: TreeNode): TreeNode => {
    // Recursively compress children first
    Object.keys(node.children).forEach((key) => {
      node.children[key] = compressNode(node.children[key]);
    });

    // If I am NOT a database and I have EXACTLY ONE child
    // Then I can be merged into my child (or rather, my child replaces me with a longer name)
    const childKeys = Object.keys(node.children);
    if (!node.isDatabase && childKeys.length === 1) {
      const child = node.children[childKeys[0]];
      // Create a merged node
      // Name becomes "myname_childname"
      // Fullname is preserved from the child (as it's the deeper one)
      // Children are adopted from the child
      // isDatabase is adopted from the child
      return {
        ...child,
        name: `${node.name}_${child.name}`,
      };
    }

    return node;
  };

  // Compress root nodes
  const compressedRoots = Object.values(rootObj).map(compressNode);

  // Sort
  return compressedRoots.sort((a, b) => a.name.localeCompare(b.name));
}
