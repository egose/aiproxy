import { useLayoutEffect, useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router';
import { Button } from '@egose/shadcn-theme/components/ui/button';
import { acceptAdminInvite } from '../services/admin';

export function InvitePage() {
  const location = useLocation();
  const navigate = useNavigate();
  const [code, setCode] = useState('');
  const [password, setPassword] = useState('');
  const [pending, setPending] = useState(false);
  const [accepted, setAccepted] = useState(false);
  const [error, setError] = useState('');
  useLayoutEffect(() => {
    if (location.hash || location.search || location.state) navigate('/invite', { replace: true, state: null });
  }, [location.hash, location.search, location.state, navigate]);

  return (
    <div className="mx-auto grid w-full max-w-2xl gap-4 p-6">
      <h1 className="text-xl font-semibold">Accept invitation</h1>
      {accepted ? (
        <>
          <p>Your account is ready. Sign in with your invited email and new password to access your workspace.</p>
          <Link to="/login" replace className="underline">
            Sign in
          </Link>
        </>
      ) : (
        <form
          className="grid gap-4"
          method="post"
          onSubmit={async (event) => {
            event.preventDefault();
            const enteredCode = code.trim();
            if (pending || !enteredCode) return;
            setPending(true);
            setError('');
            try {
              await acceptAdminInvite(enteredCode, password);
              setCode('');
              setPassword('');
              setAccepted(true);
            } catch {
              setError('Unable to accept invitation. Check the code and password, or ask for a new invitation.');
            } finally {
              setPending(false);
            }
          }}
        >
          <p>
            Paste the one-time code your inviter sent separately. If you received a link containing a code, ask your
            inviter for a new invitation. You can join the invited workspace even when public registration is closed.
          </p>
          <label className="grid gap-1">
            Invitation code
            <input
              className="rounded-md border px-3 py-2"
              name="invite-code"
              type="password"
              autoComplete="off"
              required
              value={code}
              onChange={(event) => setCode(event.target.value)}
              disabled={pending}
            />
          </label>
          <label className="grid gap-1">
            Password (min 8 characters)
            <input
              className="rounded-md border px-3 py-2"
              name="password"
              type="password"
              autoComplete="new-password"
              minLength={8}
              required
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              disabled={pending}
            />
          </label>
          {error && <p role="alert">{error}</p>}
          <Button type="submit" disabled={pending}>
            {pending ? 'Accepting...' : 'Accept invitation'}
          </Button>
          <Link to="/login" replace className="underline">
            Back to sign in
          </Link>
        </form>
      )}
    </div>
  );
}
