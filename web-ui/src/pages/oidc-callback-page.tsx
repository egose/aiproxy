import { Alert, AlertDescription } from '@egose/shadcn-theme/components/ui/alert';
import { Card, CardContent, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router';
import { setAdminSession } from '../store';

export function OidcCallbackPage() {
  const navigate = useNavigate();
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const hash = window.location.hash.replace(/^#/, '');
    const params = new URLSearchParams(hash);
    const err = params.get('error');
    const accessToken = params.get('access_token');
    const refreshToken = params.get('refresh_token');
    if (err) {
      setError(err);
      return;
    }
    if (!accessToken || !refreshToken) {
      setError('Sign-in callback did not include a session. Please try again.');
      return;
    }
    setAdminSession(accessToken, refreshToken);
    window.location.hash = '';
    navigate('/', { replace: true });
  }, [navigate]);

  return (
    <div className="mx-auto grid w-full max-w-2xl gap-6 p-6">
      <Card>
        <CardHeader>
          <CardTitle>Single sign-on</CardTitle>
        </CardHeader>
        <CardContent>
          {error ? (
            <Alert variant="danger">
              <AlertDescription>Sign-in failed: {error}</AlertDescription>
            </Alert>
          ) : (
            <div className="text-sm text-slate-500">Completing sign-in...</div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
