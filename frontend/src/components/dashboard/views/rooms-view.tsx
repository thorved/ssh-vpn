"use client";

import {
  Activity,
  Clock3,
  PlugZap,
  Search,
  Terminal,
  Trash2,
  Users,
  Wifi,
} from "lucide-react";
import { useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import type { RoomSnapshot } from "@/types/admin";

import {
  EmptyState,
  FilterButton,
  LoadBar,
  MiniStat,
  RoleBadge,
  StatusBadge,
} from "../dashboard-common";
import type { DashboardViewProps } from "../dashboard-types";
import { earliestDate, formatDate, latestDate } from "../dashboard-utils";

type RoomFilter = "all" | "live" | "idle";

export function RoomsView({
  overview,
  selectedRoom,
  selectedRoomName,
  deleting,
  onSelectRoom,
  onCommandPortChange,
  onDeleteConnection,
  onDeleteRoom,
}: DashboardViewProps) {
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<RoomFilter>("all");

  const rooms = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return overview.rooms
      .filter((room) => {
        if (filter === "live" && room.activeChannels === 0) {
          return false;
        }
        if (filter === "idle" && room.activeChannels > 0) {
          return false;
        }
        if (!normalizedQuery) {
          return true;
        }
        return (
          room.name.toLowerCase().includes(normalizedQuery) ||
          room.connections.some((connection) =>
            connection.remoteAddr.toLowerCase().includes(normalizedQuery),
          ) ||
          room.publishers.some((publisher) =>
            String(publisher.port).includes(normalizedQuery),
          )
        );
      })
      .sort((a, b) => {
        if (a.activeChannels !== b.activeChannels) {
          return b.activeChannels - a.activeChannels;
        }
        if (a.connectionCount !== b.connectionCount) {
          return b.connectionCount - a.connectionCount;
        }
        return a.name.localeCompare(b.name);
      });
  }, [filter, overview.rooms, query]);

  return (
    <section className="grid gap-4 xl:grid-cols-[380px_minmax(0,1fr)]">
      <Card className="overflow-hidden">
        <CardHeader className="border-b border-border">
          <div className="flex items-center justify-between gap-3">
            <CardTitle>Room Inventory</CardTitle>
            <Badge>{rooms.length}</Badge>
          </div>
          <div className="mt-3 grid gap-2">
            <label className="grid h-10 grid-cols-[18px_minmax(0,1fr)] items-center gap-2 rounded-md border border-border bg-background px-3 text-sm">
              <Search className="h-4 w-4 text-muted-foreground" />
              <input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Search room, host, port"
                className="min-w-0 bg-transparent outline-none placeholder:text-muted-foreground"
              />
            </label>
            <div className="grid grid-cols-3 gap-2">
              <FilterButton
                active={filter === "all"}
                onClick={() => setFilter("all")}
              >
                All
              </FilterButton>
              <FilterButton
                active={filter === "live"}
                onClick={() => setFilter("live")}
              >
                Live
              </FilterButton>
              <FilterButton
                active={filter === "idle"}
                onClick={() => setFilter("idle")}
              >
                Idle
              </FilterButton>
            </div>
          </div>
        </CardHeader>
        <CardContent className="max-h-[760px] overflow-y-auto p-2">
          {rooms.length > 0 ? (
            <div className="grid gap-2">
              {rooms.map((room) => (
                <RoomListItem
                  key={room.name}
                  room={room}
                  active={selectedRoomName === room.name}
                  adminUser={overview.adminUser}
                  deleting={deleting === room.name}
                  onSelect={() => onSelectRoom(room.name)}
                  onDelete={() => onDeleteRoom(room)}
                />
              ))}
            </div>
          ) : (
            <EmptyState label="No matching rooms" />
          )}
        </CardContent>
      </Card>

      <RoomInspector
        room={selectedRoom}
        adminUser={overview.adminUser}
        deleting={deleting}
        onCommandPortChange={onCommandPortChange}
        onDeleteConnection={onDeleteConnection}
        onDeleteRoom={onDeleteRoom}
      />
    </section>
  );
}

function RoomListItem({
  room,
  active,
  adminUser,
  deleting,
  onSelect,
  onDelete,
}: {
  room: RoomSnapshot;
  active: boolean;
  adminUser: string;
  deleting: boolean;
  onSelect: () => void;
  onDelete: () => void;
}) {
  return (
    <div
      className={cn(
        "rounded-md border border-border bg-card p-3 transition-colors",
        active && "border-primary/50 bg-primary/5",
      )}
    >
      <button
        type="button"
        onClick={onSelect}
        className="grid w-full gap-2 text-left"
      >
        <div className="flex min-w-0 items-center justify-between gap-3">
          <span className="truncate font-medium">{room.name}</span>
          <StatusBadge active={room.activeChannels > 0} />
        </div>
        <div className="grid grid-cols-3 gap-2 text-xs text-muted-foreground">
          <span>{room.connectionCount} users</span>
          <span>{room.publisherCount} forwards</span>
          <span>{room.activeChannels} active</span>
        </div>
        <LoadBar
          value={room.activeChannels}
          max={Math.max(room.connectionCount, 1)}
        />
      </button>
      <div className="mt-3 flex justify-end gap-2">
        <Button type="button" variant="outline" size="sm" onClick={onSelect}>
          Inspect
        </Button>
        <Button
          type="button"
          variant="destructive"
          size="icon"
          title="Delete room"
          disabled={room.name === adminUser || deleting}
          onClick={onDelete}
        >
          <Trash2 className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}

function RoomInspector({
  room,
  adminUser,
  deleting,
  onCommandPortChange,
  onDeleteConnection,
  onDeleteRoom,
}: {
  room: RoomSnapshot | null;
  adminUser: string;
  deleting: string;
  onCommandPortChange: (port: number) => void;
  onDeleteConnection: DashboardViewProps["onDeleteConnection"];
  onDeleteRoom: (room: RoomSnapshot) => void;
}) {
  if (!room) {
    return (
      <Card>
        <CardContent className="flex min-h-[560px] items-center justify-center text-sm text-muted-foreground">
          Select a room
        </CardContent>
      </Card>
    );
  }

  const firstConnectedAt = earliestDate(
    room.connections.map((connection) => connection.connectedAt),
  );
  const newestConnection = latestDate(
    room.connections.map((connection) => connection.connectedAt),
  );

  return (
    <Card className="overflow-hidden">
      <CardHeader className="border-b border-border">
        <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-start">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <CardTitle className="truncate text-xl">{room.name}</CardTitle>
              <StatusBadge active={room.activeChannels > 0} />
            </div>
            <p className="mt-1 text-sm text-muted-foreground">
              {room.connectionCount} connection
              {room.connectionCount === 1 ? "" : "s"} across{" "}
              {room.publisherCount} forward
              {room.publisherCount === 1 ? "" : "s"}
            </p>
          </div>
          <Button
            type="button"
            variant="destructive"
            disabled={room.name === adminUser || deleting === room.name}
            onClick={() => onDeleteRoom(room)}
          >
            <Trash2 className="h-4 w-4" />
            Remove
          </Button>
        </div>
      </CardHeader>
      <CardContent className="grid gap-4 pt-4">
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <MiniStat label="Users" value={room.connectionCount} icon={Users} />
          <MiniStat
            label="Forwards"
            value={room.publisherCount}
            icon={PlugZap}
          />
          <MiniStat
            label="Active"
            value={room.activeChannels}
            icon={Activity}
          />
          <MiniStat
            label="Ports"
            value={
              new Set(room.publishers.map((publisher) => publisher.port)).size
            }
            icon={Terminal}
          />
        </div>

        <div className="grid gap-3 lg:grid-cols-2">
          <InfoStrip
            icon={Clock3}
            label="First seen"
            value={
              firstConnectedAt ? formatDate(firstConnectedAt) : "not connected"
            }
          />
          <InfoStrip
            icon={Wifi}
            label="Last joined"
            value={
              newestConnection ? formatDate(newestConnection) : "not connected"
            }
          />
        </div>

        <section className="rounded-md border border-border">
          <div className="border-b border-border px-3 py-2 text-sm font-medium">
            Users In This Room
          </div>
          {room.connections.length > 0 ? (
            <div className="divide-y divide-border">
              {room.connections.map((connection) => (
                <div
                  key={connection.id}
                  className="grid gap-3 px-3 py-3 md:grid-cols-[minmax(0,1fr)_auto_auto] md:items-center"
                >
                  <div className="min-w-0">
                    <p className="truncate font-mono text-xs">
                      {connection.remoteAddr || "unknown"}
                    </p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      recv {connection.receiverActiveChannels} / pub{" "}
                      {connection.publisherActiveChannels}
                    </p>
                  </div>
                  <RoleBadge role={connection.role} />
                  <Button
                    type="button"
                    variant="destructive"
                    size="icon"
                    title="Disconnect user"
                    disabled={
                      room.name === adminUser ||
                      deleting === `conn-${connection.id}`
                    }
                    onClick={() => onDeleteConnection(connection)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              ))}
            </div>
          ) : (
            <EmptyState label="No connected users" />
          )}
        </section>

        <section className="rounded-md border border-border">
          <div className="border-b border-border px-3 py-2 text-sm font-medium">
            Port Lanes
          </div>
          {room.publishers.length > 0 ? (
            <div className="divide-y divide-border">
              {room.publishers.map((publisher) => (
                <button
                  type="button"
                  key={`${publisher.port}-${publisher.bindHost}`}
                  onClick={() => onCommandPortChange(publisher.port)}
                  className="grid w-full gap-1 px-3 py-3 text-left hover:bg-muted/70"
                >
                  <div className="flex items-center justify-between gap-2">
                    <span className="font-mono text-sm">{publisher.port}</span>
                    <Badge>{publisher.bindHost || "localhost"}</Badge>
                  </div>
                  <span className="truncate text-xs text-muted-foreground">
                    {publisher.remoteAddr || "publisher connected"}
                  </span>
                </button>
              ))}
            </div>
          ) : (
            <EmptyState label="No published forwards" />
          )}
        </section>
      </CardContent>
    </Card>
  );
}

function InfoStrip({
  icon: Icon,
  label,
  value,
}: {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value: string;
}) {
  return (
    <div className="flex min-w-0 items-center gap-3 rounded-md border border-border bg-muted/35 p-3">
      <Icon className="h-4 w-4 shrink-0 text-muted-foreground" />
      <div className="min-w-0">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className="truncate text-sm font-medium">{value}</p>
      </div>
    </div>
  );
}
