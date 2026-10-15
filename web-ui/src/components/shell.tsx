import { useEffect, type ReactNode } from 'react';
import SidebarLayout, { useLayoutHeader, type ISidebarData } from '@egose/shadcn-theme/layouts/sidebar1';
import { TooltipProvider } from '@egose/shadcn-theme/components/ui/tooltip';
import { useDialog } from '@egose/shadcn-theme/components/widgets/dialog-manager';
import {
  IconActivity,
  IconBox,
  IconGauge,
  IconKey,
  IconListDetails,
  IconLock,
  IconShare,
  IconShieldLock,
  IconUser,
  IconUsers,
} from '@tabler/icons-react';
import { useQueryClient } from '@tanstack/react-query';
import { Link, useLocation } from 'react-router';
import {
  useAdminMe,
  useAdminWorkspace,
  useAdminWorkspaces,
  useAdminSession,
  useAdminStatus,
  useDataAuth,
  useSnapshot,
} from '../hooks';
import { setAdminWorkspaceId } from '../store';
import { assertAuthGeneration, authLifecycle } from '../auth-lifecycle';
import { NewWorkspaceDialog } from './dialogs';

function ShellHeader({ title, group }: { title: string; group: string }) {
  return (
    <div className="flex min-w-0 items-center gap-3">
      <div className="grid min-w-0 gap-0.5">
        <span className="truncate text-[0.68rem] font-medium uppercase tracking-[0.18em] text-slate-500">
          aiproxy {group}
        </span>
        <div className="truncate text-sm font-medium">{title}</div>
      </div>
    </div>
  );
}

const sectionMeta: Record<string, { group: string; title: string }> = {
  '/': { group: 'Dashboard', title: 'Overview' },
  '/providers': { group: 'Dashboard', title: 'Providers' },
  '/aliases': { group: 'Dashboard', title: 'Aliases' },
  '/requests': { group: 'Dashboard', title: 'Recent requests' },
  '/payloads': { group: 'Dashboard', title: 'Payloads' },
  '/blocks': { group: 'Dashboard', title: 'Guardrail blocks' },
  '/token': { group: 'Dashboard', title: 'Token' },
  '/login': { group: 'Admin', title: 'Sign in' },
  '/register': { group: 'Admin', title: 'Create account' },
  '/logout': { group: 'Admin', title: 'Sign out' },
  '/admin/oidc/callback': { group: 'Admin', title: 'Sign in' },
  '/admin/workspaces': { group: 'Admin', title: 'Workspaces' },
  '/admin/users': { group: 'Admin', title: 'Users' },
  '/admin/oidc': { group: 'Admin', title: 'Single sign-on' },
  '/manage/workspace': { group: 'Management', title: 'Workspace' },
  '/manage/members': { group: 'Management', title: 'Members' },
  '/manage/teams': { group: 'Management', title: 'Teams' },
  '/manage/keys': { group: 'Management', title: 'API keys' },
};

function HeaderSlot({ group, title }: { group: string; title: string }) {
  const setHeader = useLayoutHeader();
  useEffect(() => {
    setHeader(<ShellHeader group={group} title={title} />);
    return () => setHeader(null);
  }, [group, title, setHeader]);
  return null;
}

export function Shell({ children }: { children: ReactNode }) {
  const location = useLocation();
  const snapshotQuery = useSnapshot();
  const adminStatus = useAdminStatus();
  const multi = adminStatus.data?.multi_tenancy_enabled === true;
  const session = useAdminSession();
  const dashboardAccess = useDataAuth();
  const meQuery = useAdminMe(multi);
  const signedIn = multi && !!session.accessToken;
  const meta = sectionMeta[location.pathname] ?? { group: 'Dashboard', title: 'Overview' };
  useEffect(() => {
    document.title = `${meta.title} · aiproxy`;
  }, [meta.title]);
  const workspacesQuery = useAdminWorkspaces(signedIn);
  const adminWorkspace = useAdminWorkspace();
  const queryClient = useQueryClient();
  const { openDialog } = useDialog();
  const workspaceList = multi && signedIn ? (workspacesQuery.data ?? []) : [];
  const currentWorkspaceId = workspaceList.some((o) => o.id === adminWorkspace.workspaceId)
    ? adminWorkspace.workspaceId
    : (workspaceList[0]?.id ?? '');
  useEffect(() => {
    if (
      multi &&
      signedIn &&
      workspacesQuery.data &&
      workspaceList.length > 0 &&
      currentWorkspaceId !== '' &&
      currentWorkspaceId !== adminWorkspace.workspaceId
    ) {
      setAdminWorkspaceId(currentWorkspaceId);
    }
  }, [multi, signedIn, workspacesQuery.data, workspaceList.length, currentWorkspaceId, adminWorkspace.workspaceId]);
  if (multi && !signedIn) {
    return (
      <div className="min-h-screen flex items-center justify-center p-6">
        <div className="w-full">{children}</div>
      </div>
    );
  }
  const me = signedIn ? meQuery.data : undefined;
  const version = snapshotQuery.data?.version ?? '';
  const manageResources = multi && signedIn;
  const group =
    manageResources && (location.pathname === '/providers' || location.pathname === '/aliases')
      ? 'Management'
      : meta.group;
  const headerTitle = version ? `${meta.title} · ${version}` : meta.title;
  const showAdmin =
    multi &&
    signedIn &&
    me?.is_admin === true &&
    workspaceList.find((o) => o.id === currentWorkspaceId)?.is_system === true;

  const data: ISidebarData = {
    user: me
      ? {
          name: me.email,
          email: me.is_admin ? 'administrator' : 'user',
          avatar: '',
        }
      : {
          name: 'aiproxy operator',
          email: version ? `v${version}` : 'local dashboard',
          avatar: '',
        },
    context:
      multi && signedIn
        ? {
            title: 'Workspaces',
            canAdd: true,
            addText: 'New Workspace',
            items: workspaceList.map((o) => ({
              name: o.display_name || o.name,
              text: o.display_name !== '' ? `${o.name} · ${o.role}` : o.role,
              active: o.id === currentWorkspaceId,
            })),
          }
        : {
            title: 'aiproxy',
            canAdd: false,
            items: [{ name: 'Local server', text: snapshotQuery.data?.address ?? 'serve', active: true }],
          },
    menus: [
      ...(!multi || dashboardAccess.enabled
        ? [
            {
              title: 'Dashboard',
              items: [
                { title: 'Overview', url: '/', icon: IconGauge, isActive: location.pathname === '/' },
                ...(!manageResources
                  ? [
                      {
                        title: 'Providers',
                        url: '/providers',
                        icon: IconBox,
                        isActive: location.pathname === '/providers',
                      },
                      {
                        title: 'Aliases',
                        url: '/aliases',
                        icon: IconShare,
                        isActive: location.pathname === '/aliases',
                      },
                    ]
                  : []),
                {
                  title: 'Recent requests',
                  url: '/requests',
                  icon: IconListDetails,
                  isActive: location.pathname === '/requests',
                },
                {
                  title: 'Payloads',
                  url: '/payloads',
                  icon: IconActivity,
                  isActive: location.pathname === '/payloads',
                },
                {
                  title: 'Guardrail blocks',
                  url: '/blocks',
                  icon: IconShieldLock,
                  isActive: location.pathname === '/blocks',
                },
              ],
            },
          ]
        : []),
      ...(multi
        ? []
        : [
            {
              title: 'Access',
              items: [{ title: 'Token', url: '/token', icon: IconKey, isActive: location.pathname === '/token' }],
            },
          ]),
      ...(multi && signedIn
        ? [
            {
              title: 'Management',
              items: [
                { title: 'Providers', url: '/providers', icon: IconBox, isActive: location.pathname === '/providers' },
                { title: 'Aliases', url: '/aliases', icon: IconShare, isActive: location.pathname === '/aliases' },
                {
                  title: 'Members',
                  url: '/manage/members',
                  icon: IconUsers,
                  isActive: location.pathname === '/manage/members',
                },
                {
                  title: 'Teams',
                  url: '/manage/teams',
                  icon: IconUser,
                  isActive: location.pathname === '/manage/teams',
                },
                {
                  title: 'API keys',
                  url: '/manage/keys',
                  icon: IconKey,
                  isActive: location.pathname === '/manage/keys',
                },
                {
                  title: 'Workspace',
                  url: '/manage/workspace',
                  icon: IconShare,
                  isActive: location.pathname === '/manage/workspace',
                },
              ],
            },
          ]
        : []),
      ...(showAdmin
        ? [
            {
              title: 'Admin',
              items: [
                {
                  title: 'Workspaces',
                  url: '/admin/workspaces',
                  icon: IconListDetails,
                  isActive: location.pathname === '/admin/workspaces',
                },
                { title: 'Users', url: '/admin/users', icon: IconUser, isActive: location.pathname === '/admin/users' },
                {
                  title: 'Single sign-on',
                  url: '/admin/oidc',
                  icon: IconShieldLock,
                  isActive: location.pathname === '/admin/oidc',
                },
              ],
            },
          ]
        : []),
    ],
    userMenus: [
      ...(!multi ? [{ title: 'Token settings', icon: IconKey, url: '/token' }] : []),
      ...(signedIn ? [{ title: 'Sign out', url: '/logout', icon: IconLock }] : []),
    ],
    events: {
      contextSelect: (ctx: { name: string }) => {
        const match = workspaceList.find((o) => (o.display_name || o.name) === ctx.name);
        if (match) {
          setAdminWorkspaceId(match.id);
        }
      },
      newContext: () => {
        const generation = authLifecycle.generation;
        void (async () => {
          try {
            const created = await openDialog(NewWorkspaceDialog, {});
            if (created) {
              assertAuthGeneration(generation);
              await queryClient.invalidateQueries({ queryKey: ['admin', 'workspaces'] });
              assertAuthGeneration(generation);
              setAdminWorkspaceId(created.id);
            }
          } catch {
            return;
          }
        })();
      },
    },
  };

  return (
    <TooltipProvider>
      <SidebarLayout aslink={Link} data={data}>
        <HeaderSlot group={group} title={headerTitle} />
        {children}
      </SidebarLayout>
    </TooltipProvider>
  );
}
