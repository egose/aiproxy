import { CanceledError, type AxiosInstance, type InternalAxiosRequestConfig } from 'axios';
import { proxy } from 'valtio';

export const authLifecycle = proxy({ generation: 0, session: 0 });
let controller = new AbortController();
const listeners = new Set<() => void>();

export function onAuthChange(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function advanceAuthGeneration(newSession = false) {
  authLifecycle.generation++;
  if (newSession) authLifecycle.session++;
  const previous = controller;
  controller = new AbortController();
  previous.abort();
  for (const listener of listeners) listener();
}

export function assertAuthGeneration(generation: number) {
  if (generation !== authLifecycle.generation) throw new CanceledError('Authentication context changed');
}

export function assertAdminSession(session: number) {
  if (session !== authLifecycle.session) throw new CanceledError('Account session changed');
}

export type AuthRequestConfig = InternalAxiosRequestConfig & {
  authGeneration?: number;
  authSession?: number;
  authAccessToken?: string;
  _retried?: boolean;
};

export function guardAuthClient(client: AxiosInstance) {
  client.interceptors.request.use(
    (config: AuthRequestConfig) => {
      config.authGeneration ??= authLifecycle.generation;
      config.authSession ??= authLifecycle.session;
      assertAuthGeneration(config.authGeneration);
      config.signal = config.signal
        ? AbortSignal.any([config.signal as AbortSignal, controller.signal])
        : controller.signal;
      return config;
    },
    undefined,
    { synchronous: true },
  );
  client.interceptors.response.use(
    (response) => {
      assertAuthGeneration((response.config as AuthRequestConfig).authGeneration!);
      return response;
    },
    (error) => {
      const config = error?.config as AuthRequestConfig | undefined;
      if (config?.authGeneration !== undefined) assertAuthGeneration(config.authGeneration);
      return Promise.reject(error);
    },
  );
}
