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
  createdAt: string;
}

export interface Group extends Subject {
  type: 'GROUP';
  description: string;
  memberCount: number;
  createdAt: string;
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
export interface Resource {
  id: string;
  name: string;
  address: string; // e.g., domain, IP, endpoint
  description: string;
}

// Access Rules bind subjects to resources
export interface AccessRule {
  id: string;
  resourceId: string;
  subjectId: string;
  subjectType: SubjectType;
  subjectName: string;
  effect: 'ALLOW' | 'DENY';
  createdAt: string;
}

// Selected Subject for picker
export interface SelectedSubject {
  id: string;
  type: SubjectType;
  label: string;
}
