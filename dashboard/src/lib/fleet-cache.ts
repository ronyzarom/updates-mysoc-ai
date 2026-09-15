import type { QueryClient } from "@tanstack/react-query";

export async function refreshFleetQueries(client: QueryClient, deletedId?: string) {
  if (deletedId) {
    await client.cancelQueries({ predicate: ({ queryKey }) => queryKey[1] === deletedId && (queryKey[0] === "instance" || queryKey[0] === "instance-parents") });
    // Keep a tombstone while the detail observer is mounted. Removing an active
    // query recreates it on the next render and fetches the deleted URL again.
    client.setQueryData(["instance", deletedId], null);
    client.setQueryData(["instance-parents", deletedId], []);
  }
  return client.invalidateQueries({
    predicate: ({ queryKey }) => {
      if (deletedId && queryKey[1] === deletedId) return false;
      const name = queryKey[0];
      return typeof name === "string" && (name.startsWith("instance") || name === "fleet-stats" || name === "inactive-children");
    },
  });
}
