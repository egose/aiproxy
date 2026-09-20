import { zodResolver } from '@hookform/resolvers/zod';
import { Alert, AlertDescription } from '@egose/shadcn-theme/components/ui/alert';
import { Button } from '@egose/shadcn-theme/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { Input } from '@egose/shadcn-theme/components/ui/input';
import { Label } from '@egose/shadcn-theme/components/ui/label';
import { useForm } from 'react-hook-form';
import { useNavigate } from 'react-router';
import { useSnapshot } from 'valtio';
import { dashboardStore, setDashboardToken } from '../store';
import { tokenFormSchema, type TokenForm } from '../types';
import { errorMessage, fetchSnapshot } from '../services/dashboard';
import { useState } from 'react';

export function TokenPage() {
  const navigate = useNavigate();
  const current = useSnapshot(dashboardStore).token;
  const [checking, setChecking] = useState(false);
  const [checkError, setCheckError] = useState<string | null>(null);
  const form = useForm<TokenForm>({
    resolver: zodResolver(tokenFormSchema),
    defaultValues: { token: current },
  });

  const onSubmit = async (values: TokenForm) => {
    setChecking(true);
    setCheckError(null);
    const previous = dashboardStore.token;
    setDashboardToken(values.token.trim());
    try {
      await fetchSnapshot();
      navigate('/', { replace: true });
    } catch (err) {
      setDashboardToken(previous);
      setCheckError(errorMessage(err));
    } finally {
      setChecking(false);
    }
  };

  const onSignOut = () => {
    setDashboardToken('');
    form.reset({ token: '' });
  };

  return (
    <div className="mx-auto grid w-full max-w-2xl gap-6 p-6">
      <Card>
        <CardHeader>
          <CardTitle>Dashboard token</CardTitle>
          <CardDescription>
            Same bearer token the server uses for <span className="font-mono">/_internal/dashboard/*</span>. When the
            config omits <span className="font-mono">dashboard.token</span>, the server mints one and writes it to the
            dashboard token file; paste it here. It is stored only in this browser.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form className="grid gap-4" onSubmit={form.handleSubmit(onSubmit)}>
            <div className="grid gap-2">
              <Label htmlFor="token">Bearer token</Label>
              <Input
                id="token"
                type="password"
                autoComplete="off"
                placeholder="paste dashboard token"
                {...form.register('token')}
              />
              {form.formState.errors.token && (
                <span className="text-sm text-red-500">{form.formState.errors.token.message}</span>
              )}
            </div>
            {checkError && (
              <Alert variant="danger">
                <AlertDescription>{checkError}</AlertDescription>
              </Alert>
            )}
            <div className="flex gap-2">
              <Button variant="primary" type="submit" disabled={checking}>
                {checking ? 'Verifying...' : 'Save and connect'}
              </Button>
              <Button variant="outline" type="button" onClick={onSignOut}>
                Sign out
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
