import { Outlet, useSearchParams } from 'react-router-dom'
import { Sidebar } from '@/components/dashboard/sidebar'
import { Header } from '@/components/dashboard/header'
import SetupChecklist from '@/components/dashboard/SetupChecklist'

export default function DashboardLayout() {
  const [searchParams] = useSearchParams()
  const showSetup = searchParams.get('setup') === 'true'

  return (
    <div className="flex h-screen overflow-hidden bg-background">
      {/* Sidebar Navigation */}
      <Sidebar />

      {/* Main Content Area */}
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Header */}
        <Header />

        {/* Setup Checklist */}
        {showSetup && <SetupChecklist />}

        {/* Page Content */}
        <main className="flex-1 overflow-y-auto">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
