import { CanceledError } from 'axios';
import { assertAdminSession, assertAuthGeneration } from './auth-lifecycle';
import { adminWorkspaceStore } from './store';
import type { AdminWorkspace } from './types';

export const systemWorkspaceId = '00000000-0000-0000-0000-000000000000';

export type WorkspaceContext = {
  workspaceId: string;
  generation: number;
  session: number;
};

export function selectWorkspace(workspaces: readonly AdminWorkspace[], savedId: string, globalAdmin: boolean) {
  const saved = workspaces.find((workspace) => workspace.id === savedId);
  if (saved) return saved;
  if (globalAdmin) return workspaces.find((workspace) => workspace.is_system);
  return workspaces.find((workspace) => !workspace.is_system) ?? workspaces.find((workspace) => workspace.is_system);
}

export function canManageWorkspaceResources(workspace: AdminWorkspace | undefined, globalAdmin: boolean) {
  return !!workspace && (globalAdmin || (!workspace.is_system && workspace.role === 'admin'));
}

export function resourceInWorkspace(resource: { source: string; workspace_id?: string }, workspaceId: string) {
  return resource.source === 'config' ? workspaceId === systemWorkspaceId : resource.workspace_id === workspaceId;
}

export function assertWorkspaceContext(context: WorkspaceContext | undefined): asserts context is WorkspaceContext {
  if (!context?.workspaceId) throw new CanceledError('Choose an authorized workspace before continuing');
  assertAuthGeneration(context.generation);
  assertAdminSession(context.session);
  if (context.workspaceId !== adminWorkspaceStore.workspaceId) {
    throw new CanceledError('Selected workspace changed');
  }
}
