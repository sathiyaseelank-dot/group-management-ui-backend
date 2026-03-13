import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react'

export interface SignupState {
  email: string
  useCase: 'home' | 'work' | ''
  networkName: string
  networkSlug: string
  attemptId: string
  teamSize: string // "just-me" | "2-10" | "11-50" | "51-200" | "200+"
}

const STORAGE_KEY = 'ztna_signup_state'

const defaultState: SignupState = {
  email: '',
  useCase: '',
  networkName: '',
  networkSlug: '',
  attemptId: '',
  teamSize: '',
}

function loadState(): SignupState {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY)
    if (raw) return { ...defaultState, ...JSON.parse(raw) }
  } catch { /* ignore */ }
  return { ...defaultState }
}

interface SignupContextValue {
  state: SignupState
  updateState: (partial: Partial<SignupState>) => void
  clearState: () => void
}

const SignupContext = createContext<SignupContextValue | null>(null)

export function SignupProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<SignupState>(loadState)

  useEffect(() => {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(state))
  }, [state])

  const updateState = useCallback((partial: Partial<SignupState>) => {
    setState(prev => ({ ...prev, ...partial }))
  }, [])

  const clearState = useCallback(() => {
    sessionStorage.removeItem(STORAGE_KEY)
    setState({ ...defaultState })
  }, [])

  return (
    <SignupContext.Provider value={{ state, updateState, clearState }}>
      {children}
    </SignupContext.Provider>
  )
}

export function useSignup() {
  const ctx = useContext(SignupContext)
  if (!ctx) throw new Error('useSignup must be used within SignupProvider')
  return ctx
}

export { STORAGE_KEY }
