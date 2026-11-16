package provider

import (
	"encoding/json"
	"io"
)

func writeOpenAIChunk(w io.Writer, chunk openAIChunk) error {
	if limited, ok := w.(*executionStreamWriter); ok {
		return limited.writeChunk(chunk)
	}
	encoded, err := json.Marshal(chunk)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, "data: ")
	if err != nil {
		return err
	}
	_, err = w.Write(encoded)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, "\n\n")
	return err
}
