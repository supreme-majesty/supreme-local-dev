interface TableCreatorProps {
  database: string;
  onCancel: () => void;
  onSave: (query: string) => void;
  isLoading?: boolean;
}

export function TableCreator({
  database,
  onCancel,
  onSave,
  isLoading,
}: TableCreatorProps) {
  void database;
  void isLoading;
  return (
    <div>
      <h3>Table Creator Placeholder</h3>
      <button onClick={() => onSave("CREATE TABLE ...")}>Create</button>
      <button onClick={onCancel}>Cancel</button>
    </div>
  );
}
