'use client';

export interface SignInPolicyState {
  reauth: {
    days: number;
    hours: number;
  };
  mfa: {
    required: boolean;
  };
}

const STORAGE_KEY = 'gm_sign_in_policy_v1';

const DEFAULT_STATE: SignInPolicyState = {
  reauth: { days: 30, hours: 0 },
  mfa: { required: false },
};

function read(): SignInPolicyState {
  if (typeof window === 'undefined') return DEFAULT_STATE;
  const raw = window.localStorage.getItem(STORAGE_KEY);
  if (!raw) return DEFAULT_STATE;
  try {
    const parsed = JSON.parse(raw) as Partial<SignInPolicyState>;
    const days = Number((parsed.reauth as any)?.days);
    const hours = Number((parsed.reauth as any)?.hours);
    const required = Boolean((parsed.mfa as any)?.required);
    return {
      reauth: {
        days: Number.isFinite(days) && days >= 0 ? days : 30,
        hours: Number.isFinite(hours) && hours >= 0 ? hours : 0,
      },
      mfa: { required },
    };
  } catch {
    return DEFAULT_STATE;
  }
}

function write(state: SignInPolicyState) {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
}

export function getSignInPolicy(): SignInPolicyState {
  return read();
}

export function saveReauth(days: number, hours: number) {
  const state = read();
  write({ ...state, reauth: { days, hours } });
}

export function saveMfaRequired(required: boolean) {
  const state = read();
  write({ ...state, mfa: { required } });
}

