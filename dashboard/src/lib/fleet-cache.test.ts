import { describe, expect, it } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { refreshFleetQueries } from "./fleet-cache";

describe("fleet deletion cache refresh", () => {
  it("removes the deleted detail and invalidates the actual list, tree, hierarchy and count keys", async () => {
    const client = new QueryClient();
    const keys = [["instances-list", "siemcore"], ["instance-tree"], ["instance-children", "parent"], ["fleet-stats"], ["inactive-children", "parent"]];
    for (const key of keys) client.setQueryData(key, { items: ["deleted"] });
    client.setQueryData(["instance", "deleted"], { id: "deleted" });
    client.setQueryData(["releases"], []);
    await refreshFleetQueries(client, "deleted");
    expect(client.getQueryData(["instance", "deleted"])).toBeUndefined();
    for (const key of keys) expect(client.getQueryState(key)?.isInvalidated).toBe(true);
    expect(client.getQueryState(["releases"])?.isInvalidated).toBe(false);
    client.clear();
  });
});
