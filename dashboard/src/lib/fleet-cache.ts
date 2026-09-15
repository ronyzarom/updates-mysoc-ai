import type { QueryClient } from "@tanstack/react-query";

export function refreshFleetQueries(client: QueryClient, deletedId?: string) {
  if (deletedId) client.removeQueries({ queryKey: ["instance", deletedId], exact: true });
  return client.invalidateQueries({
    predicate: ({ queryKey }) => {
      const name = queryKey[0];
      return typeof name === "string" && (name.startsWith("instance") || name === "fleet-stats" || name === "inactive-children");
    },
  });
}
