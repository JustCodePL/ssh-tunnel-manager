export type TunnelStatus = "disconnected" | "connecting" | "connected" | "reconnecting" | "error";

export interface PortForward {
  localPort: number;
  remoteHost: string;
  remotePort: number;
  description: string;
  portless?: boolean;
  domain?: string;
  exposePort?: number;
  hostHeader?: string;
  hostHeaderOn?: boolean;
  protocol?: string;
  sshUser?: string;
}

export const PORTLESS_TLD = "ssh-local";

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
  pinned?: boolean;
  rdpPort?: number;
  portlessSuffix?: string;
  sourceFile?: string;
  sourceFileLabel?: string;
}

export interface StatusEvent {
  tunnelId: string;
  status: TunnelStatus;
  error?: string;
}

export interface PortForwardError {
  tunnelId: string;
  localAddress: string;
  remoteAddress: string;
  message: string;
}

export interface LogEntry {
  timestamp: string;
  level: "info" | "warn" | "error";
  message: string;
}

export interface ConfigFileInfo {
  path: string;
  label: string;
}

export interface RemoteDesktop {
  /** "rd:" followed by the name. */
  id: string;
  name: string;
  address: string;
  port: number;
  protocol: "rdp" | "vnc";
  group: string;
  color?: string;
  pinned: boolean;
  sourceFile?: string;
  sourceFileLabel?: string;
}

/** The prefix of a remote desktop's ID, which tool windows take as the tunnel ID. */
export const REMOTE_DESKTOP_PREFIX = "rd:";

export function isRemoteDesktopId(id: string | undefined): boolean {
  return !!id && id.startsWith(REMOTE_DESKTOP_PREFIX);
}
