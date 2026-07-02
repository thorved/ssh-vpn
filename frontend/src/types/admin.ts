export type Overview = {
  publicDomain: string;
  publicSshPort: string;
  adminUser: string;
  adminDashboardPort: number;
  totals: OverviewTotals;
  rooms: RoomSnapshot[];
};

export type OverviewTotals = {
  rooms: number;
  connections: number;
  publishers: number;
  activeChannels: number;
};

export type RoomSnapshot = {
  name: string;
  connectionCount: number;
  publisherCount: number;
  activeChannels: number;
  connections: ConnectionSnapshot[];
  publishers: PublisherSnapshot[];
};

export type ConnectionSnapshot = {
  id: number;
  room: string;
  remoteAddr: string;
  connectedAt: string;
  activeChannels: number;
  role: "publisher" | "receiver" | "publisher+receiver" | "connected";
  publishedPorts: number[];
  receiverActiveChannels: number;
  publisherActiveChannels: number;
};

export type PublisherSnapshot = {
  bindHost: string;
  port: number;
  connectionId: number;
  remoteAddr: string;
  registeredAt: string;
};
