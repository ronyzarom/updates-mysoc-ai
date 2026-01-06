"use client";

import { usePathname } from "next/navigation";
import { Sidebar } from "./Sidebar";
import { useAuth } from "@/lib/auth-context";

// Pages that don't show the sidebar
const noSidebarPaths = ["/login"];

export function LayoutWrapper({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const { isAuthenticated, isLoading } = useAuth();

  const showSidebar = isAuthenticated && !noSidebarPaths.includes(pathname);

  // Show loading state while checking auth
  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-slate-950">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-cyan-500"></div>
      </div>
    );
  }

  if (!showSidebar) {
    return <>{children}</>;
  }

  return (
    <div className="flex min-h-screen">
      <Sidebar />
      <main className="flex-1 p-8 ml-64 bg-slate-950">
        {children}
      </main>
    </div>
  );
}
