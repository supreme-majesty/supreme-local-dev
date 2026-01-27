import { useState } from "react";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Label } from "@/components/ui/Label";
import { useCreateDatabaseMutation } from "@/hooks/use-database";
import { Database } from "lucide-react";

interface CreateDatabaseModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export function CreateDatabaseModal({
  isOpen,
  onClose,
}: CreateDatabaseModalProps) {
  const [name, setName] = useState("");
  const createMutation = useCreateDatabaseMutation();

  const handleCreate = async () => {
    if (!name.trim()) return;

    createMutation.mutate(name, {
      onSuccess: () => {
        setName("");
        onClose();
      },
    });
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="Create New Database"
      footer={
        <div className="flex justify-end gap-3">
          <Button
            variant="ghost"
            onClick={onClose}
            disabled={createMutation.isPending}
          >
            Cancel
          </Button>
          <Button
            onClick={handleCreate}
            loading={createMutation.isPending}
            disabled={!name.trim()}
          >
            <Database size={16} className="mr-2" />
            Create Database
          </Button>
        </div>
      }
    >
      <div className="space-y-4">
        <div className="p-3 bg-purple-500/10 text-purple-500 rounded-lg text-sm">
          Create a fresh database on your local server.
        </div>

        <div className="space-y-2">
          <Label>Database Name</Label>
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="my_new_app"
            autoFocus
            onKeyDown={(e) => {
              if (e.key === "Enter" && name.trim()) {
                handleCreate();
              }
            }}
          />
        </div>
      </div>
    </Modal>
  );
}
