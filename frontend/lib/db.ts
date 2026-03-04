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
      certificate_identity TEXT UNIQUE,
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
      last_policy_version INTEGER NOT NULL DEFAULT 0,
      last_seen_at TEXT,
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
      protocol TEXT NOT NULL DEFAULT 'TCP',
      port_from INTEGER,
      port_to INTEGER,
      alias TEXT,
      description TEXT NOT NULL,
      remote_network_id TEXT,
      FOREIGN KEY (remote_network_id) REFERENCES remote_networks(id) ON DELETE SET NULL
    );

    CREATE TABLE IF NOT EXISTS access_rules (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      resource_id TEXT NOT NULL,
      enabled INTEGER NOT NULL DEFAULT 1,
      created_at TEXT NOT NULL,
      updated_at TEXT NOT NULL,
      FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
    );

    CREATE TABLE IF NOT EXISTS access_rule_groups (
      rule_id TEXT NOT NULL,
      group_id TEXT NOT NULL,
      PRIMARY KEY (rule_id, group_id),
      FOREIGN KEY (rule_id) REFERENCES access_rules(id) ON DELETE CASCADE,
      FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE
    );

    CREATE TABLE IF NOT EXISTS connector_logs (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      connector_id TEXT NOT NULL,
      timestamp TEXT NOT NULL,
      message TEXT NOT NULL,
      FOREIGN KEY (connector_id) REFERENCES connectors(id) ON DELETE CASCADE
    );

    CREATE TABLE IF NOT EXISTS connector_policy_versions (
      connector_id TEXT PRIMARY KEY,
      version INTEGER NOT NULL DEFAULT 0,
      compiled_at TEXT NOT NULL,
      policy_hash TEXT
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

function ensureUserCertificateIdentity(db: Database.Database) {
  const columns = db.prepare('PRAGMA table_info(users)').all() as { name: string }[];
  const hasColumn = columns.some((col) => col.name === 'certificate_identity');
  if (!hasColumn) {
    db.exec(`ALTER TABLE users ADD COLUMN certificate_identity TEXT UNIQUE`);
  }
}

function ensureResourceProtocolColumns(db: Database.Database) {
  const columns = db.prepare('PRAGMA table_info(resources)').all() as { name: string }[];
  const hasProtocol = columns.some((col) => col.name === 'protocol');
  const hasPortFrom = columns.some((col) => col.name === 'port_from');
  const hasPortTo = columns.some((col) => col.name === 'port_to');
  if (!hasProtocol) {
    db.exec(`ALTER TABLE resources ADD COLUMN protocol TEXT NOT NULL DEFAULT 'TCP'`);
  }
  if (!hasPortFrom) {
    db.exec(`ALTER TABLE resources ADD COLUMN port_from INTEGER`);
  }
  if (!hasPortTo) {
    db.exec(`ALTER TABLE resources ADD COLUMN port_to INTEGER`);
  }
}

function ensureConnectorPolicyColumns(db: Database.Database) {
  const columns = db.prepare('PRAGMA table_info(connectors)').all() as { name: string }[];
  const hasLastPolicy = columns.some((col) => col.name === 'last_policy_version');
  const hasLastSeenAt = columns.some((col) => col.name === 'last_seen_at');
  if (!hasLastPolicy) {
    db.exec(`ALTER TABLE connectors ADD COLUMN last_policy_version INTEGER NOT NULL DEFAULT 0`);
  }
  if (!hasLastSeenAt) {
    db.exec(`ALTER TABLE connectors ADD COLUMN last_seen_at TEXT`);
    db.exec(`UPDATE connectors SET last_seen_at = last_seen WHERE last_seen_at IS NULL`);
  }
}

function ensureConnectorPolicyVersionsTable(db: Database.Database) {
  db.exec(`
    CREATE TABLE IF NOT EXISTS connector_policy_versions (
      connector_id TEXT PRIMARY KEY,
      version INTEGER NOT NULL DEFAULT 0,
      compiled_at TEXT NOT NULL,
      policy_hash TEXT
    );
  `);
}

function ensureAccessRuleSchema(db: Database.Database) {
  const ruleColumns = db.prepare('PRAGMA table_info(access_rules)').all() as { name: string }[];
  const hasLegacySubject = ruleColumns.some((col) => col.name === 'subject_id');
  const hasEnabled = ruleColumns.some((col) => col.name === 'enabled');

  if (hasLegacySubject) {
    db.exec(`
      CREATE TABLE IF NOT EXISTS access_rules_new (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        resource_id TEXT NOT NULL,
        enabled INTEGER NOT NULL DEFAULT 1,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL,
        FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
      );

      CREATE TABLE IF NOT EXISTS access_rule_groups (
        rule_id TEXT NOT NULL,
        group_id TEXT NOT NULL,
        PRIMARY KEY (rule_id, group_id),
        FOREIGN KEY (rule_id) REFERENCES access_rules_new(id) ON DELETE CASCADE,
        FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE
      );
    `);

    const legacyRows = db.prepare(
      `SELECT id, resource_id, subject_id, subject_type, subject_name, created_at
       FROM access_rules`
    ).all() as {
      id: string;
      resource_id: string;
      subject_id: string;
      subject_type: string;
      subject_name: string;
      created_at: string;
    }[];

    const insertRule = db.prepare(
      'INSERT OR IGNORE INTO access_rules_new (id, name, resource_id, enabled, created_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)'
    );
    const insertRuleGroup = db.prepare(
      'INSERT OR IGNORE INTO access_rule_groups (rule_id, group_id) VALUES (?, ?)'
    );

    const tx = db.transaction(() => {
      legacyRows.forEach((row) => {
        if (row.subject_type !== 'GROUP') return;
        insertRule.run(
          row.id,
          `${row.subject_name} access`,
          row.resource_id,
          row.created_at,
          row.created_at
        );
        insertRuleGroup.run(row.id, row.subject_id);
      });
    });
    tx();

    db.exec('DROP TABLE access_rules');
    db.exec('ALTER TABLE access_rules_new RENAME TO access_rules');
  }

  if (!hasEnabled && !hasLegacySubject) {
    // Fresh schema but missing new fields (defensive).
    db.exec(`ALTER TABLE access_rules ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1`);
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
    'INSERT INTO users (id, name, email, certificate_identity, status, created_at) VALUES (?, ?, ?, ?, ?, ?)'
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
    'INSERT INTO connectors (id, name, status, version, hostname, remote_network_id, last_seen, last_policy_version, last_seen_at, installed) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)'
  );
  const insertTunneler = db.prepare(
    'INSERT INTO tunnelers (id, name, status, version, hostname, remote_network_id) VALUES (?, ?, ?, ?, ?, ?)'
  );
  const insertResource = db.prepare(
    'INSERT INTO resources (id, name, type, address, ports, protocol, port_from, port_to, alias, description, remote_network_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)'
  );
  const insertAccessRule = db.prepare(
    'INSERT INTO access_rules (id, name, resource_id, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)'
  );
  const insertAccessRuleGroup = db.prepare(
    'INSERT INTO access_rule_groups (rule_id, group_id) VALUES (?, ?)'
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
      0,
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
      0,
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
      0,
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
      0,
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
      0,
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
    insertUser.run('usr_1', 'Alice Johnson', 'alice@company.com', 'identity-usr_1', 'active', '2026-01-10');
    insertUser.run('usr_2', 'Bob Smith', 'bob@company.com', 'identity-usr_2', 'active', '2026-01-12');
    insertUser.run('usr_3', 'Charlie Davis', 'charlie@company.com', 'identity-usr_3', 'active', '2026-01-15');
    insertUser.run('usr_4', 'Diana Wilson', 'diana@company.com', 'identity-usr_4', 'inactive', '2026-02-01');

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
      'TCP',
      5432,
      5432,
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
      'TCP',
      443,
      443,
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
      'TCP',
      443,
      443,
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
      'TCP',
      null,
      null,
      null,
      'Internal Confluence Wiki',
      'net_2'
    );

    // Access Rules (group-based)
    insertAccessRule.run('rule_1', 'Engineering DB Access', 'res_1', 1, '2026-01-20', '2026-01-20');
    insertAccessRuleGroup.run('rule_1', 'grp_1');
    insertAccessRule.run('rule_2', 'Engineering API Access', 'res_2', 1, '2026-01-20', '2026-01-20');
    insertAccessRuleGroup.run('rule_2', 'grp_1');

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
    ensureUserCertificateIdentity(dbInstance);
    ensureResourceProtocolColumns(dbInstance);
    ensureConnectorPolicyColumns(dbInstance);
    ensureConnectorPolicyVersionsTable(dbInstance);
    ensureAccessRuleSchema(dbInstance);
    seedIfNeeded(dbInstance);
  }
  return dbInstance;
}

export { DB_PATH };
