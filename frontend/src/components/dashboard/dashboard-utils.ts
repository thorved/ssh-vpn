import type { Overview, RoomSnapshot } from "@/types/admin";

export const fallbackPort = 8080;

export function normalizeOverview(overview: Overview): Overview {
  return {
    ...overview,
    rooms: overview.rooms.map((room) => ({
      ...room,
      connections: (room.connections ?? []).map((connection) => ({
        ...connection,
        room: connection.room ?? room.name,
        role: connection.role ?? "connected",
        publishedPorts: connection.publishedPorts ?? [],
        receiverActiveChannels: connection.receiverActiveChannels ?? 0,
        publisherActiveChannels: connection.publisherActiveChannels ?? 0,
      })),
      publishers: (room.publishers ?? []).map((publisher) => ({
        ...publisher,
        connectionId: publisher.connectionId ?? 0,
      })),
    })),
  };
}

export function summarizeRooms(rooms: RoomSnapshot[]) {
  const hosts = new Set<string>();
  const ports = new Set<number>();
  let liveRooms = 0;
  let busiestRoom = "";
  let busiestChannels = -1;

  for (const room of rooms) {
    if (room.activeChannels > 0) {
      liveRooms++;
    }
    if (room.activeChannels > busiestChannels) {
      busiestChannels = room.activeChannels;
      busiestRoom = room.name;
    }
    for (const connection of room.connections) {
      hosts.add(connection.remoteAddr.split(":")[0] || connection.remoteAddr);
    }
    for (const publisher of room.publishers) {
      ports.add(publisher.port);
    }
  }

  return {
    liveRooms,
    idleRooms: Math.max(rooms.length - liveRooms, 0),
    uniqueHosts: hosts.size,
    uniquePorts: ports.size,
    busiestRoom: busiestChannels > 0 ? busiestRoom : "",
  };
}

export function dashboardAccessCommand(overview: Overview) {
  return `ssh -N -L ${overview.adminDashboardPort}:localhost:${overview.adminDashboardPort} ${overview.adminUser}@${overview.publicDomain} -p ${overview.publicSshPort}`;
}

export function publishCommand(overview: Overview, room: string, port: number) {
  return `ssh -N -R ${port}:localhost:${port} ${room}@${overview.publicDomain} -p ${overview.publicSshPort}`;
}

export function connectCommand(overview: Overview, room: string, port: number) {
  return `ssh -N -L ${port}:localhost:${port} ${room}@${overview.publicDomain} -p ${overview.publicSshPort}`;
}

export function clampPort(value: number) {
  if (!Number.isFinite(value)) {
    return fallbackPort;
  }
  return Math.min(Math.max(Math.trunc(value), 1), 65535);
}

export function earliestDate(values: string[]) {
  return values.filter(Boolean).sort()[0] ?? "";
}

export function latestDate(values: string[]) {
  return values.filter(Boolean).sort().at(-1) ?? "";
}

export function connectionAge(value: string) {
  if (!value) {
    return "new";
  }
  const elapsedMs = Date.now() - new Date(value).getTime();
  const minutes = Math.max(Math.floor(elapsedMs / 60000), 0);
  if (minutes < 1) {
    return "now";
  }
  if (minutes < 60) {
    return `${minutes}m`;
  }
  const hours = Math.floor(minutes / 60);
  if (hours < 24) {
    return `${hours}h`;
  }
  return `${Math.floor(hours / 24)}d`;
}

export function formatDate(value: string) {
  if (!value) {
    return "";
  }
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

export function formatTime(value: Date) {
  return new Intl.DateTimeFormat(undefined, {
    hour: "numeric",
    minute: "2-digit",
    second: "2-digit",
  }).format(value);
}
