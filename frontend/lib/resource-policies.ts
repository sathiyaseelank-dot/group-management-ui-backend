'use client';

export type DeviceSecurityMode = 'any' | 'trusted' | 'custom';

export interface ResourcePolicy {
  id: string;
  name: string;
  isDefault: boolean;
  deviceSecurity: {
    mode: DeviceSecurityMode;
  };
}

const STORAGE_KEY = 'gm_resource_policies_v1';
const DEFAULT_ID = 'default';

function safeParse(json: string | null): ResourcePolicy[] {
  if (!json) return [];
  try {
    const parsed = JSON.parse(json) as unknown;
    if (!Array.isArray(parsed)) return [];
    return parsed as ResourcePolicy[];
  } catch {
    return [];
  }
}

function readAll(): ResourcePolicy[] {
  if (typeof window === 'undefined') return [];
  return safeParse(window.localStorage.getItem(STORAGE_KEY));
}

function writeAll(policies: ResourcePolicy[]) {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(policies));
}

function newId(prefix: string) {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return `${prefix}_${crypto.randomUUID()}`;
  }
  return `${prefix}_${Date.now()}_${Math.random().toString(16).slice(2)}`;
}

export function ensureDefaultResourcePolicy() {
  const policies = readAll();
  const hasDefault = policies.some((p) => p.id === DEFAULT_ID);
  if (hasDefault) return;
  const seeded: ResourcePolicy[] = [
    {
      id: DEFAULT_ID,
      name: 'Default Policy',
      isDefault: true,
      deviceSecurity: { mode: 'any' },
    },
    ...policies,
  ];
  writeAll(seeded);
}

export function listResourcePolicies(): ResourcePolicy[] {
  ensureDefaultResourcePolicy();
  return readAll();
}

export function getResourcePolicy(id: string): ResourcePolicy | null {
  const policies = listResourcePolicies();
  return policies.find((p) => p.id === id) ?? null;
}

export function createResourcePolicy(name: string): ResourcePolicy {
  const trimmed = name.trim();
  if (!trimmed) throw new Error('Policy name is required.');
  const policies = listResourcePolicies();
  const exists = policies.some((p) => p.name.toLowerCase() === trimmed.toLowerCase());
  if (exists) throw new Error('A policy with this name already exists.');
  const policy: ResourcePolicy = {
    id: newId('pol'),
    name: trimmed,
    isDefault: false,
    deviceSecurity: { mode: 'any' },
  };
  writeAll([policy, ...policies]);
  return policy;
}

export function saveResourcePolicy(policy: ResourcePolicy) {
  const policies = listResourcePolicies();
  const next = policies.map((p) => (p.id === policy.id ? policy : p));
  writeAll(next);
}

export function deleteResourcePolicy(id: string) {
  if (id === DEFAULT_ID) return;
  const policies = listResourcePolicies();
  writeAll(policies.filter((p) => p.id !== id));
}

