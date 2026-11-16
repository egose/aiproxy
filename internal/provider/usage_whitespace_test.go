package provider

import (
	"bytes"
	"testing"
)

func TestUsageObservationAcceptsJSONWhitespaceWithoutChangingBody(t *testing.T) {
	for _, body := range [][]byte{
		[]byte(" \n\t\r{\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\r\n "),
		[]byte(" \n{\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"total_tokens\":5}}\n"),
		[]byte("\t{\"usageMetadata\":{\"promptTokenCount\":2,\"candidatesTokenCount\":3,\"totalTokenCount\":5}} "),
	} {
		original := bytes.Clone(body)
		expected := Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}
		if usage := usageFromBody(body); usage != expected {
			t.Fatalf("JSON whitespace discarded known usage: got %+v, want %+v; body=%q", usage, expected, body)
		}
		budget := &executionTestBudget{limit: 4 << 20}
		state, err := newExecutionState(ExecutionLimits{RequestBytes: 1024, ResponseBytes: 1024, Work: budget})
		if err != nil {
			t.Fatal(err)
		}
		usage := usageFromBodyWithLimits(body, state)
		state.release()
		if usage != expected || !bytes.Equal(body, original) {
			t.Fatalf("bounded usage observer changed usage or delivered bytes: usage=%+v body=%q", usage, body)
		}
		if used, _, _ := budget.stats(); used != 0 {
			t.Fatalf("usage observation retained temporary work: %d", used)
		}
	}
}
