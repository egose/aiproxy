import { zodResolver } from '@hookform/resolvers/zod';
import { HookFormCheckbox } from '@egose/shadcn-theme/components/form/hook-checkbox';
import { HookFormTextInput } from '@egose/shadcn-theme/components/form/hook-text-input';
import { Alert, AlertDescription } from '@egose/shadcn-theme/components/ui/alert';
import { Button } from '@egose/shadcn-theme/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@egose/shadcn-theme/components/ui/card';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useEffect } from 'react';
import { FormProvider, useForm } from 'react-hook-form';
import { z } from 'zod';
import { useAdminOIDCConfig } from '../hooks';
import { updateAdminOIDCConfig } from '../services/admin';
import { errorMessage } from '../services/dashboard';

const oidcSchema = z.object({
  enabled: z.boolean().optional(),
  issuer_url: z.string().optional(),
  client_id: z.string().optional(),
  client_secret: z.string().optional(),
  scopes: z.string().optional(),
  username_claim: z.string().optional(),
  admin_claim: z.string().optional(),
  admin_value: z.string().optional(),
  display_name: z.string().optional(),
});

type OidcForm = z.infer<typeof oidcSchema>;

export function AdminOidcPage() {
  const query = useAdminOIDCConfig();
  const queryClient = useQueryClient();
  const form = useForm<OidcForm>({
    resolver: zodResolver(oidcSchema),
    defaultValues: {
      enabled: false,
      issuer_url: '',
      client_id: '',
      client_secret: '',
      scopes: 'openid email profile',
      username_claim: 'email',
      admin_claim: '',
      admin_value: '',
      display_name: 'Single Sign-On',
    },
  });

  const data = query.data;
  useEffect(() => {
    if (!data) return;
    form.reset({
      enabled: data.enabled,
      issuer_url: data.issuer_url,
      client_id: data.client_id,
      client_secret: '',
      scopes: data.scopes || 'openid email profile',
      username_claim: data.username_claim || 'email',
      admin_claim: data.admin_claim,
      admin_value: data.admin_value,
      display_name: data.display_name || 'Single Sign-On',
    });
  }, [data, form]);

  const saveMutation = useMutation({
    mutationFn: async (values: OidcForm) =>
      updateAdminOIDCConfig({
        enabled: values.enabled ?? false,
        issuer_url: (values.issuer_url ?? '').trim(),
        client_id: (values.client_id ?? '').trim(),
        client_secret: values.client_secret || undefined,
        scopes: (values.scopes ?? '').trim(),
        username_claim: (values.username_claim ?? '').trim(),
        admin_claim: (values.admin_claim ?? '').trim(),
        admin_value: (values.admin_value ?? '').trim(),
        display_name: (values.display_name ?? '').trim(),
      }),
    onSuccess: async () => {
      form.setValue('client_secret', '');
      await queryClient.invalidateQueries({ queryKey: ['admin', 'oidc-config'] });
    },
  });

  return (
    <div className="grid w-full gap-6 p-6">
      {saveMutation.error && (
        <Alert variant="danger">
          <AlertDescription>{errorMessage(saveMutation.error)}</AlertDescription>
        </Alert>
      )}
      {saveMutation.isSuccess && (
        <Alert>
          <AlertDescription>Saved. Enabling verifies OIDC discovery before accepting the change.</AlertDescription>
        </Alert>
      )}
      <Card>
        <CardHeader>
          <CardTitle>Single sign-on (OIDC)</CardTitle>
          <CardDescription>
            Optional. When disabled, username and password sign-in keeps working on its own. When enabled, the login
            page also offers SSO; password sign-in stays available as a fallback. First-time SSO users are provisioned
            automatically; the admin role syncs from the claim below when configured.
          </CardDescription>
        </CardHeader>
        <CardContent className="grid max-w-2xl gap-4">
          <FormProvider {...form}>
            <form
              className="grid max-w-2xl gap-4"
              onSubmit={form.handleSubmit((values) => saveMutation.mutate(values))}
            >
              <div className="flex items-center gap-2 text-sm">
                <HookFormCheckbox<OidcForm> name="enabled" label="Enable single sign-on" />
                {data && (
                  <span className="text-xs text-slate-500">
                    {data.effective_enabled ? 'active' : 'inactive'}
                    {data.has_client_secret ? ' · secret set' : ' · no secret stored'}
                  </span>
                )}
              </div>
              <HookFormTextInput<OidcForm>
                name="issuer_url"
                label="Issuer URL"
                placeholder="http://localhost:18081/realms/aiproxy"
              />
              <HookFormTextInput<OidcForm> name="client_id" label="Client ID" placeholder="aiproxy-web" />
              <HookFormTextInput<OidcForm>
                name="client_secret"
                label="Client secret (write-only, empty keeps the stored one)"
                type="password"
                autoComplete="off"
                placeholder={data?.has_client_secret ? '•••••• (stored)' : 'client secret'}
              />
              <HookFormTextInput<OidcForm> name="scopes" label="Scopes" />
              <HookFormTextInput<OidcForm>
                name="username_claim"
                label="Username claim (falls back to email, then preferred_username)"
              />
              <HookFormTextInput<OidcForm> name="admin_claim" label="Admin claim (optional, e.g. groups)" />
              <HookFormTextInput<OidcForm>
                name="admin_value"
                label="Admin value (membership or equality grants admin)"
              />
              <HookFormTextInput<OidcForm> name="display_name" label="Button label" />
              <div className="text-xs text-slate-500">
                Register this redirect URI on the OIDC client:{' '}
                <span className="font-mono">http(s)://&lt;server-host&gt;/_internal/admin/oidc/callback</span>
              </div>
              <div>
                <Button variant="primary" type="submit" disabled={saveMutation.isPending || query.isLoading}>
                  {saveMutation.isPending ? 'Saving...' : 'Save OIDC settings'}
                </Button>
              </div>
            </form>
          </FormProvider>
        </CardContent>
      </Card>
    </div>
  );
}
