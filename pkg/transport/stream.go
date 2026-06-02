package transport

import (
	"context"
	"encoding/json"
)

// StreamTransport is an abstract pub/sub transport.
// Implement MessageBroker to back it with Kafka, NATS, or WebSocket.
type StreamTransport struct {
	broker MessageBroker
}

// NewStreamTransport constructs a StreamTransport backed by broker.
func NewStreamTransport(broker MessageBroker) *StreamTransport {
	return &StreamTransport{broker: broker}
}

func (t *StreamTransport) Serve(ctx context.Context, handler WorkflowHandler) error {
	msgs, err := t.broker.Subscribe(ctx, "bunshin.workflow.requests")
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case data, ok := <-msgs:
			if !ok {
				return nil
			}
			var req WorkflowRequest
			if err := json.Unmarshal(data, &req); err != nil {
				continue
			}
			go t.dispatch(ctx, req, handler)
		}
	}
}

func (t *StreamTransport) dispatch(ctx context.Context, req WorkflowRequest, h WorkflowHandler) {
	runnable, err := h.Handle(req.WorkflowID)
	resp := WorkflowResponse{ThreadID: req.ThreadID}
	if err != nil {
		resp.Error = err.Error()
	} else {
		out, err := runnable.Invoke(ctx, req.Input)
		if err != nil {
			resp.Error = err.Error()
		} else if m, ok := out.(map[string]any); ok {
			resp.Output = m
		} else {
			resp.Output = map[string]any{"result": out}
		}
	}
	data, _ := json.Marshal(resp)
	_ = t.broker.Publish(ctx, "bunshin.workflow.responses."+req.ThreadID, data)
}

func (t *StreamTransport) Shutdown(_ context.Context) error {
	return t.broker.Close()
}
