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
  SelectedSubject,
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
    ports: row.ports,
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
    resourceId: row.resource_id,
    subjectId: row.subject_id,
    subjectType: row.subject_type as SubjectType,
    subjectName: row.subject_name,
    effect: row.effect,
    createdAt: row.created_at,
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
    `INSERT INTO connectors (id, name, status, version, hostname, remote_network_id, last_seen, installed)
     VALUES (?, ?, 'offline', '1.0.0', ?, ?, ?, 0)`
  ).run(id, data.name, hostname, data.remoteNetworkId, new Date().toISOString());
}

export function simulateConnectorHeartbeat(connectorId: string): void {
  const db = getDb();
  db.prepare('UPDATE connectors SET status = ?, last_seen = ?, installed = 1 WHERE id = ?')
    .run('online', new Date().toISOString(), connectorId);
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
    "SELECT COUNT(*) as count FROM access_rules WHERE subject_type = 'GROUP' AND subject_id = ?"
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
    JOIN resources r ON r.id = ar.resource_id
    WHERE ar.subject_type = 'GROUP' AND ar.subject_id = ?
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
  const existingStmt = db.prepare(
    "SELECT id FROM access_rules WHERE subject_type = 'GROUP' AND subject_id = ? AND resource_id = ?"
  );
  const insertStmt = db.prepare(
    `INSERT INTO access_rules (id, resource_id, subject_id, subject_type, subject_name, effect, created_at)
     VALUES (?, ?, ?, 'GROUP', ?, 'ALLOW', ?)`
  );

  const groupNameRow = db.prepare('SELECT name FROM groups WHERE id = ?').get(groupId);
  const groupName = groupNameRow?.name ?? 'Unknown Group';

  const tx = db.transaction(() => {
    resourceIds.forEach((resourceId) => {
      const existing = existingStmt.get(groupId, resourceId);
      if (!existing) {
        insertStmt.run(
          `rule_${Date.now()}_${groupId}_${resourceId}`,
          resourceId,
          groupId,
          groupName,
          new Date().toISOString().split('T')[0]
        );
      }
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
  return {
    resource: resourceRow ? mapResource(resourceRow) : undefined,
    accessRules: accessRuleRows.map(mapAccessRule),
  };
}

export function addResource(data: {
  network_id: string;
  name: string;
  type: ResourceType;
  address: string;
  ports: string;
  alias?: string;
}): void {
  const db = getDb();
  db.prepare(
    `INSERT INTO resources (id, name, type, address, ports, alias, description, remote_network_id)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
  ).run(
    `res_${Date.now()}`,
    data.name,
    data.type,
    data.address,
    data.ports,
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
  ports: string;
  alias?: string;
}): void {
  const db = getDb();
  db.prepare(
    `UPDATE resources
     SET name = ?, type = ?, address = ?, ports = ?, alias = ?, remote_network_id = ?
     WHERE id = ?`
  ).run(
    data.name,
    data.type,
    data.address,
    data.ports,
    data.alias ?? null,
    data.network_id,
    resourceId
  );
}

function resolveSubjectName(subject: SelectedSubject): string {
  const db = getDb();
  if (subject.type === 'USER') {
    const row = db.prepare('SELECT name FROM users WHERE id = ?').get(subject.id);
    return row?.name ?? subject.label;
  }
  if (subject.type === 'GROUP') {
    const row = db.prepare('SELECT name FROM groups WHERE id = ?').get(subject.id);
    return row?.name ?? subject.label;
  }
  const row = db.prepare('SELECT name FROM service_accounts WHERE id = ?').get(subject.id);
  return row?.name ?? subject.label;
}

export function createAccessRule(resourceId: string, subjects: SelectedSubject[], effect: 'ALLOW' | 'DENY'): void {
  const db = getDb();
  const insertStmt = db.prepare(
    `INSERT INTO access_rules (id, resource_id, subject_id, subject_type, subject_name, effect, created_at)
     VALUES (?, ?, ?, ?, ?, ?, ?)`
  );
  const tx = db.transaction(() => {
    subjects.forEach((subject) => {
      const subjectName = resolveSubjectName(subject);
      insertStmt.run(
        `rule_${Date.now()}_${subject.id}`,
        resourceId,
        subject.id,
        subject.type,
        subjectName,
        effect,
        new Date().toISOString().split('T')[0]
      );
    });
  });
  tx();
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
