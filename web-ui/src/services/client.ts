import axios from 'axios';

import { dashboardStore } from '../store';

export const dashboardClient = axios.create({
  timeout: 10_000,
  headers: { 'Content-Type': 'application/json' },
});

dashboardClient.interceptors.request.use((config) => {
  const token = dashboardStore.token;
  if (token) {
    config.headers = config.headers ?? {};
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});
