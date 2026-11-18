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
      return typeof name === "string" && isFleetQueryKey(name);
    },
  });
}

// Every query key that reflects fleet state, so a single mutation refreshes the
// detail, list, tree, customer-directory and security views together. Missing
// one here leaves that view stale for up to the global refetch interval.
function isFleetQueryKey(name: string): boolean {
  return (
    name.startsWith("instance") || // instance, instance-parents, instance-children, instance-tree, instances-list
    name.startsWith("fleet-") || // fleet-stats, fleet-tree, fleet-tree-roots
    name.startsWith("customer-") || // customer-directory, customer-children
    name.startsWith("security-") || // security-stats, security-list
    name === "inactive-children"
  );
}
