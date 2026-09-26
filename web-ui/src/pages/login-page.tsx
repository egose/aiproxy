import { zodResolver } from '@hookform/resolvers/zod';
import { HookFormTextInput } from '@egose/shadcn-theme/components/form/hook-text-input';
import { Alert, AlertDescription } from '@egose/shadcn-theme/components/ui/alert';
import { Button } from '@egose/shadcn-theme/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { useMutation } from '@tanstack/react-query';
import { FormProvider, useForm } from 'react-hook-form';
import { Link, useNavigate } from 'react-router';
import { adminLogin } from '../services/admin';
import { errorMessage } from '../services/dashboard';
import { setAdminSession } from '../store';
import { adminLoginSchema, type AdminLogin } from '../types';
import { useAdminStatus } from '../hooks';

export function LoginPage() {
  const navigate = useNavigate();
  const status = useAdminStatus();
  const form = useForm<AdminLogin>({
    resolver: zodResolver(adminLoginSchema),
    defaultValues: { email: '', password: '' },
  });

  const loginMutation = useMutation({
    mutationFn: async (values: AdminLogin) => adminLogin(values.email.trim(), values.password),
    onSuccess: (session) => {
      setAdminSession(session.access_token, session.refresh_token);
      navigate('/', { replace: true });
    },
  });

  const ssoEnabled = status.data?.oidc_enabled === true;
  const ssoLabel = status.data?.oidc_display_name || 'Single Sign-On';
  const registrationEnabled = status.data?.registration_enabled === true;

  return (
    <div className="mx-auto grid w-full max-w-2xl gap-6 p-6">
      <Card>
        <CardHeader>
          <CardTitle>Sign in</CardTitle>
          <CardDescription>
            Sign in with your account email and password to manage your organizations, API keys, and usage.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <FormProvider {...form}>
            <form className="grid gap-4" onSubmit={form.handleSubmit((values) => loginMutation.mutate(values))}>
              <HookFormTextInput<AdminLogin>
                name="email"
                label="Email"
                placeholder="you@example.com"
                autoComplete="username"
              />
              <HookFormTextInput<AdminLogin>
                name="password"
                label="Password"
                type="password"
                autoComplete="current-password"
              />
              {loginMutation.error && (
                <Alert variant="danger">
                  <AlertDescription>{errorMessage(loginMutation.error)}</AlertDescription>
                </Alert>
              )}
              <div className="flex items-center gap-3">
                <Button variant="primary" type="submit" disabled={loginMutation.isPending}>
                  {loginMutation.isPending ? 'Signing in...' : 'Sign in'}
                </Button>
                {registrationEnabled && (
                  <Link className="text-sm underline" to="/register">
                    Create account
                  </Link>
                )}
              </div>
            </form>
          </FormProvider>
          {ssoEnabled && (
            <>
              <div className="my-4 flex items-center gap-2 text-xs text-slate-500">
                <span className="h-px flex-1 bg-slate-200" /> or <span className="h-px flex-1 bg-slate-200" />
              </div>
              <Button
                appearance="outline"
                type="button"
                onClick={() => {
                  window.location.href = '/_internal/admin/oidc/start';
                }}
              >
                Continue with {ssoLabel}
              </Button>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
