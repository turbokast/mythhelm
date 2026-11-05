package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// attachmentRequestBudget is the end-to-end server budget of one attachment
// request (NFR-2): handler, encoding and the wait for queue acceptance. It
// does not bound the socket write of an accepted frame.
const attachmentRequestBudget = 2 * time.Second

// AttachmentHandler answers one transport-only attachment request. A
// returned response carries its own cleanup obligation; on error the handler
// keeps that obligation, and a response returned beside an error is released
// and discarded.
type AttachmentHandler func(ctx context.Context, peer Peer, intent Intent) (*AttachmentResponse, error)

// AttachmentResponse is an immutable reply and a one-shot cleanup
// obligation. It confers no authority: the cleanup releases a scope the
// handler has already verified. Only the connection that served the request
// consumes it.
type AttachmentResponse struct {
	result  Result
	release func()
	once    sync.Once
}

// NewAttachmentResponse copies result, whose body must be valid JSON within
// AttachmentFrameBytes, and takes over release. On error release is not
// taken over and stays the caller's obligation.
func NewAttachmentResponse(result Result, release func()) (*AttachmentResponse, error) {
	if release == nil {
		return nil, errors.New("control: attachment response needs a release")
	}
	if len(result.Body) > AttachmentFrameBytes {
		return nil, fmt.Errorf("control: attachment response body %d bytes exceeds limit %d", len(result.Body), AttachmentFrameBytes)
	}
	if len(result.Body) > 0 && !json.Valid(result.Body) {
		return nil, errors.New("control: attachment response body is not valid JSON")
	}
	r := &AttachmentResponse{result: Result{OperationID: result.OperationID, Revision: result.Revision}, release: release}
	if result.Body != nil {
		r.result.Body = bytes.Clone(result.Body)
	}
	if result.Error != nil {
		e := *result.Error
		r.result.Error = &e
	}
	return r, nil
}

// settle runs the cleanup obligation exactly once.
func (r *AttachmentResponse) settle() { r.once.Do(r.release) }

// encode serialises the reply for the request operationID within the
// attachment frame limit.
func (r *AttachmentResponse) encode(operationID string) ([]byte, error) {
	res := r.result
	res.OperationID = operationID
	out, err := Encode(res)
	if err != nil {
		return nil, err
	}
	if len(out) > AttachmentFrameBytes {
		return nil, fmt.Errorf("control: attachment response frame %d bytes exceeds limit %d", len(out), AttachmentFrameBytes)
	}
	return out, checkOutgoing(out)
}

func attachmentMethod(method string) bool {
	switch method {
	case "attachment.open", "attachment.read", "attachment.close":
		return true
	}
	return false
}

// RegisterAttachment binds one of attachment.open, attachment.read and
// attachment.close to h. Any other method, a nil handler or a duplicate
// registration is an error naming the method.
func (s *Server) RegisterAttachment(method string, h AttachmentHandler) error {
	if !attachmentMethod(method) {
		return fmt.Errorf("control: method %q is not a transport attachment method", method)
	}
	if h == nil {
		return fmt.Errorf("control: attachment method %q needs a handler", method)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.attachments[method]; dup {
		return fmt.Errorf("control: method %q is already registered", method)
	}
	s.attachments[method] = h
	return nil
}

// runAttachment authenticates and validates before the attachment table is
// consulted; it never runs Execute, so no operation or result row exists.
func (s *Server) runAttachment(ctx context.Context, peer Peer, in Intent) (*AttachmentResponse, error) {
	if !peer.SameUser {
		return nil, newError(CodePermissionDenied, "attachment unavailable")
	}
	if err := in.Validate(); err != nil {
		return nil, newError(CodeInvalidContract, "invalid intent: %v", err)
	}
	s.mu.RLock()
	h, ok := s.attachments[in.Method]
	s.mu.RUnlock()
	if !ok {
		return nil, newError(CodeCapabilityUnsupported, "method %q is not supported", in.Method)
	}
	resp, err := h(ctx, peer, in)
	if err != nil {
		if resp != nil {
			resp.settle()
		}
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("control: attachment handler returned no response")
	}
	return resp, nil
}

// answerAttachment runs one attachment request within its budget and offers
// the encoded reply to the connection queue. The response is released right
// after the queue accepts its frame, or when encoding, the budget or the
// connection fails first, in which case no byte of it is exposed. It
// reports false when the connection must end.
func (s *Server) answerAttachment(ctx context.Context, peer Peer, in Intent, queue chan<- []byte) bool {
	ctx, cancel := context.WithTimeout(ctx, attachmentRequestBudget)
	defer cancel()
	resp, err := s.runAttachment(ctx, peer, in)
	if err != nil {
		return offerFailure(ctx, queue, in.OperationID, err)
	}
	if ctx.Err() != nil {
		resp.settle()
		return false
	}
	out, err := resp.encode(in.OperationID)
	if err != nil {
		resp.settle()
		return offerFailure(ctx, queue, in.OperationID, err)
	}
	accepted := offer(ctx, queue, out)
	resp.settle()
	return accepted
}

// offerFailure offers err as a typed error frame. A frame over
// AttachmentFrameBytes is replaced by one keeping only the error code.
func offerFailure(ctx context.Context, queue chan<- []byte, operationID string, err error) bool {
	ce := replyError(err)
	out, err := Encode(Result{OperationID: operationID, Error: ce})
	if err != nil {
		return false
	}
	if len(out) > AttachmentFrameBytes {
		out, err = Encode(Result{OperationID: operationID, Error: newError(ce.Code, "the error message exceeds the attachment frame limit")})
		if err != nil || len(out) > AttachmentFrameBytes {
			return false
		}
	}
	return offer(ctx, queue, out)
}

// offer hands out to the writer queue, giving up when ctx ends first. An
// already-ended ctx never hands out, even into an empty queue.
func offer(ctx context.Context, queue chan<- []byte, out []byte) bool {
	if ctx.Err() != nil {
		return false
	}
	select {
	case queue <- out:
		return true
	case <-ctx.Done():
		return false
	}
}
