import type { ConnectionSnapshot, Overview, RoomSnapshot } from "@/types/admin";

export type DashboardView = "overview" | "rooms" | "connections" | "commands";

export type DashboardViewProps = {
  overview: Overview;
  selectedRoom: RoomSnapshot | null;
  selectedRoomName: string;
  commandPort: number;
  copied: string;
  deleting: string;
  onSelectRoom: (room: string) => void;
  onCommandPortChange: (port: number) => void;
  onCopy: (key: string, value: string) => void;
  onDeleteRoom: (room: RoomSnapshot) => void;
  onDeleteConnection: (connection: ConnectionSnapshot) => void;
};
