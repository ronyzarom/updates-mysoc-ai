import { describe, expect, it, vi } from "vitest";
import { QueryClient, QueryObserver } from "@tanstack/react-query";
import { refreshFleetQueries } from "./fleet-cache";

describe("fleet deletion cache refresh", () => {
  it("removes the deleted detail and invalidates the actual list, tree, hierarchy and count keys", async () => {
    const client = new QueryClient();
    const keys = [["instances-list", "siemcore"], ["instance-tree"], ["instance-children", "parent"], ["fleet-stats"], ["inactive-children", "parent"]];
    for (const key of keys) client.setQueryData(key, { items: ["deleted"] });
    client.setQueryData(["instance", "deleted"], { id: "deleted" });
    client.setQueryData(["releases"], []);
    await refreshFleetQueries(client, "deleted");
    expect(client.getQueryData(["instance", "deleted"])).toBeNull();
    for (const key of keys) expect(client.getQueryState(key)?.isInvalidated).toBe(true);
    expect(client.getQueryState(["releases"])?.isInvalidated).toBe(false);
    client.clear();
  });
});


it("does not refetch deleted detail or ancestry while observers remain mounted", async () => {
  const client = new QueryClient();
  const detailFetch = vi.fn(async () => ({ id: "deleted" }));
  const parentFetch = vi.fn(async () => []);
  const detailOptions = { queryKey: ["instance", "deleted"], queryFn: detailFetch, initialData: { id: "deleted" }, staleTime: Infinity };
  const parentOptions = { queryKey: ["instance-parents", "deleted"], queryFn: parentFetch, initialData: [], staleTime: Infinity };
  const detail = new QueryObserver(client, detailOptions);
  const parents = new QueryObserver(client, parentOptions);
  const stopDetail = detail.subscribe(() => {});
  const stopParents = parents.subscribe(() => {});
  await refreshFleetQueries(client, "deleted");
  // Model the still-mounted detail component rendering before navigation commits.
  detail.setOptions(detailOptions);
  parents.setOptions(parentOptions);
  await Promise.resolve();
  expect(detailFetch).not.toHaveBeenCalled();
  expect(parentFetch).not.toHaveBeenCalled();
  expect(client.getQueryData(["instance", "deleted"])).toBeNull();
  stopDetail(); stopParents(); client.clear();
});
