export type Platform = 'Windows' | 'macOS' | 'iOS' | 'Linux' | 'Android';

export type { TrustedProfile } from './types';

export interface DeviceProfilesState {
  approvedOS: Record<Platform, boolean>;
}

const STORAGE_KEY = 'gm_device_profiles_v1';

const PLATFORMS: Platform[] = ['Windows', 'macOS', 'iOS', 'Linux', 'Android'];

const DEFAULT_APPROVED_OS: Record<Platform, boolean> = {
  Windows: true,
  macOS: true,
  iOS: true,
  Linux: true,
  Android: true,
};

function readApprovedOS(): Record<Platform, boolean> {
  if (typeof window === 'undefined') return DEFAULT_APPROVED_OS;
  const raw = window.localStorage.getItem(STORAGE_KEY);
  if (!raw) return DEFAULT_APPROVED_OS;
  try {
    const parsed = JSON.parse(raw) as { approvedOS?: Record<string, any> };
    const approved = (parsed.approvedOS ?? {}) as Record<string, any>;
    return Object.fromEntries(
      PLATFORMS.map((p) => [p, Boolean(approved[p])]),
    ) as Record<Platform, boolean>;
  } catch {
    return DEFAULT_APPROVED_OS;
  }
}

function writeApprovedOS(approvedOS: Record<Platform, boolean>) {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify({ approvedOS }));
}

export function getDeviceProfiles(): DeviceProfilesState {
  return { approvedOS: readApprovedOS() };
}

export function setApprovedOS(platform: Platform, enabled: boolean) {
  const current = readApprovedOS();
  writeApprovedOS({ ...current, [platform]: enabled });
}

export const ALL_PLATFORMS = PLATFORMS;
