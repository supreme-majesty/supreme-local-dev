import { useState } from "react";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Label } from "@/components/ui/Label";
import { useCloneDatabaseMutation } from "@/hooks/use-daemon";
import { Copy } from "lucide-react";

interface CloneDatabaseModalProps {
  isOpen: boolean;
  onClose: () => void;
  sourceDatabase: string;
}

export function CloneDatabaseModal({
  isOpen,
  onClose,
  sourceDatabase,
}: CloneDatabaseModalProps) {
  const [targetName, setTargetName] = useState(`${sourceDatabase}_clone`);
  const cloneMutation = useCloneDatabaseMutation();

  const handleClone = async () => {
    if (!targetName.trim()) return;

    cloneMutation.mutate(
      { source: sourceDatabase, target: targetName },
      {
        onSuccess: () => {
          onClose();
        },
      }
    );
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="Clone Database"
      footer={
        <div className="flex justify-end gap-3">
          <Button
            variant="ghost"
            onClick={onClose}
            disabled={cloneMutation.isPending}
          >
            Cancel
          </Button>
          <Button
            onClick={handleClone}
            loading={cloneMutation.isPending}
            disabled={!targetName.trim()}
          >
            <Copy size={16} className="mr-2" />
            Clone
          </Button>
        </div>
      }
    >
      <div className="space-y-4">
        <div className="p-3 bg-blue-500/10 text-blue-500 rounded-lg text-sm">
          Cloning <strong>{sourceDatabase}</strong> will create a new database
          with all tables and data.
        </div>

        <div className="space-y-2">
          <Label>Target Database Name</Label>
          <Input
            value={targetName}
            onChange={(e) => setTargetName(e.target.value)}
            placeholder="new_database_name"
            autoFocus
          />
        </div>
      </div>
    </Modal>
  );
}
