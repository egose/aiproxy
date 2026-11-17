import { useId } from 'react';
import type { EncryptedReasoningDeclaration, EncryptedReasoningView } from '../types';
import { reasoningDeclaration, type ReasoningDraft } from '../lib/encrypted-reasoning-form';

export function ReasoningPolicyContext({ view, scope }: { view?: EncryptedReasoningView; scope?: 'alias' }) {
  const effective = view?.encrypted_reasoning_effective;
  return (
    <div className="grid gap-1 text-xs text-slate-500">
      {scope === 'alias' && view && view.encrypted_reasoning == null ? (
        <span>
          Saved effective policy: target-dependent. Each selected target resolves its own model → provider → root →
          built-ins policy; there is no single alias-wide effective policy.
        </span>
      ) : effective ? (
        <>
          <span>
            Saved effective policy:{' '}
            {scope === 'alias' ? (
              'explicit alias override'
            ) : (
              <>
                source {view?.encrypted_reasoning_source ?? 'unavailable'}
                {view?.encrypted_reasoning_inherited === undefined
                  ? ' · inheritance unavailable'
                  : view.encrypted_reasoning_inherited
                    ? ' · inherited from base'
                    : ' · not inherited from base'}
              </>
            )}
          </span>
          <pre className="whitespace-pre-wrap">{JSON.stringify(effective, null, 2)}</pre>
        </>
      ) : (
        <span>Effective context unavailable until read from the server.</span>
      )}
      <span>
        Effective context is read-only and reflects saved state, not unsaved edits or proof of runtime activation.
      </span>
    </div>
  );
}

export function ReasoningPolicyReadOnly({
  label,
  view,
  scope,
}: {
  label: string;
  view: EncryptedReasoningView;
  scope?: 'alias';
}) {
  return (
    <section aria-label={label} className="grid gap-1">
      <h4 className="text-sm font-medium">{label}</h4>
      <span className="text-xs">
        Local declaration:{' '}
        {view.encrypted_reasoning == null ? 'none (inherit)' : JSON.stringify(view.encrypted_reasoning)}
      </span>
      <ReasoningPolicyContext view={view} scope={scope} />
    </section>
  );
}

export function EncryptedReasoningControl({
  label,
  value,
  onChange,
  view,
  error,
  disabled,
  scope,
}: {
  label: string;
  value: ReasoningDraft;
  onChange: (value: ReasoningDraft) => void;
  view?: EncryptedReasoningView;
  error?: string;
  disabled?: boolean;
  scope?: 'alias';
}) {
  const id = useId();
  const declaration = value.declaration;
  const member = (key: keyof EncryptedReasoningDeclaration, next: boolean | string | string[] | undefined) => {
    const changed = { ...declaration, [key]: next };
    if (next === undefined) delete changed[key];
    onChange({ ...value, declaration: changed });
  };
  const inputClass = 'rounded-md border bg-transparent p-2 text-sm';
  return (
    <fieldset disabled={disabled} className="grid gap-2 rounded-md border p-3" aria-label={label}>
      <legend className="text-sm font-medium">{label}</legend>
      <label htmlFor={`${id}-mode`}>Policy mode</label>
      <select
        id={`${id}-mode`}
        className={inputClass}
        value={value.mode}
        onChange={(e) => onChange({ ...value, mode: e.target.value as ReasoningDraft['mode'] })}
      >
        <option value="inherit">Inherit (no local declaration)</option>
        <option value="reset">Safe reset (explicit empty block)</option>
        <option value="custom">Custom (declared members only)</option>
      </select>
      <p className="text-xs text-slate-500">
        Blocks replace wholesale. Omitted members use built-ins, not lower-layer values: passthrough true, mismatch
        fail, built-in match messages. Empty mismatch or an empty list also selects its built-in behavior.
      </p>
      {value.mode === 'custom' && (
        <>
          <label htmlFor={`${id}-passthrough`}>Passthrough member</label>
          <select
            id={`${id}-passthrough`}
            className={inputClass}
            value={declaration.passthrough === undefined ? 'omit' : String(declaration.passthrough)}
            onChange={(e) => member('passthrough', e.target.value === 'omit' ? undefined : e.target.value === 'true')}
          >
            <option value="omit">Omit member (built-in true)</option>
            <option value="true">Declare true</option>
            <option value="false">Declare false (strip before inference)</option>
          </select>
          <label htmlFor={`${id}-mismatch`}>Mismatch member</label>
          <select
            id={`${id}-mismatch`}
            className={inputClass}
            value={declaration.on_caller_mismatch === undefined ? 'omit' : declaration.on_caller_mismatch}
            onChange={(e) => member('on_caller_mismatch', e.target.value === 'omit' ? undefined : e.target.value)}
          >
            <option value="omit">Omit member (built-in fail)</option>
            <option value="">Declare empty string (built-in fail)</option>
            <option value="fail">Declare fail</option>
            <option value="strip_and_retry">Declare strip_and_retry</option>
          </select>
          <label htmlFor={`${id}-messages-mode`}>Match messages member</label>
          <select
            id={`${id}-messages-mode`}
            className={inputClass}
            value={declaration.match_messages === undefined ? 'omit' : 'declare'}
            onChange={(e) => member('match_messages', e.target.value === 'omit' ? undefined : [])}
          >
            <option value="omit">Omit member (built-in patterns)</option>
            <option value="declare">Declare list (empty = built-in patterns)</option>
          </select>
          {declaration.match_messages !== undefined && (
            <>
              <label htmlFor={`${id}-messages`}>Match messages (one per line)</label>
              <textarea
                id={`${id}-messages`}
                className={inputClass}
                value={declaration.match_messages.join('\n')}
                aria-invalid={!!error}
                aria-describedby={error ? `${id}-error` : undefined}
                onChange={(e) => member('match_messages', e.target.value === '' ? [] : e.target.value.split('\n'))}
              />
            </>
          )}
        </>
      )}
      {error && (
        <p id={`${id}-error`} role="alert" className="text-sm text-red-500">
          {error}
        </p>
      )}
      <span className="text-xs">Selected local declaration: {JSON.stringify(reasoningDeclaration(value))}</span>
      <ReasoningPolicyContext view={view} scope={scope} />
    </fieldset>
  );
}
