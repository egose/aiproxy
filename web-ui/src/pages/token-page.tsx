import { zodResolver } from '@hookform/resolvers/zod';
import { HookFormTextInput } from '@egose/shadcn-theme/components/form/hook-text-input';
import { Alert, AlertDescription } from '@egose/shadcn-theme/components/ui/alert';
import { Button } from '@egose/shadcn-theme/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { useMutation } from '@tanstack/react-query';
import { FormProvider, useForm } from 'react-hook-form';
import { useNavigate } from 'react-router';
import { useSnapshot } from 'valtio';
import { dashboardStore, setDashboardToken } from '../store';
import { tokenFormSchema, type TokenForm } from '../types';
import { errorMessage, fetchSnapshot } from '../services/dashboard';

export function TokenPage() {
  const navigate = useNavigate();
  const current = useSnapshot(dashboardStore).token;
  const form = useForm<TokenForm>({
    resolver: zodResolver(tokenFormSchema),
    defaultValues: { token: current },
  });

  const connectMutation = useMutation({
    mutationFn: async (values: TokenForm) => {
      const previous = dashboardStore.token;
      setDashboardToken(values.token.trim());
      try {
        await fetchSnapshot();
      } catch (err) {
        setDashboardToken(previous);
        throw err;
      }
    },
    onSuccess: () => navigate('/', { replace: true }),
  });

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
          <FormProvider {...form}>
            <form className="grid gap-4" onSubmit={form.handleSubmit((values) => connectMutation.mutate(values))}>
              <HookFormTextInput<TokenForm>
                name="token"
                label="Bearer token"
                type="password"
                autoComplete="off"
                placeholder="paste dashboard token"
              />
              {connectMutation.error && (
                <Alert variant="danger">
                  <AlertDescription>{errorMessage(connectMutation.error)}</AlertDescription>
                </Alert>
              )}
              <div className="flex gap-2">
                <Button variant="primary" type="submit" disabled={connectMutation.isPending}>
                  {connectMutation.isPending ? 'Verifying...' : 'Save and connect'}
                </Button>
                <Button appearance="outline" type="button" onClick={onSignOut}>
                  Sign out
                </Button>
              </div>
            </form>
          </FormProvider>
        </CardContent>
      </Card>
    </div>
  );
}
