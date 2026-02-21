import Database from 'better-sqlite3';
import path from 'path';

const DB_PATH = path.join(process.cwd(), 'ztna.db');

let dbInstance: Database.Database | null = null;

function initSchema(db: Database.Database) {
  db.exec(`
    PRAGMA foreign_keys = ON;
    PRAGMA journal_mode = WAL;

    CREATE TABLE IF NOT EXISTS meta (
      key TEXT PRIMARY KEY,
      value TEXT NOT NULL
    );

    CREATE TABLE IF NOT EXISTS users (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      email TEXT NOT NULL,
      status TEXT NOT NULL,
      created_at TEXT NOT NULL
    );

    CREATE TABLE IF NOT EXISTS groups (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      description TEXT NOT NULL,
      created_at TEXT NOT NULL
    );

    CREATE TABLE IF NOT EXISTS group_members (
      group_id TEXT NOT NULL,
      user_id TEXT NOT NULL,
      PRIMARY KEY (group_id, user_id),
      FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE,
      FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
    );

    CREATE TABLE IF NOT EXISTS service_accounts (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      status TEXT NOT NULL,
      associated_resource_count INTEGER NOT NULL DEFAULT 0,
      created_at TEXT NOT NULL
    );

    CREATE TABLE IF NOT EXISTS remote_networks (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      location TEXT NOT NULL,
      created_at TEXT NOT NULL
    );

    CREATE TABLE IF NOT EXISTS connectors (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      status TEXT NOT NULL,
      version TEXT NOT NULL,
      hostname TEXT NOT NULL,
      remote_network_id TEXT NOT NULL,
      last_seen TEXT NOT NULL,
      installed INTEGER NOT NULL DEFAULT 0,
      FOREIGN KEY (remote_network_id) REFERENCES remote_networks(id) ON DELETE CASCADE
    );

    CREATE TABLE IF NOT EXISTS tunnelers (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      status TEXT NOT NULL,
      version TEXT NOT NULL,
      hostname TEXT NOT NULL,
      remote_network_id TEXT NOT NULL,
      FOREIGN KEY (remote_network_id) REFERENCES remote_networks(id) ON DELETE CASCADE
    );

    CREATE TABLE IF NOT EXISTS resources (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      type TEXT NOT NULL,
      address TEXT NOT NULL,
      ports TEXT NOT NULL,
      alias TEXT,
      description TEXT NOT NULL,
      remote_network_id TEXT,
      FOREIGN KEY (remote_network_id) REFERENCES remote_networks(id) ON DELETE SET NULL
    );

    CREATE TABLE IF NOT EXISTS access_rules (
      id TEXT PRIMARY KEY,
      resource_id TEXT NOT NULL,
      subject_id TEXT NOT NULL,
      subject_type TEXT NOT NULL,
      subject_name TEXT NOT NULL,
      effect TEXT NOT NULL,
      created_at TEXT NOT NULL,
      FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
    );

    CREATE TABLE IF NOT EXISTS connector_logs (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      connector_id TEXT NOT NULL,
      timestamp TEXT NOT NULL,
      message TEXT NOT NULL,
      FOREIGN KEY (connector_id) REFERENCES connectors(id) ON DELETE CASCADE
    );
  `);
}

function ensureConnectorInstalledColumn(db: Database.Database) {
  const columns = db.prepare('PRAGMA table_info(connectors)').all() as { name: string }[];
  const hasInstalled = columns.some((col) => col.name === 'installed');
  if (!hasInstalled) {
    db.exec(`ALTER TABLE connectors ADD COLUMN installed INTEGER NOT NULL DEFAULT 0`);
    db.exec(`UPDATE connectors SET installed = 1`);
  }
}

function seedIfNeeded(db: Database.Database) {
  const seeded = db
    .prepare('SELECT value FROM meta WHERE key = ?')
    .get('seeded') as { value?: string } | undefined;

  if (seeded?.value === '1') return;

  const insertMeta = db.prepare(
    'INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value'
  );

  const insertUser = db.prepare(
    'INSERT INTO users (id, name, email, status, created_at) VALUES (?, ?, ?, ?, ?)'
  );
  const insertGroup = db.prepare(
    'INSERT INTO groups (id, name, description, created_at) VALUES (?, ?, ?, ?)'
  );
  const insertGroupMember = db.prepare(
    'INSERT INTO group_members (group_id, user_id) VALUES (?, ?)'
  );
  const insertServiceAccount = db.prepare(
    'INSERT INTO service_accounts (id, name, status, associated_resource_count, created_at) VALUES (?, ?, ?, ?, ?)'
  );
  const insertRemoteNetwork = db.prepare(
    'INSERT INTO remote_networks (id, name, location, created_at) VALUES (?, ?, ?, ?)'
  );
  const insertConnector = db.prepare(
    'INSERT INTO connectors (id, name, status, version, hostname, remote_network_id, last_seen, installed) VALUES (?, ?, ?, ?, ?, ?, ?, ?)'
  );
  const insertTunneler = db.prepare(
    'INSERT INTO tunnelers (id, name, status, version, hostname, remote_network_id) VALUES (?, ?, ?, ?, ?, ?)'
  );
  const insertResource = db.prepare(
    'INSERT INTO resources (id, name, type, address, ports, alias, description, remote_network_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)'
  );
  const insertAccessRule = db.prepare(
    'INSERT INTO access_rules (id, resource_id, subject_id, subject_type, subject_name, effect, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)'
  );
  const insertConnectorLog = db.prepare(
    'INSERT INTO connector_logs (connector_id, timestamp, message) VALUES (?, ?, ?)'
  );

  const seedTransaction = db.transaction(() => {
    // Remote Networks
    insertRemoteNetwork.run('net_1', 'Production AWS', 'AWS', '2026-01-05');
    insertRemoteNetwork.run('net_2', 'Office LAN', 'ON_PREM', '2026-01-08');
    insertRemoteNetwork.run('net_3', 'Staging GCP', 'GCP', '2026-01-15');

    // Connectors
    insertConnector.run(
      'con_1',
      'AWS-Prod-Connector-1',
      'online',
      '1.2.0',
      'ip-172-31-0-1.ec2.internal',
      'net_1',
      '2026-02-20T10:30:00Z',
      1
    );
    insertConnector.run(
      'con_2',
      'AWS-Prod-Connector-2',
      'online',
      '1.2.0',
      'ip-172-31-0-2.ec2.internal',
      'net_1',
      '2026-02-20T10:25:00Z',
      1
    );
    insertConnector.run(
      'con_3',
      'Office-Connector-1',
      'online',
      '1.1.5',
      'office-server.local',
      'net_2',
      '2026-02-20T10:15:00Z',
      1
    );
    insertConnector.run(
      'con_4',
      'GCP-Staging-Connector-1',
      'online',
      '1.2.0',
      'gcp-staging-vm-1',
      'net_3',
      '2026-02-20T10:05:00Z',
      1
    );
    insertConnector.run(
      'con_5',
      'GCP-Staging-Connector-2',
      'offline',
      '1.2.0',
      'gcp-staging-vm-2',
      'net_3',
      '2026-02-19T14:00:00Z',
      1
    );

    // Tunnelers
    insertTunneler.run(
      'tun_1',
      'AWS-Prod-Tunneler-1',
      'online',
      '1.0.0',
      'tun-172-31-0-10.ec2.internal',
      'net_1'
    );
    insertTunneler.run(
      'tun_2',
      'AWS-Prod-Tunneler-2',
      'offline',
      '1.0.0',
      'tun-172-31-0-11.ec2.internal',
      'net_1'
    );
    insertTunneler.run(
      'tun_3',
      'Office-Tunneler-1',
      'online',
      '1.0.1',
      'tun-office-server.local',
      'net_2'
    );

    // Users
    insertUser.run('usr_1', 'Alice Johnson', 'alice@company.com', 'active', '2026-01-10');
    insertUser.run('usr_2', 'Bob Smith', 'bob@company.com', 'active', '2026-01-12');
    insertUser.run('usr_3', 'Charlie Davis', 'charlie@company.com', 'active', '2026-01-15');
    insertUser.run('usr_4', 'Diana Wilson', 'diana@company.com', 'inactive', '2026-02-01');

    // Groups
    insertGroup.run('grp_1', 'Engineering', 'Engineering team with database and API access', '2026-01-15');
    insertGroup.run('grp_2', 'Marketing', 'Marketing department', '2026-01-20');
    insertGroup.run('grp_3', 'Admin', 'System administrators', '2026-01-25');

    // Group Members
    insertGroupMember.run('grp_1', 'usr_1');
    insertGroupMember.run('grp_1', 'usr_3');
    insertGroupMember.run('grp_2', 'usr_2');
    insertGroupMember.run('grp_3', 'usr_1');

    // Service Accounts
    insertServiceAccount.run('svc_1', 'CI/CD Pipeline', 'active', 2, '2026-01-01');
    insertServiceAccount.run('svc_2', 'Analytics Sync', 'active', 1, '2026-01-10');

    // Resources
    insertResource.run(
      'res_1',
      'Database Server',
      'STANDARD',
      'db.internal.company.com:5432',
      '5432',
      null,
      'Production PostgreSQL database for main application',
      'net_1'
    );
    insertResource.run(
      'res_2',
      'API Gateway',
      'BROWSER',
      'api.company.com',
      '443',
      null,
      'Main API endpoint for frontend applications',
      'net_1'
    );
    insertResource.run(
      'res_3',
      'S3 Bucket',
      'BACKGROUND',
      'company-assets.s3.amazonaws.com',
      '443',
      null,
      'Asset storage bucket',
      'net_1'
    );
    insertResource.run(
      'res_4',
      'Internal Wiki',
      'BROWSER',
      'wiki.internal.company.com',
      '80,443',
      null,
      'Internal Confluence Wiki',
      'net_2'
    );

    // Access Rules
    insertAccessRule.run('rule_1', 'res_1', 'grp_1', 'GROUP', 'Engineering', 'ALLOW', '2026-01-20');
    insertAccessRule.run('rule_2', 'res_2', 'grp_1', 'GROUP', 'Engineering', 'ALLOW', '2026-01-20');
    insertAccessRule.run('rule_3', 'res_3', 'svc_1', 'SERVICE', 'CI/CD Pipeline', 'ALLOW', '2026-01-21');
    insertAccessRule.run('rule_4', 'res_2', 'svc_2', 'SERVICE', 'Analytics Sync', 'ALLOW', '2026-01-22');

    // Connector Logs
    insertConnectorLog.run('con_1', '2026-02-20 10:00:00', 'Connector service started');
    insertConnectorLog.run('con_1', '2026-02-20 10:00:05', 'Successfully connected to the Twingate network');
    insertConnectorLog.run('con_1', '2026-02-20 10:02:10', 'Authenticated with controller');

    insertMeta.run('seeded', '1');
  });

  seedTransaction();
}

export function getDb() {
  if (!dbInstance) {
    dbInstance = new Database(DB_PATH);
    initSchema(dbInstance);
    ensureConnectorInstalledColumn(dbInstance);
    seedIfNeeded(dbInstance);
  }
  return dbInstance;
}

export { DB_PATH };
