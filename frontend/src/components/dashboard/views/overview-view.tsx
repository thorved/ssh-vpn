"use client";

import { Activity, DoorOpen, PlugZap, Terminal, Users } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

import {
  CommandLine,
  EmptyState,
  LoadBar,
  MetricCard,
  StatusBadge,
} from "../dashboard-common";
import type { DashboardViewProps } from "../dashboard-types";
import {
  connectCommand,
  dashboardAccessCommand,
  publishCommand,
  summarizeRooms,
} from "../dashboard-utils";

export function OverviewView({
  overview,
  selectedRoom,
  commandPort,
  copied,
  onCopy,
  onSelectRoom,
}: DashboardViewProps) {
  const insights = summarizeRooms(overview.rooms);
  const topRooms = [...overview.rooms]
    .sort((a, b) => {
      if (a.activeChannels !== b.activeChannels) {
        return b.activeChannels - a.activeChannels;
      }
      return b.connectionCount - a.connectionCount;
    })
    .slice(0, 5);
  const roomName = selectedRoom?.name ?? overview.rooms[0]?.name ?? "roomname";
  const dashboardCommand = dashboardAccessCommand(overview);
  const publish = publishCommand(overview, roomName, commandPort);
  const connect = connectCommand(overview, roomName, commandPort);

  return (
    <div className="grid gap-4">
      <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          icon={DoorOpen}
          label="Rooms"
          value={overview.totals.rooms}
          detail={`${insights.liveRooms} live, ${insights.idleRooms} idle`}
          tone="emerald"
        />
        <MetricCard
          icon={Users}
          label="Connections"
          value={overview.totals.connections}
          detail={`${insights.uniqueHosts} unique host${insights.uniqueHosts === 1 ? "" : "s"}`}
          tone="blue"
        />
        <MetricCard
          icon={PlugZap}
          label="Publishers"
          value={overview.totals.publishers}
          detail={`${insights.uniquePorts} exposed port${insights.uniquePorts === 1 ? "" : "s"}`}
          tone="amber"
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
      </section>

      <section className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_420px]">
        <Card>
          <CardHeader className="border-b border-border">
            <CardTitle>Room Activity</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-2 p-3">
            {topRooms.length > 0 ? (
              topRooms.map((room) => (
                <button
                  key={room.name}
                  type="button"
                  onClick={() => onSelectRoom(room.name)}
                  className="grid gap-2 rounded-md border border-border bg-card p-3 text-left hover:bg-muted/70"
                >
                  <div className="flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="truncate font-medium">
                          {room.name}
                        </span>
                        <StatusBadge active={room.activeChannels > 0} />
                      </div>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {room.connectionCount} users, {room.publisherCount}{" "}
                        forwards
                      </p>
                    </div>
                    <Badge>{room.activeChannels} active</Badge>
                  </div>
                  <LoadBar
                    value={room.activeChannels}
                    max={Math.max(room.connectionCount, 1)}
                  />
                </button>
              ))
            ) : (
              <EmptyState label="No rooms connected" />
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="border-b border-border">
            <div className="flex items-center justify-between gap-3">
              <CardTitle>Quick Commands</CardTitle>
              <Terminal className="h-4 w-4 text-muted-foreground" />
            </div>
          </CardHeader>
          <CardContent className="grid gap-3 pt-4">
            <CommandLine
              label="Dashboard"
              value={dashboardCommand}
              copied={copied === "overview-dashboard"}
              onCopy={() => onCopy("overview-dashboard", dashboardCommand)}
            />
            <CommandLine
              label="Publish"
              value={publish}
              copied={copied === "overview-publish"}
              onCopy={() => onCopy("overview-publish", publish)}
            />
            <CommandLine
              label="Connect"
              value={connect}
              copied={copied === "overview-connect"}
              onCopy={() => onCopy("overview-connect", connect)}
            />
            <Button asChild variant="outline">
              <a href="/commands">Open Command Center</a>
            </Button>
          </CardContent>
        </Card>
      </section>
    </div>
  );
}
