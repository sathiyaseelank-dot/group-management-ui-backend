module.exports = [
"[externals]/next/dist/compiled/next-server/app-route-turbo.runtime.dev.js [external] (next/dist/compiled/next-server/app-route-turbo.runtime.dev.js, cjs)", ((__turbopack_context__, module, exports) => {

const mod = __turbopack_context__.x("next/dist/compiled/next-server/app-route-turbo.runtime.dev.js", () => require("next/dist/compiled/next-server/app-route-turbo.runtime.dev.js"));

module.exports = mod;
}),
"[externals]/next/dist/compiled/@opentelemetry/api [external] (next/dist/compiled/@opentelemetry/api, cjs)", ((__turbopack_context__, module, exports) => {

const mod = __turbopack_context__.x("next/dist/compiled/@opentelemetry/api", () => require("next/dist/compiled/@opentelemetry/api"));

module.exports = mod;
}),
"[externals]/next/dist/compiled/next-server/app-page-turbo.runtime.dev.js [external] (next/dist/compiled/next-server/app-page-turbo.runtime.dev.js, cjs)", ((__turbopack_context__, module, exports) => {

const mod = __turbopack_context__.x("next/dist/compiled/next-server/app-page-turbo.runtime.dev.js", () => require("next/dist/compiled/next-server/app-page-turbo.runtime.dev.js"));

module.exports = mod;
}),
"[externals]/next/dist/server/app-render/work-unit-async-storage.external.js [external] (next/dist/server/app-render/work-unit-async-storage.external.js, cjs)", ((__turbopack_context__, module, exports) => {

const mod = __turbopack_context__.x("next/dist/server/app-render/work-unit-async-storage.external.js", () => require("next/dist/server/app-render/work-unit-async-storage.external.js"));

module.exports = mod;
}),
"[externals]/next/dist/server/app-render/work-async-storage.external.js [external] (next/dist/server/app-render/work-async-storage.external.js, cjs)", ((__turbopack_context__, module, exports) => {

const mod = __turbopack_context__.x("next/dist/server/app-render/work-async-storage.external.js", () => require("next/dist/server/app-render/work-async-storage.external.js"));

module.exports = mod;
}),
"[externals]/next/dist/shared/lib/no-fallback-error.external.js [external] (next/dist/shared/lib/no-fallback-error.external.js, cjs)", ((__turbopack_context__, module, exports) => {

const mod = __turbopack_context__.x("next/dist/shared/lib/no-fallback-error.external.js", () => require("next/dist/shared/lib/no-fallback-error.external.js"));

module.exports = mod;
}),
"[externals]/next/dist/server/app-render/after-task-async-storage.external.js [external] (next/dist/server/app-render/after-task-async-storage.external.js, cjs)", ((__turbopack_context__, module, exports) => {

const mod = __turbopack_context__.x("next/dist/server/app-render/after-task-async-storage.external.js", () => require("next/dist/server/app-render/after-task-async-storage.external.js"));

module.exports = mod;
}),
"[externals]/path [external] (path, cjs)", ((__turbopack_context__, module, exports) => {

const mod = __turbopack_context__.x("path", () => require("path"));

module.exports = mod;
}),
"[project]/lib/db.ts [app-route] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "DB_PATH",
    ()=>DB_PATH,
    "getDb",
    ()=>getDb
]);
var __TURBOPACK__imported__module__$5b$externals$5d2f$better$2d$sqlite3__$5b$external$5d$__$28$better$2d$sqlite3$2c$__cjs$2c$__$5b$project$5d2f$node_modules$2f$better$2d$sqlite3$29$__ = __turbopack_context__.i("[externals]/better-sqlite3 [external] (better-sqlite3, cjs, [project]/node_modules/better-sqlite3)");
var __TURBOPACK__imported__module__$5b$externals$5d2f$path__$5b$external$5d$__$28$path$2c$__cjs$29$__ = __turbopack_context__.i("[externals]/path [external] (path, cjs)");
;
;
const DB_PATH = __TURBOPACK__imported__module__$5b$externals$5d2f$path__$5b$external$5d$__$28$path$2c$__cjs$29$__["default"].join(process.cwd(), 'ztna.db');
let dbInstance = null;
function initSchema(db) {
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
function ensureConnectorInstalledColumn(db) {
    const columns = db.prepare('PRAGMA table_info(connectors)').all();
    const hasInstalled = columns.some((col)=>col.name === 'installed');
    if (!hasInstalled) {
        db.exec(`ALTER TABLE connectors ADD COLUMN installed INTEGER NOT NULL DEFAULT 0`);
        db.exec(`UPDATE connectors SET installed = 1`);
    }
}
function seedIfNeeded(db) {
    const seeded = db.prepare('SELECT value FROM meta WHERE key = ?').get('seeded');
    if (seeded?.value === '1') return;
    const insertMeta = db.prepare('INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value');
    const insertUser = db.prepare('INSERT INTO users (id, name, email, status, created_at) VALUES (?, ?, ?, ?, ?)');
    const insertGroup = db.prepare('INSERT INTO groups (id, name, description, created_at) VALUES (?, ?, ?, ?)');
    const insertGroupMember = db.prepare('INSERT INTO group_members (group_id, user_id) VALUES (?, ?)');
    const insertServiceAccount = db.prepare('INSERT INTO service_accounts (id, name, status, associated_resource_count, created_at) VALUES (?, ?, ?, ?, ?)');
    const insertRemoteNetwork = db.prepare('INSERT INTO remote_networks (id, name, location, created_at) VALUES (?, ?, ?, ?)');
    const insertConnector = db.prepare('INSERT INTO connectors (id, name, status, version, hostname, remote_network_id, last_seen, installed) VALUES (?, ?, ?, ?, ?, ?, ?, ?)');
    const insertTunneler = db.prepare('INSERT INTO tunnelers (id, name, status, version, hostname, remote_network_id) VALUES (?, ?, ?, ?, ?, ?)');
    const insertResource = db.prepare('INSERT INTO resources (id, name, type, address, ports, alias, description, remote_network_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)');
    const insertAccessRule = db.prepare('INSERT INTO access_rules (id, resource_id, subject_id, subject_type, subject_name, effect, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)');
    const insertConnectorLog = db.prepare('INSERT INTO connector_logs (connector_id, timestamp, message) VALUES (?, ?, ?)');
    const seedTransaction = db.transaction(()=>{
        // Remote Networks
        insertRemoteNetwork.run('net_1', 'Production AWS', 'AWS', '2026-01-05');
        insertRemoteNetwork.run('net_2', 'Office LAN', 'ON_PREM', '2026-01-08');
        insertRemoteNetwork.run('net_3', 'Staging GCP', 'GCP', '2026-01-15');
        // Connectors
        insertConnector.run('con_1', 'AWS-Prod-Connector-1', 'online', '1.2.0', 'ip-172-31-0-1.ec2.internal', 'net_1', '2026-02-20T10:30:00Z', 1);
        insertConnector.run('con_2', 'AWS-Prod-Connector-2', 'online', '1.2.0', 'ip-172-31-0-2.ec2.internal', 'net_1', '2026-02-20T10:25:00Z', 1);
        insertConnector.run('con_3', 'Office-Connector-1', 'online', '1.1.5', 'office-server.local', 'net_2', '2026-02-20T10:15:00Z', 1);
        insertConnector.run('con_4', 'GCP-Staging-Connector-1', 'online', '1.2.0', 'gcp-staging-vm-1', 'net_3', '2026-02-20T10:05:00Z', 1);
        insertConnector.run('con_5', 'GCP-Staging-Connector-2', 'offline', '1.2.0', 'gcp-staging-vm-2', 'net_3', '2026-02-19T14:00:00Z', 1);
        // Tunnelers
        insertTunneler.run('tun_1', 'AWS-Prod-Tunneler-1', 'online', '1.0.0', 'tun-172-31-0-10.ec2.internal', 'net_1');
        insertTunneler.run('tun_2', 'AWS-Prod-Tunneler-2', 'offline', '1.0.0', 'tun-172-31-0-11.ec2.internal', 'net_1');
        insertTunneler.run('tun_3', 'Office-Tunneler-1', 'online', '1.0.1', 'tun-office-server.local', 'net_2');
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
        insertResource.run('res_1', 'Database Server', 'STANDARD', 'db.internal.company.com:5432', '5432', null, 'Production PostgreSQL database for main application', 'net_1');
        insertResource.run('res_2', 'API Gateway', 'BROWSER', 'api.company.com', '443', null, 'Main API endpoint for frontend applications', 'net_1');
        insertResource.run('res_3', 'S3 Bucket', 'BACKGROUND', 'company-assets.s3.amazonaws.com', '443', null, 'Asset storage bucket', 'net_1');
        insertResource.run('res_4', 'Internal Wiki', 'BROWSER', 'wiki.internal.company.com', '80,443', null, 'Internal Confluence Wiki', 'net_2');
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
function getDb() {
    if (!dbInstance) {
        dbInstance = new __TURBOPACK__imported__module__$5b$externals$5d2f$better$2d$sqlite3__$5b$external$5d$__$28$better$2d$sqlite3$2c$__cjs$2c$__$5b$project$5d2f$node_modules$2f$better$2d$sqlite3$29$__["default"](DB_PATH);
        initSchema(dbInstance);
        ensureConnectorInstalledColumn(dbInstance);
        seedIfNeeded(dbInstance);
    }
    return dbInstance;
}
;
}),
"[project]/lib/data.ts [app-route] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "addConnector",
    ()=>addConnector,
    "addGroup",
    ()=>addGroup,
    "addGroupResources",
    ()=>addGroupResources,
    "addRemoteNetwork",
    ()=>addRemoteNetwork,
    "addResource",
    ()=>addResource,
    "createAccessRule",
    ()=>createAccessRule,
    "deleteAccessRule",
    ()=>deleteAccessRule,
    "getConnectorDetail",
    ()=>getConnectorDetail,
    "getGroupDetail",
    ()=>getGroupDetail,
    "getRemoteNetworkDetail",
    ()=>getRemoteNetworkDetail,
    "getResourceDetail",
    ()=>getResourceDetail,
    "listConnectors",
    ()=>listConnectors,
    "listGroups",
    ()=>listGroups,
    "listRemoteNetworks",
    ()=>listRemoteNetworks,
    "listResources",
    ()=>listResources,
    "listServiceAccounts",
    ()=>listServiceAccounts,
    "listSubjects",
    ()=>listSubjects,
    "listTunnelers",
    ()=>listTunnelers,
    "listUsers",
    ()=>listUsers,
    "removeGroupMember",
    ()=>removeGroupMember,
    "simulateConnectorHeartbeat",
    ()=>simulateConnectorHeartbeat,
    "updateGroupMembers",
    ()=>updateGroupMembers,
    "updateResource",
    ()=>updateResource
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/lib/db.ts [app-route] (ecmascript)");
;
function mapUser(row) {
    return {
        id: row.id,
        name: row.name,
        type: 'USER',
        displayLabel: `User: ${row.name}`,
        email: row.email,
        status: row.status,
        groups: [],
        createdAt: row.created_at
    };
}
function mapGroup(row, memberCount, resourceCount) {
    return {
        id: row.id,
        name: row.name,
        type: 'GROUP',
        displayLabel: `Group: ${row.name}`,
        description: row.description,
        memberCount,
        resourceCount,
        createdAt: row.created_at
    };
}
function mapServiceAccount(row) {
    return {
        id: row.id,
        name: row.name,
        type: 'SERVICE',
        displayLabel: `Service: ${row.name}`,
        status: row.status,
        associatedResourceCount: row.associated_resource_count,
        createdAt: row.created_at
    };
}
function mapResource(row) {
    return {
        id: row.id,
        name: row.name,
        type: row.type,
        address: row.address,
        ports: row.ports,
        alias: row.alias ?? undefined,
        description: row.description,
        remoteNetworkId: row.remote_network_id ?? undefined
    };
}
function mapConnector(row) {
    return {
        id: row.id,
        name: row.name,
        status: row.status,
        version: row.version,
        hostname: row.hostname,
        remoteNetworkId: row.remote_network_id,
        lastSeen: row.last_seen,
        installed: !!row.installed
    };
}
function mapRemoteNetwork(row) {
    return {
        id: row.id,
        name: row.name,
        location: row.location,
        connectorCount: row.connector_count,
        onlineConnectorCount: row.online_connector_count,
        resourceCount: row.resource_count,
        createdAt: row.created_at
    };
}
function mapAccessRule(row) {
    return {
        id: row.id,
        resourceId: row.resource_id,
        subjectId: row.subject_id,
        subjectType: row.subject_type,
        subjectName: row.subject_name,
        effect: row.effect,
        createdAt: row.created_at
    };
}
function listRemoteNetworks() {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    const rows = db.prepare(`
      SELECT n.*,
        (SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id) AS connector_count,
        (SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id AND c.status = 'online') AS online_connector_count,
        (SELECT COUNT(*) FROM resources r WHERE r.remote_network_id = n.id) AS resource_count
      FROM remote_networks n
      ORDER BY n.created_at ASC
      `).all();
    return rows.map(mapRemoteNetwork);
}
function addRemoteNetwork(data) {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    db.prepare('INSERT INTO remote_networks (id, name, location, created_at) VALUES (?, ?, ?, ?)').run(`net_${Date.now()}`, data.name, data.location, new Date().toISOString().split('T')[0]);
}
function getRemoteNetworkDetail(networkId) {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    const networkRow = db.prepare(`
      SELECT n.*,
        (SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id) AS connector_count,
        (SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id AND c.status = 'online') AS online_connector_count,
        (SELECT COUNT(*) FROM resources r WHERE r.remote_network_id = n.id) AS resource_count
      FROM remote_networks n
      WHERE n.id = ?
      `).get(networkId);
    const connectorRows = db.prepare('SELECT * FROM connectors WHERE remote_network_id = ? ORDER BY name ASC').all(networkId);
    const resourceRows = db.prepare('SELECT * FROM resources WHERE remote_network_id = ? ORDER BY name ASC').all(networkId);
    return {
        network: networkRow ? mapRemoteNetwork(networkRow) : undefined,
        connectors: connectorRows.map(mapConnector),
        resources: resourceRows.map(mapResource)
    };
}
function listConnectors() {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    const rows = db.prepare('SELECT * FROM connectors ORDER BY name ASC').all();
    return rows.map(mapConnector);
}
function getConnectorDetail(connectorId) {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    const connectorRow = db.prepare('SELECT * FROM connectors WHERE id = ?').get(connectorId);
    if (!connectorRow) {
        return {
            connector: null,
            network: undefined,
            logs: []
        };
    }
    const networkRow = db.prepare(`
      SELECT n.*,
        (SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id) AS connector_count,
        (SELECT COUNT(*) FROM connectors c WHERE c.remote_network_id = n.id AND c.status = 'online') AS online_connector_count,
        (SELECT COUNT(*) FROM resources r WHERE r.remote_network_id = n.id) AS resource_count
      FROM remote_networks n
      WHERE n.id = ?
      `).get(connectorRow.remote_network_id);
    const logs = db.prepare('SELECT id, timestamp, message FROM connector_logs WHERE connector_id = ? ORDER BY id ASC').all(connectorId);
    return {
        connector: mapConnector(connectorRow),
        network: networkRow ? mapRemoteNetwork(networkRow) : undefined,
        logs
    };
}
function addConnector(data) {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    const id = `con_${Date.now()}`;
    const hostname = `${data.name.toLowerCase().replace(/\s/g, '-')}.local`;
    db.prepare(`INSERT INTO connectors (id, name, status, version, hostname, remote_network_id, last_seen, installed)
     VALUES (?, ?, 'offline', '1.0.0', ?, ?, ?, 0)`).run(id, data.name, hostname, data.remoteNetworkId, new Date().toISOString());
}
function simulateConnectorHeartbeat(connectorId) {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    db.prepare('UPDATE connectors SET status = ?, last_seen = ?, installed = 1 WHERE id = ?').run('online', new Date().toISOString(), connectorId);
}
function listTunnelers() {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    const rows = db.prepare('SELECT * FROM tunnelers ORDER BY name ASC').all();
    return rows.map((row)=>({
            id: row.id,
            name: row.name,
            status: row.status,
            version: row.version,
            hostname: row.hostname,
            remoteNetworkId: row.remote_network_id
        }));
}
function listUsers() {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    const userRows = db.prepare('SELECT * FROM users ORDER BY name ASC').all();
    const userGroups = db.prepare('SELECT group_id FROM group_members WHERE user_id = ?');
    return userRows.map((row)=>{
        const groups = userGroups.all(row.id).map((g)=>g.group_id);
        return {
            ...mapUser(row),
            groups
        };
    });
}
function listServiceAccounts() {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    const rows = db.prepare('SELECT * FROM service_accounts ORDER BY name ASC').all();
    return rows.map(mapServiceAccount);
}
function listGroups() {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    const rows = db.prepare('SELECT * FROM groups ORDER BY name ASC').all();
    const memberCountStmt = db.prepare('SELECT COUNT(*) as count FROM group_members WHERE group_id = ?');
    const resourceCountStmt = db.prepare("SELECT COUNT(*) as count FROM access_rules WHERE subject_type = 'GROUP' AND subject_id = ?");
    return rows.map((row)=>{
        const memberCount = memberCountStmt.get(row.id).count;
        const resourceCount = resourceCountStmt.get(row.id).count;
        return mapGroup(row, memberCount, resourceCount);
    });
}
function getGroupDetail(groupId) {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    const groupRow = db.prepare('SELECT * FROM groups WHERE id = ?').get(groupId);
    if (!groupRow) {
        return {
            group: undefined,
            members: [],
            resources: []
        };
    }
    const memberRows = db.prepare(`
    SELECT u.id as userId, u.name as userName, u.email as email
    FROM group_members gm
    JOIN users u ON u.id = gm.user_id
    WHERE gm.group_id = ?
    ORDER BY u.name ASC
    `).all(groupId);
    const resourceRows = db.prepare(`
    SELECT r.*
    FROM access_rules ar
    JOIN resources r ON r.id = ar.resource_id
    WHERE ar.subject_type = 'GROUP' AND ar.subject_id = ?
    GROUP BY r.id
    ORDER BY r.name ASC
    `).all(groupId);
    const memberCount = memberRows.length;
    const resourceCount = resourceRows.length;
    return {
        group: mapGroup(groupRow, memberCount, resourceCount),
        members: memberRows,
        resources: resourceRows.map(mapResource)
    };
}
function addGroup(data) {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    db.prepare('INSERT INTO groups (id, name, description, created_at) VALUES (?, ?, ?, ?)').run(`grp_${Date.now()}`, data.name, data.description, new Date().toISOString().split('T')[0]);
}
function updateGroupMembers(groupId, memberIds) {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    const deleteStmt = db.prepare('DELETE FROM group_members WHERE group_id = ?');
    const insertStmt = db.prepare('INSERT INTO group_members (group_id, user_id) VALUES (?, ?)');
    const tx = db.transaction(()=>{
        deleteStmt.run(groupId);
        memberIds.forEach((id)=>insertStmt.run(groupId, id));
    });
    tx();
}
function removeGroupMember(groupId, userId) {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    db.prepare('DELETE FROM group_members WHERE group_id = ? AND user_id = ?').run(groupId, userId);
}
function addGroupResources(groupId, resourceIds) {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    const existingStmt = db.prepare("SELECT id FROM access_rules WHERE subject_type = 'GROUP' AND subject_id = ? AND resource_id = ?");
    const insertStmt = db.prepare(`INSERT INTO access_rules (id, resource_id, subject_id, subject_type, subject_name, effect, created_at)
     VALUES (?, ?, ?, 'GROUP', ?, 'ALLOW', ?)`);
    const groupNameRow = db.prepare('SELECT name FROM groups WHERE id = ?').get(groupId);
    const groupName = groupNameRow?.name ?? 'Unknown Group';
    const tx = db.transaction(()=>{
        resourceIds.forEach((resourceId)=>{
            const existing = existingStmt.get(groupId, resourceId);
            if (!existing) {
                insertStmt.run(`rule_${Date.now()}_${groupId}_${resourceId}`, resourceId, groupId, groupName, new Date().toISOString().split('T')[0]);
            }
        });
    });
    tx();
}
function listResources() {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    const rows = db.prepare('SELECT * FROM resources ORDER BY name ASC').all();
    return rows.map(mapResource);
}
function getResourceDetail(resourceId) {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    const resourceRow = db.prepare('SELECT * FROM resources WHERE id = ?').get(resourceId);
    const accessRuleRows = db.prepare('SELECT * FROM access_rules WHERE resource_id = ? ORDER BY created_at ASC').all(resourceId);
    return {
        resource: resourceRow ? mapResource(resourceRow) : undefined,
        accessRules: accessRuleRows.map(mapAccessRule)
    };
}
function addResource(data) {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    db.prepare(`INSERT INTO resources (id, name, type, address, ports, alias, description, remote_network_id)
     VALUES (?, ?, ?, ?, ?, ?, ?, ?)`).run(`res_${Date.now()}`, data.name, data.type, data.address, data.ports, data.alias ?? null, `A new ${data.type.toLowerCase()} resource`, data.network_id);
}
function updateResource(resourceId, data) {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    db.prepare(`UPDATE resources
     SET name = ?, type = ?, address = ?, ports = ?, alias = ?, remote_network_id = ?
     WHERE id = ?`).run(data.name, data.type, data.address, data.ports, data.alias ?? null, data.network_id, resourceId);
}
function resolveSubjectName(subject) {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
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
function createAccessRule(resourceId, subjects, effect) {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    const insertStmt = db.prepare(`INSERT INTO access_rules (id, resource_id, subject_id, subject_type, subject_name, effect, created_at)
     VALUES (?, ?, ?, ?, ?, ?, ?)`);
    const tx = db.transaction(()=>{
        subjects.forEach((subject)=>{
            const subjectName = resolveSubjectName(subject);
            insertStmt.run(`rule_${Date.now()}_${subject.id}`, resourceId, subject.id, subject.type, subjectName, effect, new Date().toISOString().split('T')[0]);
        });
    });
    tx();
}
function deleteAccessRule(ruleId) {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    db.prepare('DELETE FROM access_rules WHERE id = ?').run(ruleId);
}
function listSubjects(type) {
    const db = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$db$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["getDb"])();
    const subjects = [];
    if (!type || type === 'USER') {
        const rows = db.prepare('SELECT * FROM users ORDER BY name ASC').all();
        subjects.push(...rows.map((row)=>({
                id: row.id,
                name: row.name,
                type: 'USER',
                displayLabel: `User: ${row.name}`
            })));
    }
    if (!type || type === 'GROUP') {
        const rows = db.prepare('SELECT * FROM groups ORDER BY name ASC').all();
        subjects.push(...rows.map((row)=>({
                id: row.id,
                name: row.name,
                type: 'GROUP',
                displayLabel: `Group: ${row.name}`
            })));
    }
    if (!type || type === 'SERVICE') {
        const rows = db.prepare('SELECT * FROM service_accounts ORDER BY name ASC').all();
        subjects.push(...rows.map((row)=>({
                id: row.id,
                name: row.name,
                type: 'SERVICE',
                displayLabel: `Service: ${row.name}`
            })));
    }
    return subjects;
}
}),
"[project]/app/api/subjects/route.ts [app-route] (ecmascript)", ((__turbopack_context__) => {
"use strict";

__turbopack_context__.s([
    "GET",
    ()=>GET,
    "runtime",
    ()=>runtime
]);
var __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f$next$2f$server$2e$js__$5b$app$2d$route$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/node_modules/next/server.js [app-route] (ecmascript)");
var __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$data$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__ = __turbopack_context__.i("[project]/lib/data.ts [app-route] (ecmascript)");
;
;
const runtime = 'nodejs';
async function GET(req) {
    const url = new URL(req.url);
    const typeParam = url.searchParams.get('type');
    const upper = typeParam?.toUpperCase();
    const type = upper === 'USER' || upper === 'GROUP' || upper === 'SERVICE' ? upper : undefined;
    const subjects = (0, __TURBOPACK__imported__module__$5b$project$5d2f$lib$2f$data$2e$ts__$5b$app$2d$route$5d$__$28$ecmascript$29$__["listSubjects"])(type);
    return __TURBOPACK__imported__module__$5b$project$5d2f$node_modules$2f$next$2f$server$2e$js__$5b$app$2d$route$5d$__$28$ecmascript$29$__["NextResponse"].json(subjects);
}
}),
];

//# sourceMappingURL=%5Broot-of-the-server%5D__fbe9e087._.js.map