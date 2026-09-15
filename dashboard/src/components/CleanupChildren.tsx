"use client";

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";

export function CleanupChildren({ parentId }: { parentId: string }) {
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState<string[]>([]);
  const [result, setResult] = useState("");
  const client = useQueryClient();
  const candidates = useQuery({
    queryKey: ["inactive-children", parentId],
    queryFn: () => api.getInactiveChildren(parentId),
    enabled: open,
    staleTime: 0,
    refetchOnWindowFocus: false,
  });
  useEffect(() => {
    if (candidates.data) setSelected(candidates.data.items.map((item) => item.id));
  }, [candidates.data]);
  const cleanup = useMutation({
    mutationFn: () => api.cleanupChildren(parentId, selected),
    onSuccess: ({ removed }) => {
      setResult(`${removed} entries removed.${removed < selected.length ? " Entries that are no longer eligible were skipped." : ""}`);
      setOpen(false);
      client.invalidateQueries({ queryKey: ["instance-children"] });
      client.invalidateQueries({ queryKey: ["instances"] });
      client.invalidateQueries({ queryKey: ["inactive-children", parentId] });
    },
  });
  const items = candidates.data?.items ?? [];
  return <>
    <button type="button" className="text-sm text-red-400 hover:text-red-300" onClick={() => { cleanup.reset(); setResult(""); setOpen(true); }}>Delete inactive entries</button>
    {result && <p role="status" className="text-sm text-slate-300 mt-2">{result}</p>}
    {open && <div className="fixed inset-0 z-50 bg-black/70 flex items-center justify-center p-4">
      <div role="dialog" aria-modal="true" aria-labelledby="cleanup-title" className="w-full max-w-2xl max-h-[calc(100dvh-2rem)] flex flex-col rounded-xl border border-slate-700 bg-slate-900 shadow-xl">
        <div className="p-5 border-b border-slate-700">
          <h2 id="cleanup-title" className="text-lg font-semibold text-white">Delete inactive hierarchy entries</h2>
          <p className="text-sm text-slate-400 mt-2">Review offline entries with no heartbeat for at least one hour. Hosts with update holds and nodes with children are excluded. Offline does not necessarily mean the host has been removed.</p>
          <p className="text-sm text-slate-400 mt-2">This removes entries from the hierarchy and retains their history. It does not uninstall software. A genuine new heartbeat can make a host visible again.</p>
        </div>
        <div className="p-5 overflow-y-auto min-h-0">
          {candidates.isFetching ? <p className="text-slate-400">Checking inactive entries…</p> : candidates.isError ? <p role="alert" className="text-red-400">{candidates.error.message}</p> : items.length === 0 ? <p className="text-slate-400">No eligible inactive entries.</p> : <>
            <label className="flex gap-2 text-white mb-4"><input type="checkbox" checked={selected.length === items.length} onChange={(e) => setSelected(e.target.checked ? items.map((item) => item.id) : [])} /> Select all ({items.length})</label>
            {items.length === 1000 && <p className="text-amber-400 text-sm mb-3">Showing up to 1,000 entries. Repeat cleanup if more remain.</p>}
            <ul className="space-y-3">{items.map((item) => <li key={item.id}><label className="flex items-start gap-3 text-sm text-slate-200"><input type="checkbox" className="mt-1" checked={selected.includes(item.id)} onChange={(e) => setSelected(e.target.checked ? [...selected, item.id] : selected.filter((id) => id !== item.id))} /><span className="break-all">{item.instance_id}<span className="block text-xs text-slate-500">Last heartbeat: {item.last_heartbeat ? new Date(item.last_heartbeat).toLocaleString() : "Never"}</span></span></label></li>)}</ul>
          </>}
          {cleanup.isError && <p role="alert" className="text-red-400 mt-3">{cleanup.error.message}</p>}
        </div>
        <div className="p-5 border-t border-slate-700 flex justify-end gap-3">
          <button type="button" className="btn btn-secondary" disabled={cleanup.isPending} onClick={() => setOpen(false)}>Cancel</button>
          <button type="button" className="btn bg-red-600 text-white" disabled={cleanup.isPending || candidates.isFetching || candidates.isError || selected.length === 0} onClick={() => cleanup.mutate()}>{cleanup.isPending ? "Removing…" : `Delete selected (${selected.length})`}</button>
        </div>
      </div>
    </div>}
  </>;
}
