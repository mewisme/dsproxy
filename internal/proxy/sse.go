package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"time"

	"dsproxy/internal/reasoning"
	"dsproxy/internal/stream"
	"dsproxy/internal/transform"
)

func bufioReader(r io.Reader) *bufio.Reader {
	return bufio.NewReader(r)
}

func sseData(payload map[string]any) []byte {
	b, _ := json.Marshal(payload)
	return append(append([]byte("data: "), b...), '\n', '\n')
}

func rewriteSSELine(
	line []byte,
	originalModel string,
	accumulator *stream.Accumulator,
	store *reasoning.Store,
	cacheNamespace string,
	contexts []transform.RecordingContext,
	display *stream.DisplayAdapter,
	recoveryNotice string,
) (rewritten []byte, finalized bool, notice string, usage map[string]any) {
	notice = recoveryNotice
	trimmed := bytes.TrimSpace(line)
	if !bytes.HasPrefix(trimmed, []byte("data:")) {
		return line, false, notice, nil
	}
	data := bytes.TrimSpace(trimmed[5:])
	if bytes.Equal(data, []byte("[DONE]")) {
		for _, ctx := range contexts {
			_ = accumulator.StoreReasoning(store, ctx.Scope, cacheNamespace, ctx.Messages)
		}
		prefix := []byte{}
		if display == nil && recoveryNotice != "" {
			chunk := transform.RecoveryNoticeChunk(originalModel, recoveryNotice)
			chunk["created"] = float64(time.Now().Unix())
			prefix = sseData(chunk)
			notice = ""
		}
		if display != nil {
			if closing := display.FlushChunk(originalModel); closing != nil {
				closing["created"] = float64(time.Now().Unix())
				prefix = append(prefix, sseData(closing)...)
			}
			if recoveryNotice != "" {
				chunk := transform.RecoveryNoticeChunk(originalModel, recoveryNotice)
				chunk["created"] = float64(time.Now().Unix())
				prefix = append(prefix, sseData(chunk)...)
				notice = ""
			}
		}
		return append(prefix, []byte("data: [DONE]\n\n")...), true, notice, nil
	}

	var chunk map[string]any
	if err := json.Unmarshal(data, &chunk); err != nil {
		return line, false, notice, nil
	}
	if recoveryNotice != "" && transform.InjectRecoveryNotice(chunk, recoveryNotice) {
		notice = ""
	}
	accumulator.IngestChunk(chunk)
	for _, ctx := range contexts {
		_ = accumulator.StoreReadyReasoning(store, ctx.Scope, cacheNamespace, ctx.Messages)
	}
	if u, ok := chunk["usage"].(map[string]any); ok {
		usage = u
	}
	if display != nil {
		display.RewriteChunk(chunk)
	}
	if _, ok := chunk["model"]; ok {
		chunk["model"] = originalModel
	}
	ending := "\n"
	if bytes.HasSuffix(line, []byte("\r\n")) {
		ending = "\r\n"
	}
	b, _ := json.Marshal(chunk)
	return append(append(append([]byte("data: "), b...), []byte(ending)...), '\n'), false, notice, usage
}
