import { NextResponse } from 'next/server';
import { getDb } from '@/lib/db';

export const runtime = 'nodejs';

export async function GET(
  _req: Request,
  { params }: { params: Promise<{ ruleId: string }> }
) {
  const { ruleId } = await params;
  const db = getDb();

  const row = db.prepare(
    `SELECT COUNT(DISTINCT u.id) as count
     FROM access_rule_groups arg
     JOIN group_members gm ON gm.group_id = arg.group_id
     JOIN users u ON u.id = gm.user_id
     WHERE arg.rule_id = ? AND u.certificate_identity IS NOT NULL`
  ).get(ruleId) as { count?: number } | undefined;

  return NextResponse.json({ count: row?.count ?? 0 });
}
