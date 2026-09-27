import axios from 'axios';

import { dashboardStore } from '../store';
import { guardAuthClient } from '../auth-lifecycle';

export const dashboardClient = axios.create({
  timeout: 10_000,
  headers: { 'Content-Type': 'application/json' },
});

guardAuthClient(dashboardClient);

dashboardClient.interceptors.request.use(
  (config) => {
    const token = dashboardStore.token;
    if (token) {
      config.headers = config.headers ?? {};
      config.headers.Authorization = `Bearer ${token}`;
    }
    return config;
  },
  undefined,
  { synchronous: true },
);
