import { useState } from 'react'
import { Link } from 'react-router-dom'
import { CheckCircle2, Circle, X, Plug, Box, Users } from 'lucide-react'
import { Button } from '@/components/ui/button'

const DISMISSED_KEY = 'ztna_setup_checklist_dismissed'

interface ChecklistItem {
  label: string
  description: string
  href: string
  icon: React.ReactNode
}

const items: ChecklistItem[] = [
  {
    label: 'Deploy a Connector',
    description: 'Connect your network by deploying a connector',
    href: '/dashboard/connectors',
    icon: <Plug className="h-4 w-4" />,
  },
  {
    label: 'Add a Resource',
    description: 'Define the resources users can access',
    href: '/dashboard/resources',
    icon: <Box className="h-4 w-4" />,
  },
  {
    label: 'Invite Team Members',
    description: 'Add users to your organization',
    href: '/dashboard/users',
    icon: <Users className="h-4 w-4" />,
  },
]

export default function SetupChecklist() {
  const [dismissed, setDismissed] = useState(
    () => localStorage.getItem(DISMISSED_KEY) === 'true'
  )

  if (dismissed) return null

  const handleDismiss = () => {
    localStorage.setItem(DISMISSED_KEY, 'true')
    setDismissed(true)
  }

  return (
    <div className="mx-6 mt-6 rounded-lg border bg-card p-4">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-sm font-semibold">Get started with your network</h3>
        <Button variant="ghost" size="icon" className="h-6 w-6" onClick={handleDismiss}>
          <X className="h-3.5 w-3.5" />
        </Button>
      </div>
      <div className="space-y-2">
        {items.map(item => (
          <Link
            key={item.href}
            to={item.href}
            className="flex items-center gap-3 rounded-md p-2 hover:bg-muted/50 transition-colors"
          >
            <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-muted">
              {item.icon}
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium">{item.label}</p>
              <p className="text-xs text-muted-foreground">{item.description}</p>
            </div>
            <Circle className="h-4 w-4 shrink-0 text-muted-foreground" />
          </Link>
        ))}
      </div>
    </div>
  )
}
