import { useEffect, useState } from 'react'
import { Navigate, Outlet } from 'react-router-dom'

export default function RequireAuth() {
  const [ready, setReady] = useState(false)
  const [authenticated, setAuthenticated] = useState(false)

  useEffect(() => {
    let mounted = true
    ;(async () => {
      try {
        const res = await fetch('/api/auth/session', { credentials: 'include' })
        if (!mounted) return
        setAuthenticated(res.ok)
      } catch {
        if (!mounted) return
        setAuthenticated(false)
      } finally {
        if (mounted) setReady(true)
      }
    })()
    return () => {
      mounted = false
    }
  }, [])

  if (!ready) {
    return null
  }

  if (!authenticated) {
    return <Navigate to="/login" replace />
  }
  return <Outlet />
}
