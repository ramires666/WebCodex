package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"
)

func TestConcurrentCallsWithSamePublicIDStayIsolated(t *testing.T) {
	const callCount = 64

	requestReader, requestWriter := io.Pipe()
	responseReader, responseWriter := io.Pipe()
	defer requestReader.Close()
	defer requestWriter.Close()
	defer responseReader.Close()
	defer responseWriter.Close()

	client := &mcpClient{stdin: requestWriter, pending: make(map[string]chan json.RawMessage)}
	go client.readLoop(responseReader)

	errCh := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(requestReader)
		ids := make([]string, 0, callCount)
		names := make([]string, 0, callCount)
		for scanner.Scan() && len(ids) < callCount {
			var req struct {
				ID string `json:"id"`
				Params struct { Agent string `json:"agent"` } `json:"params"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &req); err != nil { errCh <- err; return }
			ids = append(ids, req.ID)
			names = append(names, req.Params.Agent)
		}
		seen := map[string]bool{}
		for _, id := range ids {
			if seen[id] { errCh <- fmt.Errorf("duplicate id %s", id); return }
			seen[id] = true
		}
		for i := len(ids)-1; i >= 0; i-- {
			fmt.Fprintf(responseWriter, `{"jsonrpc":"2.0","id":%q,"result":{"agent":%q}}
`, ids[i], names[i])
		}
		errCh <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	results := make(chan string, callCount)
	var wg sync.WaitGroup
	for i := 0; i < callCount; i++ {
		name := fmt.Sprintf("agent-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.call(ctx, json.RawMessage(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"agent":%q}}`, name)))
			if err != nil { results <- err.Error(); return }
			var out struct { Result struct { Agent string `json:"agent"` } `json:"result"` }
			json.Unmarshal(resp, &out)
			results <- out.Result.Agent
		}()
	}
	wg.Wait()
	close(results)
	if err := <-errCh; err != nil { t.Fatal(err) }
	for result := range results {
		if len(result) == 0 { t.Fatal("empty result") }
	}
}
