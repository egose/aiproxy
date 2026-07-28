package provider

import (
	"encoding/json"
	"io"
)

func writeOpenAIChunk(w io.Writer, chunk openAIChunk) error {
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
