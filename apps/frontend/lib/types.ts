/**
 * Zero-Trust Identity Provider Type Definitions
 */

// Subject Types (Identity Primitives)
export type SubjectType = 'USER' | 'GROUP' | 'SERVICE';

export interface Subject {
  id: string;
  name: string;
  type: SubjectType;
  displayLabel: string; // e.g., "User: Alice Johnson"
}

export interface User extends Subject {
  type: 'USER';
  email: string;
  status: 'active' | 'inactive';
  role: string;
  groups: string[]; // Group IDs this user belongs to
  certificateIdentity?: string | null;
  createdAt: string;
}

export interface Group extends Subject {
  type: 'GROUP';
  description: string;
  memberCount: number;
  resourceCount: number;
  createdAt: string;
  updatedAt?: string;
}

export interface ServiceAccount extends Subject {
  type: 'SERVICE';
  status: 'active' | 'inactive';
  associatedResourceCount: number;
  createdAt: string;
}

// Group Membership
export interface GroupMember {
  userId: string;
  userName: string;
  email: string;
}

// Resources and Access Control
export type ResourceType = 'STANDARD' | 'BROWSER' | 'BACKGROUND';
export type FirewallStatus = 'protected' | 'unprotected';

export interface Resource {
  id: string;
  name: string;
  type: ResourceType;
  address: string; // e.g., domain, IP, endpoint
  protocol: 'TCP' | 'UDP';
  portFrom?: number | null;
  portTo?: number | null;
  alias?: string;
  description: string;
  remoteNetworkId?: string;
  firewallStatus: FirewallStatus;
}

// Remote Networks (Twingate-style)
export interface Connector {
  id: string;
  name: string;
  status: 'online' | 'offline' | 'revoked';
  version: string;
  hostname: string;
  remoteNetworkId: string;
  lastSeen: string; // Timestamp of when the connector was last seen online
  installed: boolean;
  lastPolicyVersion: number;
  lastSeenAt?: string | null;
  privateIp?: string;
  revoked?: boolean;
}

export interface RemoteNetwork {
  id: string;
  name: string;
  location: 'AWS' | 'GCP' | 'AZURE' | 'ON_PREM' | 'OTHER';
  connectorCount: number;
  onlineConnectorCount: number;
  resourceCount: number;
  createdAt: string;
  updatedAt: string;
}

export interface Agent {
  id: string;
  name: string;
  status: 'online' | 'offline' | 'revoked';
  version: string;
  hostname: string;
  remoteNetworkId: string; // The remote network this agent is part of
  connectorId?: string;
  revoked?: boolean;
  installed?: boolean;
  lastSeen?: string;
  lastSeenAt?: string | null;
}

// Access Rules bind subjects to resources
export interface AccessRule {
  id: string;
  name: string;
  resourceId: string;
  allowedGroups: string[];
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
}

// Network Discovery
export interface DiscoveredResource {
  id: string;
  ip: string;
  port: number;
  protocol: string;
  serviceName: string;
  reachableFrom: string;
  firstSeen: number;
}

export type ScanStatus = 'pending' | 'in_progress' | 'completed' | 'failed';

export interface ScanJob {
  requestId: string;
  connectorId: string;
  status: ScanStatus;
  targets: string[];
  ports: number[];
  startedAt: string;
  completedAt?: string;
  results?: DiscoveredResource[];
  error?: string;
}

// Workspaces
export interface Workspace {
  id: string;
  name: string;
  slug: string;
  trustDomain: string;
  caCertPem?: string;
  status: string;
  role?: string;
  createdAt: string;
  updatedAt: string;
}

export interface WorkspaceMember {
  workspaceId: string;
  userId: string;
  role: 'owner' | 'admin' | 'member';
  email?: string;
  name?: string;
  joinedAt: string;
}

// Selected Subject for picker
export interface SelectedSubject {
  id: string;
  type: SubjectType;
  label: string;
}

// Diagnostics
export interface ConnectorDiagnostic {
  id: string;
  name: string;
  status: string;
  streamActive: boolean;
  stalenessSeconds: number;
  lastSeenAt: string | null;
  remoteNetworkId: string;
}

export interface TunnelerDiagnostic {
  id: string;
  name: string;
  status: string;
  lastSeenAt: string | null;
}

export interface DiagnosticsData {
  connectors: ConnectorDiagnostic[];
  tunnelers: TunnelerDiagnostic[];
}

export interface PingResult {
  connectorId: string;
  streamActive: boolean;
  stalenessSeconds: number;
  lastSeenAt: string | null;
  message: string;
}

export interface TraceHop {
  type: 'user' | 'group' | 'resource' | 'remote_network' | 'connector';
  id: string;
  name: string;
  status: string;
  healthy: boolean;
}

export interface AccessTrace {
  allowed: boolean;
  reason: string;
  path: TraceHop[];
  userGroups: { id: string; name: string }[];
  matchedRules: { id: string; name: string; enabled: boolean }[];
}
