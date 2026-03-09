import { useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { Shield } from 'lucide-react';
import { Button } from '@/components/ui/button';

const CONTROLLER_URL = import.meta.env.VITE_CONTROLLER_URL || 'http://localhost:8081';

export default function LoginPage() {
  const navigate = useNavigate();

  // If a ?token= param is present (post-OAuth redirect), store it and go to dashboard.
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const token = params.get('token');
    if (token) {
      localStorage.setItem('authToken', token);
      navigate('/dashboard/groups', { replace: true });
    }
  }, [navigate]);

  const handleLogin = () => {
    window.location.href = `${CONTROLLER_URL}/oauth/google/login`;
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="flex flex-col items-center gap-8 rounded-xl border bg-card p-10 shadow-sm">
        <div className="flex h-14 w-14 items-center justify-center rounded-xl bg-primary">
          <Shield className="h-8 w-8 text-primary-foreground" />
        </div>
        <div className="text-center">
          <h1 className="text-2xl font-bold">ZTNA Admin</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Sign in to manage your zero-trust network
          </p>
        </div>
        <Button className="w-full gap-2" onClick={handleLogin}>
          Sign in with Google
        </Button>
      </div>
    </div>
  );
}
