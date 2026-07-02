"use client";

import { Activity, Clock3, Network, Search, Trash2, Users } from "lucide-react";
import { useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

import {
  EmptyState,
  MetricCard,
  RoleBadge,
  StatusBadge,
} from "../dashboard-common";
import type { DashboardViewProps } from "../dashboard-types";
import { connectionAge, formatDate, summarizeRooms } from "../dashboard-utils";

export function ConnectionsView({
  deleting,
  overview,
  onDeleteConnection,
  onSelectRoom,
}: DashboardViewProps) {
  const [query, setQuery] = useState("");
  const insights = summarizeRooms(overview.rooms);

  const rows = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    return overview.rooms
      .flatMap((room) =>
        room.connections.map((connection) => ({
          room,
          connection,
        })),
      )
      .filter(({ room, connection }) => {
        if (!normalizedQuery) {
          return true;
        }
        return (
          room.name.toLowerCase().includes(normalizedQuery) ||
          connection.remoteAddr.toLowerCase().includes(normalizedQuery)
        );
      })
      .sort((a, b) => {
        if (a.connection.activeChannels !== b.connection.activeChannels) {
          return b.connection.activeChannels - a.connection.activeChannels;
        }
        return b.connection.connectedAt.localeCompare(a.connection.connectedAt);
      });
  }, [overview.rooms, query]);

  return (
    <div className="grid gap-4">
      <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          icon={Users}
          label="Sessions"
          value={overview.totals.connections}
          detail={`${insights.uniqueHosts} unique host${insights.uniqueHosts === 1 ? "" : "s"}`}
          tone="blue"
        />
        <MetricCard
          icon={Activity}
          label="Active channels"
          value={overview.totals.activeChannels}
          detail={
            insights.busiestRoom ? `top: ${insights.busiestRoom}` : "no traffic"
          }
          tone="violet"
        />
        <MetricCard
          icon={Network}
          label="Rooms online"
          value={overview.totals.rooms}
          detail={`${insights.liveRooms} live rooms`}
          tone="emerald"
        />
        <MetricCard
          icon={Clock3}
          label="Idle rooms"
          value={insights.idleRooms}
          detail="waiting for traffic"
          tone="amber"
        />
      </section>

      <Card className="overflow-hidden">
        <CardHeader className="border-b border-border">
          <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_360px] lg:items-center">
            <CardTitle>Connected Users</CardTitle>
            <label className="grid h-10 grid-cols-[18px_minmax(0,1fr)] items-center gap-2 rounded-md border border-border bg-background px-3 text-sm">
              <Search className="h-4 w-4 text-muted-foreground" />
              <input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Search room or remote address"
                className="min-w-0 bg-transparent outline-none placeholder:text-muted-foreground"
              />
            </label>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          {rows.length > 0 ? (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[1080px] text-sm">
                <thead className="bg-muted text-left text-xs font-medium uppercase text-muted-foreground">
                  <tr>
                    <th className="px-4 py-3">Room</th>
                    <th className="px-4 py-3">Remote</th>
                    <th className="px-4 py-3">Role</th>
                    <th className="px-4 py-3">Ports</th>
                    <th className="px-4 py-3">Connected</th>
                    <th className="px-4 py-3">Age</th>
                    <th className="px-4 py-3">Traffic</th>
                    <th className="px-4 py-3">Channels</th>
                    <th className="px-4 py-3 text-right">Action</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map(({ room, connection }) => (
                    <tr
                      key={`${room.name}-${connection.id}`}
                      className="border-t border-border"
                    >
                      <td className="px-4 py-3">
                        <button
                          type="button"
                          className="font-medium text-foreground hover:text-primary"
                          onClick={() => onSelectRoom(room.name)}
                        >
                          {room.name}
                        </button>
                      </td>
                      <td className="px-4 py-3 font-mono text-xs text-muted-foreground">
                        {connection.remoteAddr || "unknown"}
                      </td>
                      <td className="px-4 py-3">
                        <RoleBadge role={connection.role} />
                      </td>
                      <td className="px-4 py-3">
                        {connection.publishedPorts.length > 0 ? (
                          <div className="flex flex-wrap gap-1">
                            {connection.publishedPorts.map((port) => (
                              <Badge key={port}>{port}</Badge>
                            ))}
                          </div>
                        ) : (
                          <span className="text-muted-foreground">none</span>
                        )}
                      </td>
                      <td className="px-4 py-3 text-muted-foreground">
                        {formatDate(connection.connectedAt)}
                      </td>
                      <td className="px-4 py-3">
                        <Badge>{connectionAge(connection.connectedAt)}</Badge>
                      </td>
                      <td className="px-4 py-3">
                        <StatusBadge active={connection.activeChannels > 0} />
                      </td>
                      <td className="px-4 py-3 text-xs text-muted-foreground">
                        recv {connection.receiverActiveChannels} / pub{" "}
                        {connection.publisherActiveChannels}
                      </td>
                      <td className="px-4 py-3 text-right">
                        <Button
                          type="button"
                          variant="destructive"
                          size="icon"
                          title="Disconnect user"
                          disabled={
                            room.name === overview.adminUser ||
                            deleting === `conn-${connection.id}`
                          }
                          onClick={() => onDeleteConnection(connection)}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <EmptyState label="No connected users" />
          )}
        </CardContent>
      </Card>
    </div>
  );
}
