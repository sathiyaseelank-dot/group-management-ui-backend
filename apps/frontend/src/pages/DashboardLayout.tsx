import { Outlet, useSearchParams } from 'react-router-dom'
import { Sidebar } from '@/components/dashboard/sidebar'
import { Header } from '@/components/dashboard/header'
import SetupChecklist from '@/components/dashboard/SetupChecklist'
import { AnimatedGridPattern } from '@/components/ui/animated-grid-pattern'

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

        {/* Page Content — animated grid + gradient for depth */}
        <main className="relative flex-1 overflow-y-auto">
          <AnimatedGridPattern
            numSquares={30}
            maxOpacity={0.08}
            duration={5}
            className="text-primary/40 [mask-image:radial-gradient(500px_circle_at_center,white,transparent)]"
          />
          <div className="absolute inset-0 pointer-events-none" style={{
            background: 'radial-gradient(ellipse 60% 40% at 50% 0%, oklch(0.68 0.19 250 / 0.03), transparent 70%)'
          }} />
          <div className="relative z-[1]">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  )
}
