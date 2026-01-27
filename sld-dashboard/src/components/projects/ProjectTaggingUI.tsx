import { useState } from "react";
import { Tag, X, Plus, Check } from "lucide-react";
import { api } from "@/api/daemon";

interface ProjectTaggingUIProps {
  domain: string;
  currentTags: string[];
  currentCategory: string;
  onUpdate?: () => void;
}

const PRESET_CATEGORIES = [
  "Work",
  "Personal",
  "Client",
  "Experimental",
  "Archive",
];

const PRESET_TAGS = [
  "laravel",
  "wordpress",
  "react",
  "vue",
  "api",
  "frontend",
  "backend",
  "wip",
  "production",
];

export function ProjectTaggingUI({
  domain,
  currentTags,
  currentCategory,
  onUpdate,
}: ProjectTaggingUIProps) {
  const [tags, setTags] = useState<string[]>(currentTags || []);
  const [category, setCategory] = useState(currentCategory || "");
  const [newTag, setNewTag] = useState("");
  const [saving, setSaving] = useState(false);

  const addTag = (tag: string) => {
    if (tag && !tags.includes(tag)) {
      setTags([...tags, tag]);
    }
    setNewTag("");
  };

  const removeTag = (tag: string) => {
    setTags(tags.filter((t) => t !== tag));
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      await api.updateSite(domain, tags, category);
      onUpdate?.();
    } catch (e) {
      console.error("Failed to save tags:", e);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-4 p-4 bg-gray-50 dark:bg-gray-800 rounded-lg">
      {/* Category Selection */}
      <div>
        <label className="block text-sm font-medium mb-2">Category</label>
        <div className="flex flex-wrap gap-2">
          {PRESET_CATEGORIES.map((cat) => (
            <button
              key={cat}
              onClick={() => setCategory(category === cat ? "" : cat)}
              className={`px-3 py-1 text-sm rounded-full transition ${
                category === cat
                  ? "bg-blue-600 text-white"
                  : "bg-gray-200 dark:bg-gray-700 hover:bg-gray-300 dark:hover:bg-gray-600"
              }`}
            >
              {cat}
            </button>
          ))}
        </div>
      </div>

      {/* Tags */}
      <div>
        <label className="block text-sm font-medium mb-2">Tags</label>
        <div className="flex flex-wrap gap-2 mb-3">
          {tags.map((tag) => (
            <span
              key={tag}
              className="flex items-center gap-1 px-2 py-1 bg-blue-100 dark:bg-blue-900 text-blue-800 dark:text-blue-200 text-sm rounded"
            >
              <Tag className="w-3 h-3" />
              {tag}
              <button
                onClick={() => removeTag(tag)}
                className="hover:text-red-500"
              >
                <X className="w-3 h-3" />
              </button>
            </span>
          ))}
        </div>

        <div className="flex gap-2 mb-3">
          <input
            type="text"
            value={newTag}
            onChange={(e) => setNewTag(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && addTag(newTag)}
            placeholder="Add custom tag..."
            className="flex-1 px-3 py-1.5 text-sm border rounded dark:bg-gray-700 dark:border-gray-600"
          />
          <button
            onClick={() => addTag(newTag)}
            className="px-3 py-1.5 bg-blue-600 text-white rounded hover:bg-blue-700"
          >
            <Plus className="w-4 h-4" />
          </button>
        </div>

        <div className="flex flex-wrap gap-1">
          {PRESET_TAGS.filter((t) => !tags.includes(t)).map((tag) => (
            <button
              key={tag}
              onClick={() => addTag(tag)}
              className="px-2 py-0.5 text-xs bg-gray-200 dark:bg-gray-700 rounded hover:bg-gray-300 dark:hover:bg-gray-600"
            >
              + {tag}
            </button>
          ))}
        </div>
      </div>

      {/* Save Button */}
      <button
        onClick={handleSave}
        disabled={saving}
        className="flex items-center gap-2 px-4 py-2 bg-green-600 text-white rounded hover:bg-green-700 disabled:opacity-50"
      >
        <Check className="w-4 h-4" />
        {saving ? "Saving..." : "Save Changes"}
      </button>
    </div>
  );
}
