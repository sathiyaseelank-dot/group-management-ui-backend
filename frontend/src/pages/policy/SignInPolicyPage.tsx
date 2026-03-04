import { useEffect, useMemo, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import { getSignInPolicy, saveMfaRequired, saveReauth } from '@/lib/sign-in-policy';

type ManagePanel = 'reauth' | 'mfa' | null;

export default function SignInPolicyPage() {
  const [panel, setPanel] = useState<ManagePanel>(null);

  const [days, setDays] = useState('30');
  const [hours, setHours] = useState('0');
  const [mfaMode, setMfaMode] = useState<'no' | 'yes'>('no');
  const [error, setError] = useState<string | null>(null);

  const state = useMemo(() => getSignInPolicy(), [panel]);

  useEffect(() => {
    const s = getSignInPolicy();
    setDays(String(s.reauth.days));
    setHours(String(s.reauth.hours));
    setMfaMode(s.mfa.required ? 'yes' : 'no');
  }, []);

  const summaryReauth = `Authenticate every ${state.reauth.days} days`;
  const summaryMfa = state.mfa.required ? 'MFA Required' : 'MFA Not Required';

  const handleConfirmReauth = () => {
    setError(null);
    const d = Number(days);
    const h = Number(hours);
    if (!Number.isFinite(d) || d < 0 || !Number.isInteger(d)) {
      setError('Days must be a whole number (0 or greater).');
      return;
    }
    if (!Number.isFinite(h) || h < 0 || !Number.isInteger(h) || h > 23) {
      setError('Hours must be a whole number between 0 and 23.');
      return;
    }
    if (d === 0 && h === 0) {
      setError('Days and hours cannot both be 0.');
      return;
    }
    saveReauth(d, h);
    setPanel(null);
  };

  const handleConfirmMfa = () => {
    saveMfaRequired(mfaMode === 'yes');
    setPanel(null);
  };

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Sign In Requirements</h2>
        <p className="text-sm text-muted-foreground">
          Configure re-authentication frequency and MFA requirements.
        </p>
      </div>

      <div className="space-y-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0">
            <div className="space-y-1">
              <CardTitle className="text-base">Authenticate every 30 days</CardTitle>
              <CardDescription>{summaryReauth}</CardDescription>
            </div>
            <Button variant="outline" onClick={() => setPanel(panel === 'reauth' ? null : 'reauth')}>
              Manage
            </Button>
          </CardHeader>
          {panel === 'reauth' && (
            <CardContent className="pt-0">
              <div className="rounded-md border bg-muted/30 p-4 space-y-4">
                <div>
                  <h3 className="text-sm font-semibold">Authentication</h3>
                  <p className="text-sm text-muted-foreground">
                    Require authentication at least once every
                  </p>
                </div>

                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="reauthDays">Days</Label>
                    <Input
                      id="reauthDays"
                      inputMode="numeric"
                      value={days}
                      onChange={(e) => setDays(e.target.value)}
                      placeholder="30"
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="reauthHours">Hours</Label>
                    <Input
                      id="reauthHours"
                      inputMode="numeric"
                      value={hours}
                      onChange={(e) => setHours(e.target.value)}
                      placeholder="0"
                    />
                  </div>
                </div>

                {error && (
                  <p className="text-sm text-destructive" role="alert">
                    {error}
                  </p>
                )}

                <div className="flex justify-end">
                  <Button onClick={handleConfirmReauth}>Confirm changes</Button>
                </div>
              </div>
            </CardContent>
          )}
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0">
            <div className="space-y-1">
              <CardTitle className="text-base">MFA Not Required</CardTitle>
              <CardDescription>{summaryMfa}</CardDescription>
            </div>
            <Button variant="outline" onClick={() => setPanel(panel === 'mfa' ? null : 'mfa')}>
              Manage
            </Button>
          </CardHeader>
          {panel === 'mfa' && (
            <CardContent className="pt-0">
              <div className="rounded-md border bg-muted/30 p-4 space-y-4">
                <RadioGroup value={mfaMode} onValueChange={(v) => setMfaMode(v as 'no' | 'yes')}>
                  <div className="flex items-start gap-3">
                    <RadioGroupItem value="no" id="mfa-no" />
                    <div className="grid gap-1">
                      <Label htmlFor="mfa-no">Don&apos;t require MFA</Label>
                    </div>
                  </div>
                  <div className="flex items-start gap-3">
                    <RadioGroupItem value="yes" id="mfa-yes" />
                    <div className="grid gap-1">
                      <Label htmlFor="mfa-yes">Require MFA</Label>
                      <p className="text-xs text-muted-foreground">
                        Users will be required to authenticate with a second method like TOTP,
                        Biometrics, or a Security Key.
                      </p>
                    </div>
                  </div>
                </RadioGroup>

                <div className="flex justify-end">
                  <Button onClick={handleConfirmMfa}>Confirm changes</Button>
                </div>
              </div>
            </CardContent>
          )}
        </Card>
      </div>
    </div>
  );
}
