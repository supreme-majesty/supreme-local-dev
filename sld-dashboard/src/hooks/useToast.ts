import { useAppStore } from "@/stores/useAppStore";

type ToastType = "success" | "error" | "info" | "warning";

export function useToast() {
  const { toasts, addToast, removeToast } = useAppStore();

  const toast = (type: ToastType, title: string, description?: string) => {
    addToast({ type, title, description });
  };

  return {
    toasts,
    toast,
    success: (title: string, description?: string) =>
      toast("success", title, description),
    error: (title: string, description?: string) =>
      toast("error", title, description),
    info: (title: string, description?: string) =>
      toast("info", title, description),
    warning: (title: string, description?: string) =>
      toast("warning", title, description),
    dismiss: removeToast,
  };
}
