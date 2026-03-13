import { Navigate, Route, Routes, useNavigate, useLocation } from 'react-router-dom'
import { useEffect, useState, type ReactNode } from 'react'
import DashboardLayout from './pages/DashboardLayout'
import LoginPage from './pages/Login'
import GroupsPage from './pages/groups/GroupsPage'
import GroupDetailPage from './pages/groups/GroupDetailPage'
import UsersPage from './pages/users/UsersPage'
import ResourcesPage from './pages/resources/ResourcesPage'
import ResourceDetailPage from './pages/resources/ResourceDetailPage'
import ConnectorsPage from './pages/connectors/ConnectorsPage'
import ConnectorDetailPage from './pages/connectors/ConnectorDetailPage'
import RemoteNetworksPage from './pages/remote-networks/RemoteNetworksPage'
import NetworkDetailPage from './pages/remote-networks/NetworkDetailPage'
import AgentsPage from './pages/agents/AgentsPage'
import NewAgentPage from './pages/agents/NewAgentPage'
import AgentDetailPage from './pages/agents/AgentDetailPage'
import PolicyLayout from './pages/policy/PolicyLayout'
import ResourcePoliciesPage from './pages/policy/ResourcePoliciesPage'
import ResourcePolicyDetailPage from './pages/policy/ResourcePolicyDetailPage'
import SignInPolicyPage from './pages/policy/SignInPolicyPage'
import DeviceProfilesPage from './pages/policy/DeviceProfilesPage'
import AuditLogsPage from './pages/AuditLogsPage'
import NetworkDiagnosticsPage from './pages/diagnostics/NetworkDiagnosticsPage'
import NetworkDiscoveryPage from './pages/resources/NetworkDiscoveryPage'
import WorkspaceSelectorPage from './pages/workspaces/WorkspaceSelectorPage'
import WorkspaceCreatePage from './pages/workspaces/WorkspaceCreatePage'
import WorkspaceSettingsPage from './pages/workspaces/WorkspaceSettingsPage'
import SignupLayout from './pages/signup/SignupLayout'
import SignupPage from './pages/signup/SignupPage'
import SignupCustomizePage from './pages/signup/SignupCustomizePage'
import SignupFinalizePage from './pages/signup/SignupFinalizePage'
import SignupAuthPage from './pages/signup/SignupAuthPage'
import UserLayout from './pages/app/UserLayout'
import UserHomePage from './pages/app/UserHomePage'
import WelcomePage from './pages/app/WelcomePage'
import InstallPage from './pages/app/InstallPage'
import { STORAGE_KEY as SIGNUP_STORAGE_KEY } from './contexts/SignupContext'
import { getWorkspaceClaims, isDeviceToken, getAudience } from '@/lib/jwt'
import IdentityProvidersPage from './pages/settings/IdentityProvidersPage'
import SessionsPage from './pages/settings/SessionsPage'

const API_BASE = import.meta.env.VITE_API_BASE_URL || '/api'
const SIGNUP_PROCESS_PREFIX = 'ztna_signup_processed:'

// Captures ?token= from OAuth redirect at any route, stores it, then redirects.
// If signup state exists in sessionStorage, auto-creates the workspace.
function TokenCapture() {
  const navigate = useNavigate()
  const [processing, setProcessing] = useState(false)
  const [error, setError] = useState('')

  const params = new URLSearchParams(window.location.search)
  const token = params.get('token')

  useEffect(() => {
    if (!token) return

    localStorage.setItem('authToken', token)

    // Check for workspace data: first from URL params (passed through OAuth state),
    // then from sessionStorage (same-origin fallback).
    const wsName = params.get('ws_name')
    const wsSlug = params.get('ws_slug')
    const signupId = params.get('signup_id')

    let networkName = wsName || ''
    let networkSlug = wsSlug || ''
    let latestSignupId = signupId || ''

    if (!networkName || !networkSlug) {
      // Try sessionStorage as fallback (works when same origin)
      const raw = sessionStorage.getItem(SIGNUP_STORAGE_KEY)
      if (raw) {
        try {
          const signupState = JSON.parse(raw)
          networkName = networkName || signupState.networkName || ''
          networkSlug = networkSlug || signupState.networkSlug || ''
          latestSignupId = latestSignupId || signupState.attemptId || ''
        } catch { /* ignore */ }
      }
    } else if (!latestSignupId) {
      const raw = sessionStorage.getItem(SIGNUP_STORAGE_KEY)
      if (raw) {
        try {
          const signupState = JSON.parse(raw)
          latestSignupId = signupState.attemptId || ''
        } catch { /* ignore */ }
      }
    }

    if (signupId && latestSignupId && signupId !== latestSignupId) {
      sessionStorage.removeItem(SIGNUP_STORAGE_KEY)
      navigate('/signup/customize', { replace: true })
      return
    }

    if (!networkName || !networkSlug) {
      sessionStorage.removeItem(SIGNUP_STORAGE_KEY)

      const aud = getAudience(token)
      const claims = getWorkspaceClaims(token)

      if (aud === 'device') {
        navigate('/app', { replace: true })
      } else if (aud === 'admin' && claims) {
        if (claims.wrole === 'member') {
          navigate('/app', { replace: true })
        } else {
          navigate('/dashboard/groups', { replace: true })
        }
      } else if (!claims) {
        navigate('/workspaces', { replace: true })
      } else if (claims.wrole === 'admin' || claims.wrole === 'owner') {
        navigate('/dashboard/groups', { replace: true })
      } else {
        navigate('/app', { replace: true })
      }
      return
    }

    // Signup flow: create workspace automatically
    const processedKey = `${SIGNUP_PROCESS_PREFIX}${latestSignupId || `${networkSlug}:${token}`}`
    const processedState = sessionStorage.getItem(processedKey)
    if (processedState === 'pending') {
      setProcessing(true)
      return
    }
    if (processedState === 'done') {
      sessionStorage.removeItem(SIGNUP_STORAGE_KEY)
      navigate('/dashboard/groups?setup=true', { replace: true })
      return
    }
    sessionStorage.setItem(processedKey, 'pending')
    setProcessing(true)

    fetch(`${API_BASE}/workspaces`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify({ name: networkName, slug: networkSlug }),
    })
      .then(async res => {
        if (!res.ok) {
          const text = await res.text()
          if (res.status === 409) {
            sessionStorage.removeItem(processedKey)
            setError('Network URL already taken. Please choose another.')
            navigate('/signup/customize', { replace: true })
            return
          }
          throw new Error(text || 'Failed to create workspace')
        }
        return res.json()
      })
      .then(data => {
        if (!data) return
        if (data.token) {
          localStorage.setItem('authToken', data.token)
        }
        sessionStorage.setItem(processedKey, 'done')
        sessionStorage.removeItem(SIGNUP_STORAGE_KEY)
        navigate('/dashboard/groups?setup=true', { replace: true })
      })
      .catch(err => {
        sessionStorage.removeItem(processedKey)
        setError(err.message)
        setProcessing(false)
      })
  }, [token, navigate])

  if (!token) {
    return <Navigate to="/workspaces" replace />
  }

  if (error) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <div className="w-full max-w-md space-y-4 rounded-xl border bg-card p-8 shadow-sm">
          <p className="text-sm text-destructive">{error}</p>
          <button
            className="text-sm text-primary hover:underline"
            onClick={() => navigate('/workspaces', { replace: true })}
          >
            Go to workspaces
          </button>
        </div>
      </div>
    )
  }

  if (processing) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <p className="text-sm text-muted-foreground">Setting up your network...</p>
      </div>
    )
  }

  return null
}

function AuthGuard({ children }: { children: ReactNode }) {
  const navigate = useNavigate()
  const location = useLocation()

  useEffect(() => {
    const token = localStorage.getItem('authToken')
    if (!token) {
      navigate('/login', { replace: true })
      return
    }
    // Device tokens cannot access admin dashboard
    if (isDeviceToken(token) && location.pathname.startsWith('/dashboard')) {
      navigate('/app', { replace: true })
      return
    }
    const claims = getWorkspaceClaims(token)
    if (!claims && location.pathname.startsWith('/dashboard')) {
      navigate('/workspaces', { replace: true })
      return
    }
    if (claims && claims.wrole === 'member' && location.pathname.startsWith('/dashboard')) {
      navigate('/app', { replace: true })
    }
  }, [navigate, location.pathname])

  return <>{children}</>
}

function UserAuthGuard({ children }: { children: ReactNode }) {
  const navigate = useNavigate()

  useEffect(() => {
    const token = localStorage.getItem('authToken')
    if (!token) {
      navigate('/login', { replace: true })
      return
    }
    const claims = getWorkspaceClaims(token)
    if (!claims) {
      navigate('/workspaces', { replace: true })
    }
  }, [navigate])

  return <>{children}</>
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/" element={<TokenCapture />} />
      <Route path="/signup" element={<SignupLayout />}>
        <Route index element={<SignupPage />} />
        <Route path="customize" element={<SignupCustomizePage />} />
        <Route path="finalize" element={<SignupFinalizePage />} />
        <Route path="auth" element={<SignupAuthPage />} />
      </Route>
      <Route path="/workspaces" element={<WorkspaceSelectorPage />} />
      <Route path="/workspaces/create" element={<WorkspaceCreatePage />} />
      <Route path="/app" element={<UserAuthGuard><UserLayout /></UserAuthGuard>}>
        <Route index element={<UserHomePage />} />
        <Route path="welcome" element={<WelcomePage />} />
        <Route path="install" element={<InstallPage />} />
      </Route>
      <Route path="/dashboard" element={<AuthGuard><DashboardLayout /></AuthGuard>}>
        <Route index element={<Navigate to="groups" replace />} />
        <Route path="groups" element={<GroupsPage />} />
        <Route path="groups/:groupId" element={<GroupDetailPage />} />
        <Route path="users" element={<UsersPage />} />
        <Route path="resources" element={<ResourcesPage />} />
        <Route path="resources/:resourceId" element={<ResourceDetailPage />} />
        <Route path="connectors" element={<ConnectorsPage />} />
        <Route path="connectors/:connectorId" element={<ConnectorDetailPage />} />
        <Route path="remote-networks" element={<RemoteNetworksPage />} />
        <Route path="remote-networks/:networkId" element={<NetworkDetailPage />} />
        <Route path="agents" element={<AgentsPage />} />
        <Route path="agents/new" element={<NewAgentPage />} />
        <Route path="agents/:agentId" element={<AgentDetailPage />} />
        <Route path="policy" element={<PolicyLayout />}>
          <Route index element={<Navigate to="resource-policies" replace />} />
          <Route path="resource-policies" element={<ResourcePoliciesPage />} />
          <Route path="resource-policies/:policyId" element={<ResourcePolicyDetailPage />} />
          <Route path="sign-in" element={<SignInPolicyPage />} />
          <Route path="device-profiles" element={<DeviceProfilesPage />} />
        </Route>
        <Route path="discovery" element={<NetworkDiscoveryPage />} />
        <Route path="audit-logs" element={<AuditLogsPage />} />
        <Route path="diagnostics" element={<NetworkDiagnosticsPage />} />
        <Route path="workspace/settings" element={<WorkspaceSettingsPage />} />
        <Route path="workspace/settings/identity-providers" element={<IdentityProvidersPage />} />
        <Route path="workspace/settings/sessions" element={<SessionsPage />} />
      </Route>
    </Routes>
  )
}
