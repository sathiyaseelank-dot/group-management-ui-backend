export type Platform = 'Windows' | 'macOS' | 'iOS' | 'Linux' | 'Android';

export interface DeviceProfilesState {
  trustedProfiles: Platform[];
  approvedOS: Record<Platform, boolean>;
}

const STORAGE_KEY = 'gm_device_profiles_v1';

const PLATFORMS: Platform[] = ['Windows', 'macOS', 'iOS', 'Linux', 'Android'];

const DEFAULT_STATE: DeviceProfilesState = {
  trustedProfiles: [],
  approvedOS: {
    Windows: true,
    macOS: true,
    iOS: true,
    Linux: true,
    Android: true,
  },
};

function read(): DeviceProfilesState {
  if (typeof window === 'undefined') return DEFAULT_STATE;
  const raw = window.localStorage.getItem(STORAGE_KEY);
  if (!raw) return DEFAULT_STATE;
  try {
    const parsed = JSON.parse(raw) as Partial<DeviceProfilesState>;
    const trusted = Array.isArray(parsed.trustedProfiles) ? parsed.trustedProfiles : [];
    const approved = (parsed.approvedOS ?? {}) as Record<string, any>;
    const approvedOS = Object.fromEntries(
      PLATFORMS.map((p) => [p, Boolean(approved[p])]),
    ) as Record<Platform, boolean>;
    const trustedProfiles = trusted.filter((p): p is Platform => PLATFORMS.includes(p as Platform));
    return { trustedProfiles, approvedOS };
  } catch {
    return DEFAULT_STATE;
  }
}

function write(state: DeviceProfilesState) {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
}

export function getDeviceProfiles(): DeviceProfilesState {
  return read();
}

export function addTrustedProfile(platform: Platform) {
  const state = read();
  if (state.trustedProfiles.includes(platform)) return;
  write({ ...state, trustedProfiles: [platform, ...state.trustedProfiles] });
}

export function setApprovedOS(platform: Platform, enabled: boolean) {
  const state = read();
  write({ ...state, approvedOS: { ...state.approvedOS, [platform]: enabled } });
}

export const ALL_PLATFORMS = PLATFORMS;

