import crypto from 'crypto';
import { getDb } from './db';
import {
  AccessRule,
  Connector,
  Group,
  GroupMember,
  RemoteNetwork,
  Resource,
  ResourceType,
  ServiceAccount,
  Subject,
  SubjectType,
  Tunneler,
  User,
} from './types';

export interface ConnectorLog {
  id: number;
  timestamp: string;
  message: string;
}

function mapUser(row: any): User {
  return {
    id: row.id,
    name: row.name,
    type: 'USER',
    displayLabel: `User: ${row.name}`,
    email: row.email,
    status: row.status,
    groups: [],
    certificateIdentity: row.certificate_identity ?? null,
    createdAt: row.created_at,
  };
}

function mapGroup(row: any, memberCount: number, resourceCount: number): Group {
  return {
    id: row.id,
    name: row.name,
    type: 'GROUP',
    displayLabel: `Group: ${row.name}`,
    description: row.description,
    memberCount,
    resourceCount,
    createdAt: row.created_at,
    updatedAt: row.updated_at ?? row.created_at,
  };
}

function mapServiceAccount(row: any): ServiceAccount {
  return {
    id: row.id,
    name: row.name,
    type: 'SERVICE',
    displayLabel: `Service: ${row.name}`,
    status: row.status,
    associatedResourceCount: row.associated_resource_count,
    createdAt: row.created_at,
  };
}

function mapResource(row: any): Resource {
  return {
    id: row.id,
    name: row.name,
    type: row.type as ResourceType,
    address: row.address,
    protocol: (row.protocol ?? 'TCP') as 'TCP' | 'UDP',
    portFrom: row.port_from ?? null,
    portTo: row.port_to ?? null,
    alias: row.alias ?? undefined,
    description: row.description,
    remoteNetworkId: row.remote_network_id ?? undefined,
  };
}

function mapConnector(row: any): Connector {
  return {
    id: row.id,
    name: row.name,
    status: row.status,
    version: row.version,
    hostname: row.hostname,
    remoteNetworkId: row.remote_network_id,
    lastSeen: row.last_seen,
    installed: !!row.installed,
    lastPolicyVersion: row.last_policy_version ?? 0,
    lastSeenAt: row.last_seen_at ?? null,
  };
}

function mapRemoteNetwork(row: any): RemoteNetwork {
  return {
    id: row.id,
    name: row.name,
    location: row.location,
    connectorCount: row.connector_count,
    onlineConnectorCount: row.online_connector_count,
    resourceCount: row.resource_count,
    createdAt: row.created_at,
  } as RemoteNetwork;
}

function mapAccessRule(row: any): AccessRule {
  return {
    id: row.id,
    name: row.name,
    resourceId: row.resource_id,
    allowedGroups: row.allowed_groups ?? [],
    enabled: !!row.enabled,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
  };
}

export function listRemoteNetworks(): RemoteNetwork[] {
  const db = getDb();
  const rows = db
    .prepare(
      `
      SELECT n.*,
        (SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id) AS connector_count,
        (SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id AND c.status = 'online') AS online_connector_count,
        (SELECT COUNT(*) FROM resources r WHERE r.remote_network_id = n.id) AS resource_count
      FROM remote_networks n
      ORDER BY n.created_at ASC
      `
    )
    .all();
  return rows.map(mapRemoteNetwork);
}

export function addRemoteNetwork(data: { name: string; location: string }): void {
  const db = getDb();
  db.prepare('INSERT INTO remote_networks (id, name, location, created_at) VALUES (?, ?, ?, ?)')
    .run(`net_${Date.now()}`, data.name, data.location, new Date().toISOString().split('T')[0]);
}

export function getRemoteNetworkDetail(networkId: string): {
  network: RemoteNetwork | undefined;
  connectors: Connector[];
  resources: Resource[];
} {
  const db = getDb();
  const networkRow = db
    .prepare(
      `
      SELECT n.*,
        (SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id) AS connector_count,
        (SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id AND c.status = 'online') AS online_connector_count,
        (SELECT COUNT(*) FROM resources r WHERE r.remote_network_id = n.id) AS resource_count
      FROM remote_networks n
      WHERE n.id = ?
      `
    )
    .get(networkId);

  const connectorRows = db
    .prepare('SELECT * FROM connectors WHERE remote_network_id = ? ORDER BY name ASC')
    .all(networkId);

  const resourceRows = db
    .prepare('SELECT * FROM resources WHERE remote_network_id = ? ORDER BY name ASC')
    .all(networkId);

  return {
    network: networkRow ? mapRemoteNetwork(networkRow) : undefined,
    connectors: connectorRows.map(mapConnector),
    resources: resourceRows.map(mapResource),
  };
}

export function listConnectors(): Connector[] {
  const db = getDb();
  const rows = db.prepare('SELECT * FROM connectors ORDER BY name ASC').all();
  return rows.map(mapConnector);
}

export function getConnectorDetail(connectorId: string): {
  connector: Connector | null;
  network: RemoteNetwork | undefined;
  logs: ConnectorLog[];
} {
  const db = getDb();
  const connectorRow = db.prepare('SELECT * FROM connectors WHERE id = ?').get(connectorId);
  if (!connectorRow) {
    return { connector: null, network: undefined, logs: [] };
  }

  const networkRow = db
    .prepare(
      `
      SELECT n.*,
        (SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id) AS connector_count,
        (SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id AND c.status = 'online') AS online_connector_count,
        (SELECT COUNT(*) FROM resources r WHERE r.remote_network_id = n.id) AS resource_count
      FROM remote_networks n
      WHERE n.id = ?
      `
    )
    .get(connectorRow.remote_network_id);

  const logs = db
    .prepare('SELECT id, timestamp, message FROM connector_logs WHERE connector_id = ? ORDER BY id ASC')
    .all(connectorId) as ConnectorLog[];

  return {
    connector: mapConnector(connectorRow),
    network: networkRow ? mapRemoteNetwork(networkRow) : undefined,
    logs,
  };
}

export function addConnector(data: { name: string; remoteNetworkId: string }): void {
  const db = getDb();
  const id = `con_${Date.now()}`;
  const hostname = `${data.name.toLowerCase().replace(/\s/g, '-')}.local`;
  db.prepare(
    `INSERT INTO connectors (id, name, status, version, hostname, remote_network_id, last_seen, last_policy_version, last_seen_at, installed)
     VALUES (?, ?, 'offline', '1.0.0', ?, ?, ?, 0, ?, 0)`
  ).run(id, data.name, hostname, data.remoteNetworkId, new Date().toISOString(), new Date().toISOString());
}

export function simulateConnectorHeartbeat(connectorId: string): void {
  const db = getDb();
  const now = new Date().toISOString();
  db.prepare('UPDATE connectors SET status = ?, last_seen = ?, last_seen_at = ?, installed = 1 WHERE id = ?')
    .run('online', now, now, connectorId);
}

export function updateConnectorHeartbeat(connectorId: string, lastPolicyVersion: number): { updateAvailable: boolean; currentVersion: number } {
  const db = getDb();
  const now = new Date().toISOString();
  db.prepare('UPDATE connectors SET last_seen_at = ?, last_policy_version = ? WHERE id = ?')
    .run(now, lastPolicyVersion, connectorId);

  const row = db.prepare('SELECT version FROM connector_policy_versions WHERE connector_id = ?').get(connectorId) as { version?: number } | undefined;
  const currentVersion = row?.version ?? 0;
  const updateAvailable = lastPolicyVersion < currentVersion;
  return { updateAvailable, currentVersion };
}

export function listTunnelers(): Tunneler[] {
  const db = getDb();
  const rows = db.prepare('SELECT * FROM tunnelers ORDER BY name ASC').all();
  return rows.map((row) => ({
    id: row.id,
    name: row.name,
    status: row.status,
    version: row.version,
    hostname: row.hostname,
    remoteNetworkId: row.remote_network_id,
  } as Tunneler));
}

export function listUsers(): User[] {
  const db = getDb();
  const userRows = db.prepare('SELECT * FROM users ORDER BY name ASC').all();
  const userGroups = db.prepare('SELECT group_id FROM group_members WHERE user_id = ?');
  return userRows.map((row: any) => {
    const groups = userGroups.all(row.id).map((g: any) => g.group_id);
    return { ...mapUser(row), groups };
  });
}

export function addUser(data: { name: string; email: string; status: 'active' | 'inactive' }): User {
  const db = getDb();
  const id = `usr_${Date.now()}`;
  const certificateIdentity = `identity-${crypto.randomUUID()}`;
  const createdAt = new Date().toISOString().split('T')[0];
  db.prepare(
    'INSERT INTO users (id, name, email, certificate_identity, status, created_at) VALUES (?, ?, ?, ?, ?, ?)'
  ).run(id, data.name, data.email, certificateIdentity, data.status, createdAt);

  return {
    id,
    name: data.name,
    type: 'USER',
    displayLabel: `User: ${data.name}`,
    email: data.email,
    status: data.status,
    groups: [],
    certificateIdentity,
    createdAt,
  };
}

export function listServiceAccounts(): ServiceAccount[] {
  const db = getDb();
  const rows = db.prepare('SELECT * FROM service_accounts ORDER BY name ASC').all();
  return rows.map(mapServiceAccount);
}

export function listGroups(): Group[] {
  const db = getDb();
  const rows = db.prepare('SELECT * FROM groups ORDER BY name ASC').all();
  const memberCountStmt = db.prepare('SELECT COUNT(*) as count FROM group_members WHERE group_id = ?');
  const resourceCountStmt = db.prepare(
    `SELECT COUNT(DISTINCT ar.resource_id) as count
     FROM access_rules ar
     JOIN access_rule_groups arg ON arg.rule_id = ar.id
     WHERE arg.group_id = ?`
  );
  return rows.map((row: any) => {
    const memberCount = memberCountStmt.get(row.id).count as number;
    const resourceCount = resourceCountStmt.get(row.id).count as number;
    return mapGroup(row, memberCount, resourceCount);
  });
}

export function getGroupDetail(groupId: string): {
  group: Group | undefined;
  members: GroupMember[];
  resources: Resource[];
} {
  const db = getDb();
  const groupRow = db.prepare('SELECT * FROM groups WHERE id = ?').get(groupId);
  if (!groupRow) {
    return { group: undefined, members: [], resources: [] };
  }

  const memberRows = db.prepare(
    `
    SELECT u.id as userId, u.name as userName, u.email as email
    FROM group_members gm
    JOIN users u ON u.id = gm.user_id
    WHERE gm.group_id = ?
    ORDER BY u.name ASC
    `
  ).all(groupId) as GroupMember[];

  const resourceRows = db.prepare(
    `
    SELECT r.*
    FROM access_rules ar
    JOIN access_rule_groups arg ON arg.rule_id = ar.id
    JOIN resources r ON r.id = ar.resource_id
    WHERE arg.group_id = ?
    GROUP BY r.id
    ORDER BY r.name ASC
    `
  ).all(groupId);

  const memberCount = memberRows.length;
  const resourceCount = resourceRows.length;

  return {
    group: mapGroup(groupRow, memberCount, resourceCount),
    members: memberRows,
    resources: resourceRows.map(mapResource),
  };
}

export function addGroup(data: { name: string; description: string }): void {
  const db = getDb();
  db.prepare('INSERT INTO groups (id, name, description, created_at) VALUES (?, ?, ?, ?)')
    .run(`grp_${Date.now()}`, data.name, data.description, new Date().toISOString().split('T')[0]);
}

export function updateGroupMembers(groupId: string, memberIds: string[]): void {
  const db = getDb();
  const deleteStmt = db.prepare('DELETE FROM group_members WHERE group_id = ?');
  const insertStmt = db.prepare('INSERT INTO group_members (group_id, user_id) VALUES (?, ?)');
  const tx = db.transaction(() => {
    deleteStmt.run(groupId);
    memberIds.forEach((id) => insertStmt.run(groupId, id));
  });
  tx();
}

export function removeGroupMember(groupId: string, userId: string): void {
  const db = getDb();
  db.prepare('DELETE FROM group_members WHERE group_id = ? AND user_id = ?').run(groupId, userId);
}

export function addGroupResources(groupId: string, resourceIds: string[]): void {
  const db = getDb();
  if (resourceIds.length === 0) return;

  const existingStmt = db.prepare(
    `SELECT ar.id
     FROM access_rules ar
     JOIN access_rule_groups arg ON arg.rule_id = ar.id
     WHERE ar.resource_id = ? AND arg.group_id = ?`
  );
  const insertRule = db.prepare(
    `INSERT INTO access_rules (id, name, resource_id, enabled, created_at, updated_at)
     VALUES (?, ?, ?, 1, ?, ?)`
  );
  const insertRuleGroup = db.prepare(
    'INSERT INTO access_rule_groups (rule_id, group_id) VALUES (?, ?)'
  );

  const groupNameRow = db.prepare('SELECT name FROM groups WHERE id = ?').get(groupId);
  const groupName = groupNameRow?.name ?? 'Unknown Group';
  const now = new Date().toISOString().split('T')[0];

  const tx = db.transaction(() => {
    resourceIds.forEach((resourceId) => {
      const existing = existingStmt.get(resourceId, groupId);
      if (existing) return;
      const ruleId = `rule_${Date.now()}_${groupId}_${resourceId}`;
      insertRule.run(ruleId, `${groupName} access`, resourceId, now, now);
      insertRuleGroup.run(ruleId, groupId);
    });
  });
  tx();
}

export function listResources(): Resource[] {
  const db = getDb();
  const rows = db.prepare('SELECT * FROM resources ORDER BY name ASC').all();
  return rows.map(mapResource);
}

export function getResourceDetail(resourceId: string): { resource: Resource | undefined; accessRules: AccessRule[] } {
  const db = getDb();
  const resourceRow = db.prepare('SELECT * FROM resources WHERE id = ?').get(resourceId);
  const accessRuleRows = db
    .prepare('SELECT * FROM access_rules WHERE resource_id = ? ORDER BY created_at ASC')
    .all(resourceId);
  const groupStmt = db.prepare(
    'SELECT group_id FROM access_rule_groups WHERE rule_id = ? ORDER BY group_id ASC'
  );
  return {
    resource: resourceRow ? mapResource(resourceRow) : undefined,
    accessRules: accessRuleRows.map((row: any) => {
      const groups = groupStmt.all(row.id).map((g: any) => g.group_id);
      return mapAccessRule({ ...row, allowed_groups: groups });
    }),
  };
}

export function addResource(data: {
  network_id: string;
  name: string;
  type: ResourceType;
  address: string;
  protocol: 'TCP' | 'UDP';
  port_from?: number | null;
  port_to?: number | null;
  alias?: string;
}): void {
  const db = getDb();
  const ports = data.port_from && data.port_to
    ? `${data.port_from}-${data.port_to}`
    : data.port_from
      ? `${data.port_from}`
      : '';
  db.prepare(
    `INSERT INTO resources (id, name, type, address, ports, protocol, port_from, port_to, alias, description, remote_network_id)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
  ).run(
    `res_${Date.now()}`,
    data.name,
    data.type,
    data.address,
    ports,
    data.protocol,
    data.port_from ?? null,
    data.port_to ?? null,
    data.alias ?? null,
    `A new ${data.type.toLowerCase()} resource`,
    data.network_id
  );
}

export function updateResource(resourceId: string, data: {
  network_id: string;
  name: string;
  type: ResourceType;
  address: string;
  protocol: 'TCP' | 'UDP';
  port_from?: number | null;
  port_to?: number | null;
  alias?: string;
}): void {
  const db = getDb();
  const ports = data.port_from && data.port_to
    ? `${data.port_from}-${data.port_to}`
    : data.port_from
      ? `${data.port_from}`
      : '';
  db.prepare(
    `UPDATE resources
     SET name = ?, type = ?, address = ?, ports = ?, protocol = ?, port_from = ?, port_to = ?, alias = ?, remote_network_id = ?
     WHERE id = ?`
  ).run(
    data.name,
    data.type,
    data.address,
    ports,
    data.protocol,
    data.port_from ?? null,
    data.port_to ?? null,
    data.alias ?? null,
    data.network_id,
    resourceId
  );
}

export function createAccessRule(resourceId: string, data: { name: string; groupIds: string[]; enabled: boolean }): AccessRule {
  const db = getDb();
  const ruleId = `rule_${Date.now()}`;
  const now = new Date().toISOString().split('T')[0];

  const insertRule = db.prepare(
    `INSERT INTO access_rules (id, name, resource_id, enabled, created_at, updated_at)
     VALUES (?, ?, ?, ?, ?, ?)`
  );
  const insertRuleGroup = db.prepare(
    'INSERT INTO access_rule_groups (rule_id, group_id) VALUES (?, ?)'
  );

  const tx = db.transaction(() => {
    insertRule.run(ruleId, data.name, resourceId, data.enabled ? 1 : 0, now, now);
    data.groupIds.forEach((groupId) => insertRuleGroup.run(ruleId, groupId));
  });
  tx();

  return {
    id: ruleId,
    name: data.name,
    resourceId,
    allowedGroups: data.groupIds,
    enabled: data.enabled,
    createdAt: now,
    updatedAt: now,
  };
}

export function deleteAccessRule(ruleId: string): void {
  const db = getDb();
  db.prepare('DELETE FROM access_rules WHERE id = ?').run(ruleId);
}

export function listSubjects(type?: SubjectType): Subject[] {
  const db = getDb();
  const subjects: Subject[] = [];

  if (!type || type === 'USER') {
    const rows = db.prepare('SELECT * FROM users ORDER BY name ASC').all();
    subjects.push(...rows.map((row: any) => ({
      id: row.id,
      name: row.name,
      type: 'USER',
      displayLabel: `User: ${row.name}`,
    })));
  }

  if (!type || type === 'GROUP') {
    const rows = db.prepare('SELECT * FROM groups ORDER BY name ASC').all();
    subjects.push(...rows.map((row: any) => ({
      id: row.id,
      name: row.name,
      type: 'GROUP',
      displayLabel: `Group: ${row.name}`,
    })));
  }

  if (!type || type === 'SERVICE') {
    const rows = db.prepare('SELECT * FROM service_accounts ORDER BY name ASC').all();
    subjects.push(...rows.map((row: any) => ({
      id: row.id,
      name: row.name,
      type: 'SERVICE',
      displayLabel: `Service: ${row.name}`,
    })));
  }

  return subjects;
}
