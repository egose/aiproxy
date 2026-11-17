import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, describe, expect, it } from 'vitest';
import { reasoningDraft } from '../lib/encrypted-reasoning-form';
import type { EncryptedReasoningView } from '../types';
import { EncryptedReasoningControl } from './encrypted-reasoning-control';

afterEach(cleanup);

const effective = { passthrough: false, on_caller_mismatch: 'fail', match_messages: ['saved mismatch'] } as const;

function Editor({ label, view }: { label: string; view: EncryptedReasoningView }) {
  const [value, onChange] = useState(() => reasoningDraft(view.encrypted_reasoning));
  return <EncryptedReasoningControl label={label} value={value} onChange={onChange} view={view} />;
}

describe('shared saved context across provider/model and alias controls', () => {
  it.each([
    ['Provider encrypted reasoning', 'provider', false],
    ['Model inherited encrypted reasoning', 'model', true],
  ] as const)(
    'retains %s source context during unsaved resets without alias-specific metadata',
    (label, source, inherited) => {
      render(
        <Editor
          label={label}
          view={{
            encrypted_reasoning: inherited ? null : { passthrough: false },
            encrypted_reasoning_effective: { ...effective, match_messages: [...effective.match_messages] },
            encrypted_reasoning_source: source,
            encrypted_reasoning_inherited: inherited,
          }}
        />,
      );
      expect(
        screen.getByText(
          `Saved effective policy: source ${source} · ${inherited ? 'inherited from base' : 'not inherited from base'}`,
        ),
      ).toBeInTheDocument();
      fireEvent.change(screen.getByLabelText('Policy mode'), { target: { value: 'reset' } });
      expect(screen.getByLabelText('Policy mode')).toHaveValue('reset');
      expect(screen.getByText('Selected local declaration: {}')).toBeInTheDocument();
      expect(screen.getByText(/"saved mismatch"/)).toBeInTheDocument();
      expect(screen.getByText(/saved state, not unsaved edits or proof of runtime activation/)).toBeInTheDocument();
      expect(screen.queryByText(/explicit alias override|target-dependent/)).not.toBeInTheDocument();
    },
  );
});
