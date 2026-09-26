import { zodResolver } from '@hookform/resolvers/zod';
import { HookFormTextInput } from '@egose/shadcn-theme/components/form/hook-text-input';
import { Alert, AlertDescription } from '@egose/shadcn-theme/components/ui/alert';
import { Button } from '@egose/shadcn-theme/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { useMutation } from '@tanstack/react-query';
import { FormProvider, useForm } from 'react-hook-form';
import { Link, useNavigate } from 'react-router';
import { z } from 'zod';
import { publicRegister } from '../services/admin';
import { errorMessage } from '../services/dashboard';

const registerSchema = z.object({
  email: z.string().trim().min(1, 'Enter your email.').email('Enter a valid email.'),
  password: z.string().min(8, 'Password must be at least 8 characters.'),
  org_name: z.string().optional(),
});

type RegisterForm = z.infer<typeof registerSchema>;

export function RegisterPage() {
  const navigate = useNavigate();
  const form = useForm<RegisterForm>({
    resolver: zodResolver(registerSchema),
    defaultValues: { email: '', password: '', org_name: '' },
  });

  const registerMutation = useMutation({
    mutationFn: async (values: RegisterForm) =>
      publicRegister({
        email: values.email.trim(),
        password: values.password,
        org_name: values.org_name?.trim() || undefined,
      }),
    onSuccess: () => navigate('/login', { replace: true }),
  });

  return (
    <div className="mx-auto grid w-full max-w-2xl gap-6 p-6">
      <Card>
        <CardHeader>
          <CardTitle>Create account</CardTitle>
          <CardDescription>
            Registering creates your own tenant organization where you are the tenant admin. You can invite teammates
            and manage teams from there.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <FormProvider {...form}>
            <form className="grid gap-4" onSubmit={form.handleSubmit((values) => registerMutation.mutate(values))}>
              <HookFormTextInput<RegisterForm>
                name="email"
                label="Email"
                placeholder="you@example.com"
                autoComplete="username"
              />
              <HookFormTextInput<RegisterForm>
                name="password"
                label="Password (min 8 characters)"
                type="password"
                autoComplete="new-password"
              />
              <HookFormTextInput<RegisterForm>
                name="org_name"
                label="Organization name (optional, derived from email)"
                placeholder="acme"
              />
              {registerMutation.error && (
                <Alert variant="danger">
                  <AlertDescription>{errorMessage(registerMutation.error)}</AlertDescription>
                </Alert>
              )}
              <div className="flex items-center gap-3">
                <Button variant="primary" type="submit" disabled={registerMutation.isPending}>
                  {registerMutation.isPending ? 'Registering...' : 'Register'}
                </Button>
                <Link className="text-sm underline" to="/login">
                  Back to sign in
                </Link>
              </div>
            </form>
          </FormProvider>
        </CardContent>
      </Card>
    </div>
  );
}
