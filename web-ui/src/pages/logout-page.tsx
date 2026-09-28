import { useEffect, useRef } from 'react';
import { useNavigate } from 'react-router';
import { adminLogout } from '../services/admin';

export function LogoutPage() {
  const navigate = useNavigate();
  const done = useRef(false);

  useEffect(() => {
    if (done.current) return;
    done.current = true;
    void adminLogout().catch(() => undefined);
    navigate('/', { replace: true });
  }, [navigate]);

  return <div className="p-6 text-sm text-slate-500">Signing out...</div>;
}
