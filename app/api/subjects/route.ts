import { NextResponse } from 'next/server';
import { listSubjects } from '@/lib/data';
import { SubjectType } from '@/lib/types';

export const runtime = 'nodejs';

export async function GET(req: Request) {
  const url = new URL(req.url);
  const typeParam = url.searchParams.get('type');
  const upper = typeParam?.toUpperCase();
  const type =
    upper === 'USER' || upper === 'GROUP' || upper === 'SERVICE'
      ? (upper as SubjectType)
      : undefined;
  const subjects = listSubjects(type);
  return NextResponse.json(subjects);
}
