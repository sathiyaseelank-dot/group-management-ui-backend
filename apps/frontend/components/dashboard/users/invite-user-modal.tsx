'use client';

import { useState } from 'react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Mail } from 'lucide-react';
import { inviteUser } from '@/lib/mock-api';
import { getWorkspaceClaims } from '@/lib/jwt';

interface InviteUserModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export function InviteUserModal({ isOpen, onClose }: InviteUserModalProps) {
  const [email, setEmail] = useState('');
  const [isSending, setIsSending] = useState(false);
  const [result, setResult] = useState<string | null>(null);

  const handleInvite = async () => {
    if (!email.trim()) return;
    setIsSending(true);
    setResult(null);
    try {
      const token = localStorage.getItem('authToken');
      const claims = getWorkspaceClaims(token);
      const res = await inviteUser(email.trim(), claims?.wid);
      setResult(res.invite_url
        ? `Invite sent! (dev link: ${res.invite_url})`
        : 'Invite email sent successfully.');
      setEmail('');
    } catch (error) {
      setResult(`Error: ${(error as Error).message}`);
    } finally {
      setIsSending(false);
    }
  };

  const handleClose = () => {
    setEmail('');
    setResult(null);
    onClose();
  };

  return (
    <Dialog open={isOpen} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>Invite User</DialogTitle>
          <DialogDescription>
            Send an email invite. The recipient will sign in via Google OAuth.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-4">
          <div className="grid grid-cols-4 items-center gap-4">
            <Label htmlFor="invite-email" className="text-right">
              Email
            </Label>
            <Input
              id="invite-email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="col-span-3"
              placeholder="colleague@company.com"
              onKeyDown={(e) => e.key === 'Enter' && handleInvite()}
            />
          </div>
          {result && (
            <p className="col-span-4 px-1 text-sm text-muted-foreground">{result}</p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={handleClose} disabled={isSending}>
            Cancel
          </Button>
          <Button onClick={handleInvite} disabled={isSending || !email.trim()}>
            {isSending ? (
              <>
                <Mail className="mr-2 h-4 w-4 animate-pulse" /> Sending…
              </>
            ) : (
              'Send Invite'
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
