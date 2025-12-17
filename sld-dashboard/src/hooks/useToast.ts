import { useAppStore } from "@/stores/useAppStore";

type ToastType = "success" | "error" | "info" | "warning";

interface ToastOptions {
  title: string;
  description?: string;
  variant?: "default" | "destructive" | "success";
}

export function useToast() {
  const { toasts, addToast, removeToast } = useAppStore();

  const toast = (
    args: ToastOptions | ToastType,
    title?: string,
    description?: string
  ) => {
    if (typeof args === "string") {
      addToast({ type: args, title: title!, description });
    } else {
      // Map variant to type
      let type: ToastType = "info";
      if (args.variant === "destructive") type = "error";
      if (args.variant === "success") type = "success";

      addToast({ type, title: args.title, description: args.description });
    }
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
