import { Navigate, Route, Routes } from 'react-router-dom'
import DashboardLayout from './pages/DashboardLayout'
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

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/dashboard/groups" replace />} />
      <Route path="/dashboard" element={<DashboardLayout />}>
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
      </Route>
    </Routes>
  )
}
