"use client";

import { usePathname, useRouter } from "next/navigation";
import { useEffect } from "react";
import { Sidebar } from "./Sidebar";
import { useAuth } from "@/lib/auth-context";

// Pages that don't require authentication
const publicPaths = ["/login"];

export function LayoutWrapper({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { isAuthenticated, isLoading } = useAuth();

  const isPublicPage = publicPaths.includes(pathname);

  // Redirect to login if not authenticated and trying to access protected page
  useEffect(() => {
    if (!isLoading && !isAuthenticated && !isPublicPage) {
      router.replace("/login");
    }
  }, [isLoading, isAuthenticated, isPublicPage, router]);

  // Redirect to dashboard if authenticated and trying to access login
  useEffect(() => {
    if (!isLoading && isAuthenticated && isPublicPage) {
      router.replace("/");
    }
  }, [isLoading, isAuthenticated, isPublicPage, router]);

  // Show loading state while checking auth
  if (isLoading) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-slate-950">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-cyan-500"></div>
      </div>
    );
  }

  // If not authenticated and not on public page, show loading (redirect is happening)
  if (!isAuthenticated && !isPublicPage) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-slate-950">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-cyan-500"></div>
      </div>
    );
  }

  // Public pages (login) - no sidebar
  if (isPublicPage) {
    return <>{children}</>;
  }

  // Authenticated pages with sidebar
  return (
    <div className="flex min-h-screen">
      <Sidebar />
      <main className="flex-1 p-8 ml-64 bg-slate-950">
        {children}
      </main>
    </div>
  );
}
