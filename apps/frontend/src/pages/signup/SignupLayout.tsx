import { Outlet, useLocation } from 'react-router-dom'
import { Shield } from 'lucide-react'
import { SignupProvider } from '@/src/contexts/SignupContext'

const steps = ['/signup', '/signup/customize', '/signup/finalize', '/signup/auth']

function StepIndicator({ current }: { current: number }) {
  return (
    <div className="flex items-center gap-2">
      {steps.map((_, i) => (
        <div
          key={i}
          className={`h-2 w-2 rounded-full transition-colors ${
            i <= current ? 'bg-primary' : 'bg-muted'
          }`}
        />
      ))}
    </div>
  )
}

export default function SignupLayout() {
  const location = useLocation()
  const currentStep = steps.indexOf(location.pathname)

  return (
    <SignupProvider>
      <div className="flex min-h-screen items-center justify-center bg-background">
        <div className="w-full max-w-md space-y-6 rounded-xl border bg-card p-8 shadow-sm">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-primary">
                <Shield className="h-6 w-6 text-primary-foreground" />
              </div>
              <span className="text-lg font-semibold">ZTNA</span>
            </div>
            <StepIndicator current={currentStep >= 0 ? currentStep : 0} />
          </div>
          <Outlet />
        </div>
      </div>
    </SignupProvider>
  )
}
