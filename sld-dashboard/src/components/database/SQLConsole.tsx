interface SQLConsoleProps {
  selectedDB: string | null;
  onExecute: (query: string) => void;
}

export function SQLConsole({ selectedDB, onExecute }: SQLConsoleProps) {
  return (
    <div className="p-4 border rounded">
      <h3 className="font-bold">SQL Console Placeholder</h3>
      <p>Selected DB: {selectedDB || "None"}</p>
      <button
        className="px-2 py-1 bg-blue-500 text-white rounded mt-2"
        onClick={() => onExecute("SELECT 1")}
      >
        Test Query
      </button>
    </div>
  );
}
