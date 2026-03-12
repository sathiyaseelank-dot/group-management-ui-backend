import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Home, Building2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useSignup } from '@/src/contexts/SignupContext'

export default function SignupPage() {
  const navigate = useNavigate()
  const { state, updateState } = useSignup()
  const [email, setEmail] = useState(state.email)
  const [useCase, setUseCase] = useState<'home' | 'work' | ''>(state.useCase)

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!email || !useCase) return
    updateState({ email, useCase })
    navigate('/signup/customize')
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-6">
      <div>
        <h1 className="text-xl font-bold">Create your Network</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Set up your organization's zero-trust network
        </p>
      </div>

      <div className="space-y-2">
        <Label htmlFor="email">Email address</Label>
        <Input
          id="email"
          type="email"
          value={email}
          onChange={e => setEmail(e.target.value)}
          placeholder="you@company.com"
          required
        />
      </div>

      <div className="space-y-2">
        <Label>How will you use ZTNA?</Label>
        <div className="grid grid-cols-2 gap-3">
          <button
            type="button"
            onClick={() => setUseCase('home')}
            className={`flex flex-col items-center gap-2 rounded-lg border p-4 transition-colors ${
              useCase === 'home'
                ? 'border-primary bg-primary/5 ring-1 ring-primary'
                : 'hover:bg-muted/50'
            }`}
          >
            <Home className="h-6 w-6" />
            <span className="text-sm font-medium">At home</span>
          </button>
          <button
            type="button"
            onClick={() => setUseCase('work')}
            className={`flex flex-col items-center gap-2 rounded-lg border p-4 transition-colors ${
              useCase === 'work'
                ? 'border-primary bg-primary/5 ring-1 ring-primary'
                : 'hover:bg-muted/50'
            }`}
          >
            <Building2 className="h-6 w-6" />
            <span className="text-sm font-medium">At work</span>
          </button>
        </div>
      </div>

      <Button type="submit" className="w-full" disabled={!email || !useCase}>
        Get Started
      </Button>

      <p className="text-center text-sm text-muted-foreground">
        Already have an account?{' '}
        <a href="/login" className="text-primary hover:underline">Sign in</a>
      </p>
    </form>
  )
}
