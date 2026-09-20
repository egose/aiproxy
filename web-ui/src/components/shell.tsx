import { useEffect, type ReactNode } from 'react';
import SidebarLayout, { useLayoutHeader, type ISidebarData } from '@egose/shadcn-theme/layouts/sidebar1';
import { TooltipProvider } from '@egose/shadcn-theme/components/ui/tooltip';
import { IconActivity, IconBox, IconGauge, IconKey, IconListDetails, IconShieldLock } from '@tabler/icons-react';
import { Link, useLocation } from 'react-router';
import { useSnapshot } from '../hooks';

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
  '/requests': { group: 'Dashboard', title: 'Recent requests' },
  '/payloads': { group: 'Dashboard', title: 'Payloads' },
  '/blocks': { group: 'Dashboard', title: 'Guardrail blocks' },
  '/token': { group: 'Dashboard', title: 'Token' },
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
  const meta = sectionMeta[location.pathname] ?? { group: 'Dashboard', title: 'Overview' };
  const version = snapshotQuery.data?.version ?? '';
  const headerTitle = version ? `${meta.title} · ${version}` : meta.title;

  const data: ISidebarData = {
    user: {
      name: 'aiproxy operator',
      email: version ? `v${version}` : 'local dashboard',
      avatar: '',
    },
    context: {
      title: 'aiproxy',
      canAdd: false,
      items: [{ name: 'Local server', text: snapshotQuery.data?.address ?? 'serve', active: true }],
    },
    menus: [
      {
        title: 'Dashboard',
        items: [
          { title: 'Overview', url: '/', icon: IconGauge, isActive: location.pathname === '/' },
          { title: 'Providers', url: '/providers', icon: IconBox, isActive: location.pathname === '/providers' },
          {
            title: 'Recent requests',
            url: '/requests',
            icon: IconListDetails,
            isActive: location.pathname === '/requests',
          },
          { title: 'Payloads', url: '/payloads', icon: IconActivity, isActive: location.pathname === '/payloads' },
          {
            title: 'Guardrail blocks',
            url: '/blocks',
            icon: IconShieldLock,
            isActive: location.pathname === '/blocks',
          },
        ],
      },
      {
        title: 'Access',
        items: [{ title: 'Token', url: '/token', icon: IconKey, isActive: location.pathname === '/token' }],
      },
    ],
    userMenus: [{ title: 'Token settings', icon: IconKey, url: '/token' }],
    events: {},
  };

  return (
    <TooltipProvider>
      <SidebarLayout aslink={Link} data={data}>
        <HeaderSlot group={meta.group} title={headerTitle} />
        {children}
      </SidebarLayout>
    </TooltipProvider>
  );
}
