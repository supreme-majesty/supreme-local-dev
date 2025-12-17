import { QueryClient } from "@tanstack/react-query";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 1000 * 10, // Data matches "fresh" for 10 seconds
      gcTime: 1000 * 60 * 5, // Garbage collect unused data after 5 minutes
      retry: 1, // Retry failed requests once
      refetchOnWindowFocus: true, // Refetch when window gains focus
    },
  },
});
