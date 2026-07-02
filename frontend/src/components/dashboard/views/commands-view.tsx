"use client";

import { Clipboard, PlugZap, Search, Terminal } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import type { PublisherSnapshot } from "@/types/admin";

import { CommandLine, EmptyState, MetricCard } from "../dashboard-common";
import type { DashboardViewProps } from "../dashboard-types";
import {
  clampPort,
  connectCommand,
  dashboardAccessCommand,
  publishCommand,
} from "../dashboard-utils";

export function CommandsView({
  overview,
  selectedRoom,
  commandPort,
  copied,
  onSelectRoom,
  onCommandPortChange,
  onCopy,
}: DashboardViewProps) {
  const [roomInput, setRoomInput] = useState(selectedRoom?.name ?? "roomname");
  const [query, setQuery] = useState("");

  useEffect(() => {
    if (selectedRoom?.name) {
      setRoomInput(selectedRoom.name);
    }
  }, [selectedRoom?.name]);

  const publisherRows = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return overview.rooms
      .flatMap((room) =>
        room.publishers.map((publisher) => ({
          room: room.name,
          publisher,
        })),
      )
      .filter(({ room, publisher }) => {
        if (!normalizedQuery) {
          return true;
        }
        return (
          room.toLowerCase().includes(normalizedQuery) ||
          String(publisher.port).includes(normalizedQuery) ||
          publisher.remoteAddr.toLowerCase().includes(normalizedQuery)
        );
      })
      .sort((a, b) => {
        if (a.publisher.port !== b.publisher.port) {
          return a.publisher.port - b.publisher.port;
        }
        return a.room.localeCompare(b.room);
      });
  }, [overview.rooms, query]);

  const safeRoom = roomInput.trim() || "roomname";
  const dashboard = dashboardAccessCommand(overview);
  const publish = publishCommand(overview, safeRoom, commandPort);
  const connect = connectCommand(overview, safeRoom, commandPort);

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_420px]">
      <Card className="overflow-hidden">
        <CardHeader className="border-b border-border">
          <div className="flex items-center justify-between gap-3">
            <CardTitle>Command Builder</CardTitle>
            <Terminal className="h-4 w-4 text-muted-foreground" />
          </div>
        </CardHeader>
        <CardContent className="grid gap-4 pt-4">
          <div className="grid gap-3 md:grid-cols-2">
            <label className="grid gap-1">
              <span className="text-xs font-medium text-muted-foreground">
                Room
              </span>
              <input
                value={roomInput}
                onChange={(event) => setRoomInput(event.target.value)}
                className="h-10 rounded-md border border-border bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
              />
            </label>
            <label className="grid gap-1">
              <span className="text-xs font-medium text-muted-foreground">
                Port
              </span>
              <input
                type="number"
                min={1}
                max={65535}
                value={commandPort}
                onChange={(event) =>
                  onCommandPortChange(clampPort(Number(event.target.value)))
                }
                className="h-10 rounded-md border border-border bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring"
              />
            </label>
          </div>

          <CommandLine
            label="Dashboard"
            value={dashboard}
            copied={copied === "commands-dashboard"}
            onCopy={() => onCopy("commands-dashboard", dashboard)}
          />
          <CommandLine
            label="Publish"
            value={publish}
            copied={copied === "commands-publish"}
            onCopy={() => onCopy("commands-publish", publish)}
          />
          <CommandLine
            label="Connect"
            value={connect}
            copied={copied === "commands-connect"}
            onCopy={() => onCopy("commands-connect", connect)}
          />
        </CardContent>
      </Card>

      <div className="grid gap-4">
        <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-1">
          <MetricCard
            icon={PlugZap}
            label="Publishers"
            value={overview.totals.publishers}
            detail={`${publisherRows.length} matching`}
            tone="amber"
          />
          <MetricCard
            icon={Clipboard}
            label="Command room"
            value={commandPort}
            detail={safeRoom}
            tone="blue"
          />
        </section>

        <Card className="overflow-hidden">
          <CardHeader className="border-b border-border">
            <CardTitle>Publisher Inventory</CardTitle>
            <label className="mt-3 grid h-10 grid-cols-[18px_minmax(0,1fr)] items-center gap-2 rounded-md border border-border bg-background px-3 text-sm">
              <Search className="h-4 w-4 text-muted-foreground" />
              <input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Search room, port, host"
                className="min-w-0 bg-transparent outline-none placeholder:text-muted-foreground"
              />
            </label>
          </CardHeader>
          <CardContent className="max-h-[520px] overflow-y-auto p-0">
            {publisherRows.length > 0 ? (
              <div className="divide-y divide-border">
                {publisherRows.map(({ room, publisher }) => (
                  <PublisherButton
                    key={`${room}-${publisher.port}-${publisher.bindHost}`}
                    room={room}
                    publisher={publisher}
                    onUse={() => {
                      setRoomInput(room);
                      onSelectRoom(room);
                      onCommandPortChange(publisher.port);
                    }}
                  />
                ))}
              </div>
            ) : (
              <EmptyState label="No active publishers" />
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function PublisherButton({
  room,
  publisher,
  onUse,
}: {
  room: string;
  publisher: PublisherSnapshot;
  onUse: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onUse}
      className="grid w-full gap-2 px-3 py-3 text-left hover:bg-muted/70"
    >
      <div className="flex items-center justify-between gap-2">
        <span className="truncate font-medium">{room}</span>
        <Badge>{publisher.port}</Badge>
      </div>
      <div className="grid gap-1 text-xs text-muted-foreground">
        <span>{publisher.bindHost || "localhost"}</span>
        <span className="truncate">{publisher.remoteAddr || "connected"}</span>
      </div>
    </button>
  );
}
