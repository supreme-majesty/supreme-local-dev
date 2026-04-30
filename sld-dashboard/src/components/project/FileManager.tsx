import { useState, useEffect, useCallback } from "react";
import { 
  Folder, 
  File, 
  ChevronRight, 
  Save, 
  X, 
  Settings, 
  RefreshCw,
  FileCode,
  FileText,
  Database
} from "lucide-react";
import Editor from "@monaco-editor/react";
import { Button } from "@/components/ui/Button";
import { cn } from "@/lib/utils";
import { useProjectFiles, useProjectFileContent, useSaveProjectFileMutation } from "@/hooks/use-project-files";
import type { FileInfo } from "@/api/daemon";

interface FileManagerProps {
  projectPath: string;
}

export function FileManager({ projectPath }: FileManagerProps) {
  const [currentPath, setCurrentPath] = useState(projectPath);
  const [selectedFile, setSelectedFile] = useState<FileInfo | null>(null);
  const [editorContent, setEditorContent] = useState("");
  const [isDirty, setIsDirty] = useState(false);
  const [autoSave, setAutoSave] = useState(false);
  
  const { data: files = [], isLoading, refetch } = useProjectFiles(currentPath);
  const { data: fileContent, isLoading: isLoadingContent } = useProjectFileContent(selectedFile?.path || null);
  const saveMutation = useSaveProjectFileMutation();

  const handleSave = useCallback(() => {
    if (selectedFile) {
      saveMutation.mutate({
        path: selectedFile.path,
        content: editorContent,
      }, {
        onSuccess: () => setIsDirty(false)
      });
    }
  }, [selectedFile, saveMutation, editorContent]);

  // Auto-save logic
  useEffect(() => {
    if (!autoSave || !isDirty || !selectedFile) return;

    const timer = setTimeout(() => {
      handleSave();
    }, 2000); // Save after 2 seconds of inactivity

    return () => clearTimeout(timer);
  }, [editorContent, autoSave, isDirty, selectedFile, handleSave]);

  useEffect(() => {
    if (fileContent) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setEditorContent(fileContent.content);
      setIsDirty(false);
    }
  }, [fileContent]);

  const handleFileClick = (file: FileInfo) => {
    if (file.is_dir) {
      setCurrentPath(file.path);
    } else {
      setSelectedFile(file);
    }
  };

  const goBack = () => {
    if (currentPath !== projectPath) {
      const parent = currentPath.substring(0, currentPath.lastIndexOf("/"));
      setCurrentPath(parent || projectPath);
    }
  };

  const getFileIcon = (file: FileInfo) => {
    if (file.is_dir) return <Folder size={16} className="text-blue-400" />;
    const name = file.name.toLowerCase();
    if (name === ".env") return <Settings size={16} className="text-yellow-400" />;
    if (name.endsWith(".php")) return <FileCode size={16} className="text-purple-400" />;
    if (name.endsWith(".js") || name.endsWith(".ts") || name.endsWith(".tsx")) return <FileCode size={16} className="text-blue-500" />;
    if (name.endsWith(".json")) return <FileText size={16} className="text-orange-400" />;
    if (name.endsWith(".sql")) return <Database size={16} className="text-green-400" />;
    return <File size={16} className="text-gray-400" />;
  };

  const getLanguage = (fileName: string) => {
    const ext = fileName.split(".").pop()?.toLowerCase();
    switch (ext) {
      case "php": return "php";
      case "js":
      case "jsx": return "javascript";
      case "ts":
      case "tsx": return "typescript";
      case "json": return "json";
      case "css": return "css";
      case "html": return "html";
      case "md": return "markdown";
      case "sql": return "sql";
      case "env": return "ini";
      default: return "plaintext";
    }
  };

  return (
    <div className="flex h-[600px] border border-[var(--border)] rounded-lg overflow-hidden bg-[var(--card)]">
      {/* Sidebar: File List */}
      <div className="w-64 border-r border-[var(--border)] flex flex-col bg-[var(--muted)]/10">
        <div className="p-3 border-b border-[var(--border)] flex items-center justify-between bg-[var(--muted)]/20">
          <span className="text-xs font-bold uppercase tracking-wider text-[var(--muted-foreground)]">Explorer</span>
          <button onClick={() => refetch()} className="p-1 hover:bg-[var(--muted)] rounded transition-colors">
            <RefreshCw size={14} className={cn(isLoading && "animate-spin")} />
          </button>
        </div>
        
        <div className="flex-1 overflow-auto">
          {currentPath !== projectPath && (
            <div 
              className="flex items-center gap-2 px-3 py-2 text-sm cursor-pointer hover:bg-[var(--muted)]/50 text-[var(--muted-foreground)]"
              onClick={goBack}
            >
              <ChevronRight size={14} className="rotate-180" />
              <span>..</span>
            </div>
          )}
          
          {files.map((file) => (
            <div 
              key={file.path}
              className={cn(
                "flex items-center gap-2 px-3 py-2 text-sm cursor-pointer transition-colors",
                selectedFile?.path === file.path ? "bg-[var(--primary)]/10 text-[var(--primary)]" : "hover:bg-[var(--muted)]/50"
              )}
              onClick={() => handleFileClick(file)}
            >
              {getFileIcon(file)}
              <span className="truncate">{file.name}</span>
              {file.is_dir && <ChevronRight size={14} className="ml-auto text-[var(--muted-foreground)]" />}
            </div>
          ))}
        </div>
      </div>

      {/* Main Area: Editor */}
      <div className="flex-1 flex flex-col bg-[var(--background)]">
        {selectedFile ? (
          <>
            {/* Editor Toolbar */}
            <div className="p-2 border-b border-[var(--border)] flex items-center justify-between bg-[var(--card)]">
              <div className="flex items-center gap-2 px-2">
                {getFileIcon(selectedFile)}
                <span className="text-sm font-medium">{selectedFile.name}</span>
                {isDirty && <span className="w-2 h-2 rounded-full bg-orange-500" title="Unsaved changes" />}
              </div>
              
              <div className="flex items-center gap-2">
                <label className="flex items-center gap-2 mr-4 cursor-pointer">
                  <div 
                    className={cn(
                      "w-8 h-4 rounded-full relative transition-colors",
                      autoSave ? "bg-emerald-500" : "bg-[var(--muted)]"
                    )}
                    onClick={() => setAutoSave(!autoSave)}
                  >
                    <div className={cn(
                      "absolute top-0.5 w-3 h-3 rounded-full bg-white transition-all",
                      autoSave ? "left-4.5" : "left-0.5"
                    )} />
                  </div>
                  <span className="text-[10px] font-bold uppercase tracking-wider text-[var(--muted-foreground)]">Auto Save</span>
                </label>

                <Button 
                  size="sm" 
                  onClick={handleSave} 
                  disabled={!isDirty || saveMutation.isPending}
                  className="h-8 gap-1.5"
                >
                  <Save size={14} />
                  {saveMutation.isPending ? "Saving..." : "Save"}
                </Button>
                <Button 
                  variant="ghost" 
                  size="sm" 
                  onClick={() => setSelectedFile(null)}
                  className="h-8 w-8 p-0"
                >
                  <X size={16} />
                </Button>
              </div>
            </div>

            {/* Editor Content */}
            <div className="flex-1 relative">
              {isLoadingContent ? (
                <div className="absolute inset-0 flex items-center justify-center bg-[var(--background)]/50 z-10">
                  <RefreshCw size={24} className="animate-spin text-[var(--primary)]" />
                </div>
              ) : null}
              
              <Editor
                height="100%"
                theme="vs-dark"
                language={getLanguage(selectedFile.name)}
                value={editorContent}
                onChange={(val) => {
                  setEditorContent(val || "");
                  setIsDirty(true);
                }}
                options={{
                  minimap: { enabled: false },
                  fontSize: 13,
                  scrollBeyondLastLine: false,
                  automaticLayout: true,
                  padding: { top: 10, bottom: 10 },
                  suggestOnTriggerCharacters: true,
                  quickSuggestions: {
                    other: true,
                    comments: true,
                    strings: true,
                  },
                  wordBasedSuggestions: "allDocuments",
                  parameterHints: { enabled: true },
                  folding: true,
                  links: true,
                  formatOnPaste: true,
                  formatOnType: true,
                }}
              />
            </div>
          </>
        ) : (
          <div className="flex-1 flex flex-col items-center justify-center text-[var(--muted-foreground)] p-8 text-center">
            <div className="w-16 h-16 rounded-2xl bg-[var(--muted)]/20 flex items-center justify-center mb-4">
              <FileCode size={32} className="opacity-20" />
            </div>
            <h3 className="text-lg font-medium text-[var(--foreground)] mb-1">Select a file to edit</h3>
            <p className="text-sm max-w-xs">
              Browse your project files in the explorer and click on any file to open it in the editor.
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
