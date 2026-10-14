package sync

import (
	"context"
	"errors"
	"testing"
)

func TestSelectedBatchStopsAfterPageOrAttachmentFailure(t *testing.T) {
	for _, attachmentFailure := range []bool{false, true} {
		client := &execFakeClient{}
		if attachmentFailure {
			client.uploadErr = errors.New("attachment failed")
		} else {
			client.updateErr = errors.New("page failed")
		}
		executor, err := NewExecutor(ExecutorOptions{Client: client, SpaceKey: "TEST", RootPageID: 1, StopOnError: true})
		if err != nil {
			t.Fatal(err)
		}
		plan := &Plan{Actions: []Action{
			{Kind: ActionUpdate, RelPath: "a.md", PageID: 2, Images: []ImageRef{{Filename: "a.png", Data: []byte("image")}}},
			{Kind: ActionUpdate, RelPath: "b.md", PageID: 3},
		}}
		result := executor.Apply(context.Background(), plan)
		if len(result.Errors) != 1 || len(client.updateCalls) != 1 || client.updateCalls[0].id != 2 {
			t.Fatalf("continued after failure: %+v calls=%+v", result, client.updateCalls)
		}
	}
}
