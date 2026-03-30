import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  ArrowLeft,
  CheckCircle,
  Copy,
  Download,
  Monitor,
  Settings,
  Shield,
  Terminal,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { getWorkspaceClaims } from '@/lib/jwt'

function copyToClipboard(text: string) {
  if (navigator.clipboard?.writeText) {
    return navigator.clipboard.writeText(text)
  }

  const textarea = document.createElement('textarea')
  textarea.value = text
  textarea.style.position = 'fixed'
  textarea.style.opacity = '0'
  document.body.appendChild(textarea)
  textarea.select()
  document.execCommand('copy')
  document.body.removeChild(textarea)
  return Promise.resolve()
}

function detectPlatform(): string {
  const ua = navigator.userAgent.toLowerCase()
  if (ua.includes('linux')) return 'Linux'
  if (ua.includes('mac')) return 'macOS'
  if (ua.includes('win')) return 'Windows'
  return 'Unknown'
}

function buildControllerUrl() {
  const configured = import.meta.env.VITE_CONTROLLER_URL
  if (configured) {
    return configured
  }
  return `${window.location.protocol}//${window.location.hostname}:8081`
}

function buildControllerGrpcAddr(controllerUrl: string) {
  try {
    const url = new URL(controllerUrl)
    return `${url.hostname}:8443`
  } catch {
    return `${window.location.hostname}:8443`
  }
}

function CommandBlock({
  title,
  description,
  command,
}: {
  title: string
  description: string
  command: string
}) {
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    try {
      await copyToClipboard(command)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      setCopied(false)
    }
  }

  return (
    <div className="space-y-3 rounded-lg border p-4">
      <div className="space-y-1">
        <p className="text-sm font-medium">{title}</p>
        <p className="text-sm text-muted-foreground">{description}</p>
      </div>

      <div className="rounded-md border bg-muted/50">
        <div className="flex items-center justify-between border-b px-3 py-2">
          <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
            <Terminal className="h-3.5 w-3.5" />
            Command
          </div>
          <Button variant="ghost" size="sm" className="h-7 px-2" onClick={handleCopy}>
            <Copy className="h-3.5 w-3.5" />
            {copied ? 'Copied' : 'Copy'}
          </Button>
        </div>
        <pre className="overflow-x-auto px-3 py-3 text-xs leading-6 text-foreground">
          <code>{command}</code>
        </pre>
      </div>
    </div>
  )
}

export default function InstallPage() {
  const navigate = useNavigate()
  const token = localStorage.getItem('authToken')
  const claims = getWorkspaceClaims(token)
  const isAdmin = claims?.wrole === 'admin' || claims?.wrole === 'owner'

  if (!token || !claims) {
    return (
      <div className="flex min-h-[calc(100vh-3.5rem)] items-center justify-center bg-background p-4">
        <div className="w-full max-w-md space-y-4 rounded-xl border bg-card p-8 text-center shadow-sm">
          <p className="text-sm text-muted-foreground">Invalid or expired session.</p>
          <Button variant="outline" onClick={() => navigate('/login', { replace: true })}>
            Go to Login
          </Button>
        </div>
      </div>
    )
  }

  const platform = detectPlatform()
  const controllerUrl = buildControllerUrl()
  const controllerGrpcAddr = buildControllerGrpcAddr(controllerUrl)
  const installCommand =
    'curl -fsSL https://raw.githubusercontent.com/sathiyaseelank-dot/group-management-ui-backend/merge/alpha-sync-20260330/scripts/client-install-release.sh | sudo bash'
  const setupCommand = [
    'sudo tee /etc/ztna-client/client.conf >/dev/null <<\'CONF\'',
    `controller_url = "${controllerUrl}"`,
    `controller_grpc_addr = "${controllerGrpcAddr}"`,
    'CONF',
    `sudo ztna-client setup --tenant "${claims.wslug}"`,
  ].join('\n')
  const finishCommand = [
    'sudo systemctl restart ztna-client',
    'ztna-client login',
    'ztna-client resources',
  ].join('\n')

  return (
    <div className="flex min-h-[calc(100vh-3.5rem)] justify-center bg-background p-4">
      <div className="w-full max-w-3xl space-y-6 py-8">
        <div className="rounded-xl border bg-card p-8 shadow-sm">
          <div className="flex flex-col gap-6">
            <div className="flex flex-col items-center space-y-3 text-center">
              <div className="flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
                <Monitor className="h-6 w-6 text-primary" />
              </div>
              <div className="space-y-2">
                <h1 className="text-2xl font-semibold tracking-tight">Install ZTNA Client</h1>
                <p className="text-sm text-muted-foreground">
                  Install the Linux client, register the active workspace, then sign in to{' '}
                  <span className="font-medium text-foreground">{claims.wslug}</span>.
                </p>
              </div>
            </div>

            <div className="grid gap-4 md:grid-cols-2">
              <div className="rounded-lg border bg-muted/40 p-4">
                <div className="flex items-start gap-3">
                  <Shield className="mt-0.5 h-5 w-5 shrink-0 text-primary" />
                  <div className="space-y-1">
                    <p className="text-sm font-medium">Workspace</p>
                    <p className="text-sm text-muted-foreground">{claims.wslug}.zerotrust.com</p>
                  </div>
                </div>
              </div>

              <div className="rounded-lg border bg-muted/40 p-4">
                <div className="flex items-start gap-3">
                  <Download className="mt-0.5 h-5 w-5 shrink-0 text-primary" />
                  <div className="space-y-1">
                    <p className="text-sm font-medium">Detected platform</p>
                    <p className="text-sm text-muted-foreground">{platform}</p>
                  </div>
                </div>
              </div>
            </div>

            {platform !== 'Linux' && (
              <div className="flex items-center gap-2 rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
                <CheckCircle className="h-4 w-4 shrink-0" />
                The scripted installer is currently Linux-first. You can still use the commands below on a Linux host or VM.
              </div>
            )}

            <CommandBlock
              title="1. Install the client"
              description="This downloads the installer script from the current alpha branch and installs the latest released ztna-client binary plus systemd service."
              command={installCommand}
            />

            <CommandBlock
              title="2. Set the active workspace"
              description="Write the controller endpoints once, then let ztna-client setup persist the selected workspace like a network setup flow."
              command={setupCommand}
            />

            <CommandBlock
              title="3. Start and sign in"
              description="Restart the service, complete login, then confirm your resource list is available."
              command={finishCommand}
            />

            <div className="flex items-center gap-2 rounded-lg border border-green-200 bg-green-50 p-3 text-sm text-green-700">
              <CheckCircle className="h-4 w-4 shrink-0" />
              Use the copied commands in a Linux terminal on the machine where you want the client installed.
            </div>

            <div className="flex gap-3">
              <Button variant="outline" className="flex-1 gap-2" onClick={() => navigate('/app')}>
                <ArrowLeft className="h-4 w-4" />
                Back to Home
              </Button>
              {isAdmin && (
                <Button className="flex-1 gap-2" onClick={() => navigate('/dashboard/groups', { replace: true })}>
                  <Settings className="h-4 w-4" />
                  Go to Dashboard
                </Button>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
