import { useEffect, useRef } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router';
import { adminLogout } from '../services/admin';

export function LogoutPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const done = useRef(false);

  useEffect(() => {
    if (done.current) return;
    done.current = true;
    adminLogout()
      .catch(() => undefined)
      .finally(() => {
        queryClient.clear();
        navigate('/', { replace: true });
      });
  }, [navigate, queryClient]);

  return <div className="p-6 text-sm text-slate-500">Signing out...</div>;
}
