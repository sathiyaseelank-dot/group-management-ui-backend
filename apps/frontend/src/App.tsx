import { Navigate, Route, Routes, useNavigate, useLocation } from 'react-router-dom'
import { useEffect, type ReactNode } from 'react'
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
import TunnelersPage from './pages/tunnelers/TunnelersPage'
import NewTunnelerPage from './pages/tunnelers/NewTunnelerPage'
import TunnelerDetailPage from './pages/tunnelers/TunnelerDetailPage'
import PolicyLayout from './pages/policy/PolicyLayout'
import ResourcePoliciesPage from './pages/policy/ResourcePoliciesPage'
import ResourcePolicyDetailPage from './pages/policy/ResourcePolicyDetailPage'
import SignInPolicyPage from './pages/policy/SignInPolicyPage'
import DeviceProfilesPage from './pages/policy/DeviceProfilesPage'
import AuditLogsPage from './pages/AuditLogsPage'
import NetworkDiscoveryPage from './pages/resources/NetworkDiscoveryPage'

// Captures ?token= from OAuth redirect at any route, stores it, then redirects to dashboard.
function TokenCapture() {
  const params = new URLSearchParams(window.location.search)
  const token = params.get('token')
  if (token) {
    localStorage.setItem('authToken', token)
  }
  return <Navigate to="/dashboard/groups" replace />
}

function AuthGuard({ children }: { children: ReactNode }) {
  const navigate = useNavigate()
  const location = useLocation()

  useEffect(() => {
    if (!localStorage.getItem('authToken')) {
      navigate('/login', { replace: true })
    }
  }, [navigate, location.pathname])

  return <>{children}</>
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/" element={<TokenCapture />} />
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
        <Route path="tunnelers" element={<TunnelersPage />} />
        <Route path="tunnelers/new" element={<NewTunnelerPage />} />
        <Route path="tunnelers/:tunnelerId" element={<TunnelerDetailPage />} />
        <Route path="policy" element={<PolicyLayout />}>
          <Route index element={<Navigate to="resource-policies" replace />} />
          <Route path="resource-policies" element={<ResourcePoliciesPage />} />
          <Route path="resource-policies/:policyId" element={<ResourcePolicyDetailPage />} />
          <Route path="sign-in" element={<SignInPolicyPage />} />
          <Route path="device-profiles" element={<DeviceProfilesPage />} />
        </Route>
        <Route path="discovery" element={<NetworkDiscoveryPage />} />
        <Route path="audit-logs" element={<AuditLogsPage />} />
      </Route>
    </Routes>
  )
}
