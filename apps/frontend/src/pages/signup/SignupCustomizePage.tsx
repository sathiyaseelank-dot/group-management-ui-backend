import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { useSignup } from '@/src/contexts/SignupContext'

function nameToSlug(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 63)
}

const teamSizes = [
  { value: 'just-me', label: 'Just me' },
  { value: '2-10', label: '2-10' },
  { value: '11-50', label: '11-50' },
  { value: '51-200', label: '51-200' },
  { value: '200+', label: '200+' },
]

export default function SignupCustomizePage() {
  const navigate = useNavigate()
  const { state, updateState } = useSignup()
  const [name, setName] = useState(state.networkName)
  const [slug, setSlug] = useState(state.networkSlug)
  const [slugEdited, setSlugEdited] = useState(false)
  const [teamSize, setTeamSize] = useState(state.teamSize)

  // Redirect guard: require email from previous step.
  useEffect(() => {
    if (!state.email) navigate('/signup', { replace: true })
  }, [state.email, navigate])

  const handleNameChange = (val: string) => {
    setName(val)
    if (!slugEdited) {
      setSlug(nameToSlug(val))
    }
  }

  const handleSlugChange = (val: string) => {
    setSlugEdited(true)
    setSlug(val.toLowerCase().replace(/[^a-z0-9-]/g, ''))
  }

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim() || !slug) return
    updateState({
      networkName: name.trim(),
      networkSlug: slug,
      attemptId: typeof crypto !== 'undefined' && crypto.randomUUID
        ? crypto.randomUUID()
        : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`,
      teamSize,
    })
    navigate('/signup/finalize')
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      <div>
        <h1 className="text-xl font-bold">Customize your Network</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Choose a name and URL for your organization's network
        </p>
      </div>

      <div className="space-y-2">
        <Label htmlFor="networkName">Network Name</Label>
        <Input
          id="networkName"
          value={name}
          onChange={e => handleNameChange(e.target.value)}
          placeholder="Acme Corporation"
          required
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="networkSlug">Network URL</Label>
        <Input
          id="networkSlug"
          value={slug}
          onChange={e => handleSlugChange(e.target.value)}
          placeholder="acme-corp"
          required
        />
        <p className="text-xs text-muted-foreground">
          Your network: <span className="font-mono">{slug || '...'}.zerotrust.com</span>
        </p>
      </div>

      {state.useCase === 'work' && (
        <div className="space-y-3">
          <Label>Team Size</Label>
          <RadioGroup value={teamSize} onValueChange={setTeamSize} className="flex flex-wrap gap-2">
            {teamSizes.map(s => (
              <div key={s.value} className="flex items-center">
                <RadioGroupItem value={s.value} id={`ts-${s.value}`} className="peer sr-only" />
                <Label
                  htmlFor={`ts-${s.value}`}
                  className={`cursor-pointer rounded-md border px-3 py-1.5 text-sm transition-colors ${
                    teamSize === s.value
                      ? 'border-primary bg-primary/5 text-primary'
                      : 'hover:bg-muted/50'
                  }`}
                >
                  {s.label}
                </Label>
              </div>
            ))}
          </RadioGroup>
        </div>
      )}

      <div className="flex gap-3">
        <Button type="button" variant="outline" onClick={() => navigate(-1)} className="flex-1">
          Back
        </Button>
        <Button type="submit" className="flex-1" disabled={!name.trim() || !slug}>
          Continue
        </Button>
      </div>
    </form>
  )
}
