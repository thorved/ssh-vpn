"use client";

import {
  Activity,
  DoorOpen,
  Moon,
  Radio,
  RefreshCw,
  Server,
  ShieldCheck,
  Sun,
  Terminal,
  Users,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { ConnectionSnapshot, Overview, RoomSnapshot } from "@/types/admin";
import type { DashboardView } from "./dashboard-types";
import { fallbackPort, formatTime, normalizeOverview } from "./dashboard-utils";
import { CommandsView } from "./views/commands-view";
import { ConnectionsView } from "./views/connections-view";
import { OverviewView } from "./views/overview-view";
import { RoomsView } from "./views/rooms-view";

type Theme = "light" | "dark";

const navItems: Array<{
  view: DashboardView;
  href: string;
  label: string;
  icon: typeof Activity;
}> = [
  { view: "overview", href: "/", label: "Overview", icon: Activity },
  { view: "rooms", href: "/rooms", label: "Rooms", icon: DoorOpen },
  {
    view: "connections",
    href: "/connections",
    label: "Connections",
    icon: Users,
  },
  { view: "commands", href: "/commands", label: "Commands", icon: Terminal },
];

export function DashboardApp({ view }: { view: DashboardView }) {
  const [activeView, setActiveView] = useState<DashboardView>(view);
  const [overview, setOverview] = useState<Overview | null>(null);
  const [selectedRoomName, setSelectedRoomName] = useState("");
  const [commandPort, setCommandPort] = useState(fallbackPort);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [theme, setTheme] = useState<Theme>("dark");
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");
  const [copied, setCopied] = useState("");
  const [deleting, setDeleting] = useState("");
  const [lastUpdated, setLastUpdated] = useState<Date | null>(null);

  const loadOverview = useCallback(async (quiet = false) => {
    if (quiet) {
      setRefreshing(true);
    } else {
      setLoading(true);
    }
    setError("");
    try {
      const response = await fetch("/api/admin/overview", {
        cache: "no-store",
      });
      if (!response.ok) {
        throw new Error(`Overview failed with ${response.status}`);
      }
      const data = normalizeOverview((await response.json()) as Overview);
      setOverview(data);
      setLastUpdated(new Date());
      setSelectedRoomName((current) => {
        if (current && data.rooms.some((room) => room.name === current)) {
          return current;
        }
        return data.rooms[0]?.name ?? "";
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Dashboard unavailable");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    const storedTheme = window.localStorage.getItem("ssh-vpn-theme");
    if (storedTheme === "light" || storedTheme === "dark") {
      setTheme(storedTheme);
    }
  }, []);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    document.documentElement.classList.toggle("dark", theme === "dark");
    window.localStorage.setItem("ssh-vpn-theme", theme);
  }, [theme]);

  useEffect(() => {
    void loadOverview();
  }, [loadOverview]);

  useEffect(() => {
    const onPopState = () => {
      setActiveView(viewFromPath(window.location.pathname));
    };
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);

  useEffect(() => {
    if (!autoRefresh) {
      return;
    }
    const timer = window.setInterval(() => {
      void loadOverview(true);
    }, 4000);
    return () => window.clearInterval(timer);
  }, [autoRefresh, loadOverview]);

  const selectedRoom = useMemo(() => {
    return (
      overview?.rooms.find((room) => room.name === selectedRoomName) ?? null
    );
  }, [overview, selectedRoomName]);

  useEffect(() => {
    if (!selectedRoom) {
      setCommandPort(fallbackPort);
      return;
    }
    setCommandPort(selectedRoom.publishers[0]?.port ?? fallbackPort);
  }, [selectedRoom]);

  const copyText = async (key: string, value: string) => {
    if (!navigator.clipboard) {
      window.prompt("Copy", value);
      return;
    }
    await navigator.clipboard.writeText(value);
    setCopied(key);
    window.setTimeout(() => setCopied(""), 1500);
  };

  const deleteRoom = async (room: RoomSnapshot) => {
    if (!overview || room.name === overview.adminUser) {
      return;
    }
    const ok = window.confirm(`Remove room "${room.name}"?`);
    if (!ok) {
      return;
    }
    setDeleting(room.name);
    setError("");
    try {
      const response = await fetch(
        `/api/admin/rooms/${encodeURIComponent(room.name)}`,
        { method: "DELETE" },
      );
      if (!response.ok) {
        const body = (await response.json().catch(() => null)) as {
          error?: string;
        } | null;
        throw new Error(body?.error ?? `Delete failed with ${response.status}`);
      }
      await loadOverview(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Delete failed");
    } finally {
      setDeleting("");
    }
  };

  const deleteConnection = async (connection: ConnectionSnapshot) => {
    if (!overview || connection.room === overview.adminUser) {
      return;
    }
    const ok = window.confirm(
      `Disconnect ${connection.remoteAddr || `connection ${connection.id}`} from ${connection.room}?`,
    );
    if (!ok) {
      return;
    }
    setDeleting(`conn-${connection.id}`);
    setError("");
    try {
      const response = await fetch(`/api/admin/connections/${connection.id}`, {
        method: "DELETE",
      });
      if (!response.ok) {
        const body = (await response.json().catch(() => null)) as {
          error?: string;
        } | null;
        throw new Error(
          body?.error ?? `Disconnect failed with ${response.status}`,
        );
      }
      await loadOverview(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Disconnect failed");
    } finally {
      setDeleting("");
    }
  };

  const navigate = (nextView: DashboardView, href: string) => {
    setActiveView(nextView);
    if (window.location.pathname !== href) {
      window.history.pushState(null, "", href);
    }
  };

  const viewProps = overview
    ? {
        overview,
        selectedRoom,
        selectedRoomName,
        commandPort,
        copied,
        deleting,
        onSelectRoom: setSelectedRoomName,
        onCommandPortChange: setCommandPort,
        onCopy: copyText,
        onDeleteRoom: deleteRoom,
        onDeleteConnection: deleteConnection,
      }
    : null;

  return (
    <main className="min-h-screen bg-background text-foreground">
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-5 px-4 py-5 sm:px-6 lg:px-8">
        <header className="grid gap-4 border-b border-border pb-5 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2 text-sm font-medium text-muted-foreground">
              <Server className="h-4 w-4 text-emerald-500" />
              <span className="truncate">
                {overview?.publicDomain ?? "localhost"}
              </span>
              <Badge className="bg-emerald-500/12 text-emerald-700 dark:text-emerald-300">
                port {overview?.publicSshPort ?? "2222"}
              </Badge>
            </div>
            <div className="mt-2 flex flex-wrap items-end gap-3">
              <h1 className="text-2xl font-semibold leading-8">
                SSH VPN Dashboard
              </h1>
              <span className="pb-1 text-xs text-muted-foreground">
                {lastUpdated ? `updated ${formatTime(lastUpdated)}` : "waiting"}
              </span>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <Badge className="gap-1">
              <ShieldCheck className="h-3.5 w-3.5" />
              {overview?.adminUser ?? "root"}
            </Badge>
            <Button
              type="button"
              variant={autoRefresh ? "default" : "outline"}
              onClick={() => setAutoRefresh((value) => !value)}
            >
              <Radio className="h-4 w-4" />
              Auto
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
              title="Toggle theme"
            >
              {theme === "dark" ? (
                <Sun className="h-4 w-4" />
              ) : (
                <Moon className="h-4 w-4" />
              )}
              {theme === "dark" ? "Light" : "Dark"}
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => void loadOverview(true)}
              disabled={refreshing}
            >
              <RefreshCw
                className={cn("h-4 w-4", refreshing && "animate-spin")}
              />
              Refresh
            </Button>
          </div>
        </header>

        <nav className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
          {navItems.map((item) => (
            <NavItem
              key={item.view}
              active={item.view === activeView}
              href={item.href}
              icon={item.icon}
              label={item.label}
              onNavigate={() => navigate(item.view, item.href)}
            />
          ))}
        </nav>

        {error ? (
          <div className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {error}
          </div>
        ) : null}

        {loading && !overview ? (
          <div className="flex h-80 items-center justify-center gap-3 text-sm text-muted-foreground">
            <RefreshCw className="h-4 w-4 animate-spin" />
            Loading dashboard
          </div>
        ) : null}

        {viewProps && activeView === "overview" ? (
          <OverviewView {...viewProps} />
        ) : null}
        {viewProps && activeView === "rooms" ? (
          <RoomsView {...viewProps} />
        ) : null}
        {viewProps && activeView === "connections" ? (
          <ConnectionsView {...viewProps} />
        ) : null}
        {viewProps && activeView === "commands" ? (
          <CommandsView {...viewProps} />
        ) : null}
      </div>
    </main>
  );
}

function NavItem({
  active,
  href,
  icon: Icon,
  label,
  onNavigate,
}: {
  active: boolean;
  href: string;
  icon: typeof Activity;
  label: string;
  onNavigate: () => void;
}) {
  return (
    <a
      href={href}
      onClick={(event) => {
        event.preventDefault();
        onNavigate();
      }}
      className={cn(
        "flex h-11 items-center gap-2 rounded-md border border-border px-3 text-sm font-medium transition-colors",
        active
          ? "bg-primary text-primary-foreground"
          : "bg-card text-muted-foreground hover:bg-muted hover:text-foreground",
      )}
    >
      <Icon className="h-4 w-4" />
      {label}
    </a>
  );
}

function viewFromPath(pathname: string): DashboardView {
  if (pathname.startsWith("/rooms")) {
    return "rooms";
  }
  if (pathname.startsWith("/connections")) {
    return "connections";
  }
  if (pathname.startsWith("/commands")) {
    return "commands";
  }
  return "overview";
}
