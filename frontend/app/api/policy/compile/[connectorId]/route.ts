import { NextResponse } from 'next/server';
import crypto from 'crypto';
import { getDb } from '@/lib/db';

export const runtime = 'nodejs';

function computeHash(payload: unknown) {
  return crypto.createHash('sha256').update(JSON.stringify(payload)).digest('hex');
}

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ connectorId: string }> }
) {
  const { connectorId } = await params;
  const db = getDb();

  const connector = db.prepare('SELECT id, remote_network_id FROM connectors WHERE id = ?').get(connectorId) as {
    id: string;
    remote_network_id: string;
  } | undefined;

  if (!connector) {
    return NextResponse.json({ error: 'connector not found' }, { status: 404 });
  }

  const resourceRows = db.prepare(
    `SELECT id, address, protocol, port_from, port_to
     FROM resources
     WHERE remote_network_id = ?
     ORDER BY id ASC`
  ).all(connector.remote_network_id) as {
    id: string;
    address: string;
    protocol: string;
    port_from: number | null;
    port_to: number | null;
  }[];

  const identityStmt = db.prepare(
    `SELECT DISTINCT u.certificate_identity as identity
     FROM access_rules ar
     JOIN access_rule_groups arg ON arg.rule_id = ar.id
     JOIN group_members gm ON gm.group_id = arg.group_id
     JOIN users u ON u.id = gm.user_id
     WHERE ar.resource_id = ? AND ar.enabled = 1 AND u.certificate_identity IS NOT NULL
     ORDER BY u.certificate_identity ASC`
  );

  const resources = resourceRows.map((row) => {
    const identities = identityStmt.all(row.id).map((r: any) => r.identity).filter(Boolean);
    return {
      resource_id: row.id,
      address: row.address,
      protocol: row.protocol ?? 'TCP',
      port_from: row.port_from ?? null,
      port_to: row.port_to ?? null,
      allowed_identities: identities,
      _note: 'empty list = deny all - fail closed',
    };
  });

  const policyHash = computeHash({ resources });
  const now = new Date().toISOString();

  const versionRow = db.prepare(
    'SELECT version, policy_hash FROM connector_policy_versions WHERE connector_id = ?'
  ).get(connectorId) as { version?: number; policy_hash?: string } | undefined;

  let version = versionRow?.version ?? 0;
  if (!versionRow || versionRow.policy_hash !== policyHash) {
    version = version + 1;
  }

  db.prepare(
    `INSERT INTO connector_policy_versions (connector_id, version, compiled_at, policy_hash)
     VALUES (?, ?, ?, ?)
     ON CONFLICT(connector_id) DO UPDATE SET version=excluded.version, compiled_at=excluded.compiled_at, policy_hash=excluded.policy_hash`
  ).run(connectorId, version, now, policyHash);

  return NextResponse.json({
    snapshot_meta: {
      connector_id: connectorId,
      policy_version: version,
      compiled_at: now,
      policy_hash: policyHash,
    },
    resources,
  });
}
