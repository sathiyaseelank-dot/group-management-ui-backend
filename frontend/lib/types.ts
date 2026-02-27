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
}

// Remote Networks (Twingate-style)
export interface Connector {
  id: string;
  name: string;
  status: 'online' | 'offline';
  version: string;
  hostname: string;
  remoteNetworkId: string;
  lastSeen: string; // Timestamp of when the connector was last seen online
  installed: boolean;
  lastPolicyVersion: number;
  lastSeenAt?: string | null;
}

export interface RemoteNetwork {
  id: string;
  name: string;
  location: 'AWS' | 'GCP' | 'AZURE' | 'ON_PREM' | 'OTHER';
  connectorCount: number;
  onlineConnectorCount: number;
  resourceCount: number;
  createdAt: string;
}

export interface Tunneler {
  id: string;
  name: string;
  status: 'online' | 'offline';
  version: string;
  hostname: string;
  remoteNetworkId: string; // The remote network this tunneler is part of
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

// Selected Subject for picker
export interface SelectedSubject {
  id: string;
  type: SubjectType;
  label: string;
}
