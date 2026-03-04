import { Outlet } from 'react-router-dom'

export default function PolicyLayout() {
  return (
    <div className="space-y-6 p-6">
      <div>
        <h1 className="text-2xl font-bold">Policy</h1>
        <p className="text-sm text-muted-foreground">
          Configure access policies, sign-in rules, and device profiles.
        </p>
      </div>
      <Outlet />
    </div>
  )
}
