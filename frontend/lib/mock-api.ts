/**
 * API client for Zero-Trust Identity Provider
 * Uses Next.js API routes backed by SQLite.
 */

import {
  User,
  Group,
  ServiceAccount,
  GroupMember,
  Resource,
  AccessRule,
  Subject,
  SelectedSubject,
  RemoteNetwork,
  Connector,
  Tunneler,
  ResourceType,
} from './types';

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(path, {
    headers: {
      'Content-Type': 'application/json',
      ...(options.headers || {}),
    },
    ...options,
  });

  if (!res.ok) {
    const message = await res.text();
    throw new Error(message || `Request failed with ${res.status}`);
  }

  return res.json() as Promise<T>;
}

// API: Get single remote network with connectors and resources
export async function getRemoteNetwork(networkId: string) {
  return request<{ network: RemoteNetwork | undefined; connectors: Connector[]; resources: Resource[] }>(
    `/api/remote-networks/${networkId}`
  );
}

// API: Get single connector with details
export async function getConnector(connectorId: string) {
  return request<{ connector: Connector | null; network: RemoteNetwork | undefined; logs: any[] }>(
    `/api/connectors/${connectorId}`
  );
}

// API: Get all remote networks
export async function getRemoteNetworks(): Promise<RemoteNetwork[]> {
  return request<RemoteNetwork[]>('/api/remote-networks');
}

export async function addRemoteNetwork(data: { name: string; location: string }): Promise<void> {
  await request('/api/remote-networks', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

// API: Get all connectors
export async function getConnectors(): Promise<Connector[]> {
  return request<Connector[]>('/api/connectors');
}

// API: Get all tunnelers
export async function getTunnelers(): Promise<Tunneler[]> {
  return request<Tunneler[]>('/api/tunnelers');
}

// API: Get all subjects (Users, Groups, Service Accounts)
export async function getSubjects(): Promise<Subject[]> {
  return request<Subject[]>('/api/subjects');
}

// API: Get subjects filtered by type
export async function getSubjectsByType(type?: 'USER' | 'GROUP' | 'SERVICE'): Promise<Subject[]> {
  if (!type) return getSubjects();
  return request<Subject[]>(`/api/subjects?type=${encodeURIComponent(type)}`);
}

// API: Get all groups
export async function getGroups(): Promise<Group[]> {
  return request<Group[]>('/api/groups');
}

// API: Get single group with members
export async function getGroup(groupId: string) {
  return request<{ group: Group | undefined; members: GroupMember[]; resources: Resource[] }>(
    `/api/groups/${groupId}`
  );
}

// API: Get all users
export async function getUsers(): Promise<User[]> {
  return request<User[]>('/api/users');
}

// API: Get all service accounts
export async function getServiceAccounts(): Promise<ServiceAccount[]> {
  return request<ServiceAccount[]>('/api/service-accounts');
}

// API: Get single resource with access rules
export async function getResource(resourceId: string) {
  return request<{ resource: Resource | undefined; accessRules: AccessRule[] }>(
    `/api/resources/${resourceId}`
  );
}

// API: Get all resources
export async function getResources(): Promise<Resource[]> {
  return request<Resource[]>('/api/resources');
}

// API: Add a new resource
export async function addResource(data: {
  network_id: string;
  name: string;
  type: ResourceType;
  address: string;
  ports: string;
  alias?: string;
}): Promise<void> {
  await request('/api/resources', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

// API: Update an existing resource
export async function updateResource(
  resourceId: string,
  data: {
    network_id: string;
    name: string;
    type: ResourceType;
    address: string;
    ports: string;
    alias?: string;
  }
): Promise<void> {
  await request(`/api/resources/${resourceId}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

// API: Add a new connector
export async function addConnector(data: {
  name: string;
  remoteNetworkId: string;
}): Promise<void> {
  await request('/api/connectors', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

// API: Simulate a connector sending a heartbeat (going online)
export async function simulateConnectorHeartbeat(connectorId: string): Promise<void> {
  await request(`/api/connectors/${connectorId}/heartbeat`, {
    method: 'POST',
  });
}

// API: Add a new group
export async function addGroup({
  name,
  description,
}: {
  name: string;
  description: string;
}): Promise<void> {
  await request('/api/groups', {
    method: 'POST',
    body: JSON.stringify({ name, description }),
  });
}

// API: Add resources to a group (by creating access rules)
export async function addGroupResources(
  groupId: string,
  resourceIds: string[]
): Promise<void> {
  await request(`/api/groups/${groupId}/resources`, {
    method: 'POST',
    body: JSON.stringify({ resourceIds }),
  });
}

// API: Update group membership
export async function updateGroupMembers(
  groupId: string,
  memberIds: string[]
): Promise<void> {
  await request(`/api/groups/${groupId}/members`, {
    method: 'POST',
    body: JSON.stringify({ memberIds }),
  });
}

// API: Create access rule
export async function createAccessRule(
  resourceId: string,
  subjects: SelectedSubject[],
  effect: 'ALLOW' | 'DENY'
): Promise<void> {
  await request('/api/access-rules', {
    method: 'POST',
    body: JSON.stringify({ resourceId, subjects, effect }),
  });
}

// API: Delete access rule
export async function deleteAccessRule(ruleId: string): Promise<void> {
  await request(`/api/access-rules/${ruleId}`, {
    method: 'DELETE',
  });
}

// API: Delete group member
export async function removeGroupMember(
  groupId: string,
  userId: string
): Promise<void> {
  await request(`/api/groups/${groupId}/members/${userId}`, {
    method: 'DELETE',
  });
}
