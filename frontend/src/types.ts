export type TunnelStatus = "disconnected" | "connecting" | "connected" | "reconnecting" | "error";

export interface PortForward {
  localPort: number;
  remoteHost: string;
  remotePort: number;
  description: string;
}

export interface TunnelConfig {
  id: string;
  name: string;
  host: string;
  port: number;
  user: string;
  keyPath: string;
  portForwards: PortForward[];
  proxyCommand?: string;
  proxyJump?: string;
  color?: string;
  group: string;
  autoConnect: boolean;
}

export interface StatusEvent {
  tunnelId: string;
  status: TunnelStatus;
  error?: string;
}
