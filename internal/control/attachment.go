package control

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

// AttachmentGrant is the closed attachment wire contract.
type AttachmentGrant struct {
	AttachmentID string `json:"attachment_id"`
	ClientID     string `json:"client_id"`
	RunID        string `json:"run_id"`
	RepositoryID string `json:"repository_id"`
	Token        string `json:"token"`
	Generation   int64  `json:"generation"`
}

// Projection is the closed attachment wire contract.
type Projection struct {
	SchemaVersion      int         `json:"schema_version"`
	AttachmentID       string      `json:"attachment_id"`
	Generation         int64       `json:"generation"`
	ProjectionRevision uint64      `json:"projection_revision"`
	RunRevision        uint64      `json:"run_revision"`
	RunID              string      `json:"run_id"`
	TaskID             *string     `json:"task_id"`
	AttemptID          *string     `json:"attempt_id"`
	Label              *string     `json:"label"`
	Harness            *string     `json:"harness"`
	Model              *string     `json:"model"`
	WaitingReason      *string     `json:"waiting_reason"`
	RunState           string      `json:"run_state"`
	ReviewState        string      `json:"review_state"`
	Attention          string      `json:"attention"`
	Stop               *StopResult `json:"stop"`
}

// OpenAttachment is the closed attachment wire contract.
type OpenAttachment struct {
	Grant      AttachmentGrant `json:"grant"`
	Projection Projection      `json:"projection"`
}

// StopRequest is the closed attachment wire contract.
type StopRequest struct {
	OperationID      string `json:"operation_id"`
	RunID            string `json:"run_id"`
	AttemptID        string `json:"attempt_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Generation       int64  `json:"generation"`
}

// StopAck is the closed attachment wire contract.
type StopAck struct {
	OperationID string `json:"operation_id"`
	Accepted    bool   `json:"accepted"`
	RunRevision uint64 `json:"run_revision"`
	ErrorCode   string `json:"error_code"`
}

// StopResult is the closed attachment wire contract.
type StopResult struct {
	OperationID string  `json:"operation_id"`
	State       string  `json:"state"`
	Reason      string  `json:"reason"`
	AttemptID   string  `json:"attempt_id"`
	Ack         StopAck `json:"ack"`
}

// AttachmentClient exposes only scoped existing-run observation and stop.
type AttachmentClient interface {
	Open(context.Context, string) (OpenAttachment, error)
	Read(context.Context, AttachmentGrant) (Projection, error)
	Stop(context.Context, AttachmentGrant, StopRequest) (StopAck, error)
	Operation(context.Context, AttachmentGrant, string) (StopResult, error)
	Close(context.Context, AttachmentGrant) error
}

// AttachmentFrameBytes bounds complete optional-client frames, including prefix.
const AttachmentFrameBytes = 64 << 10

func (AttachmentGrant) frameLimit() int { return AttachmentFrameBytes }
func (Projection) frameLimit() int      { return AttachmentFrameBytes }
func (OpenAttachment) frameLimit() int  { return AttachmentFrameBytes }
func (StopRequest) frameLimit() int     { return AttachmentFrameBytes }
func (StopAck) frameLimit() int         { return AttachmentFrameBytes }
func (StopResult) frameLimit() int      { return AttachmentFrameBytes }

// Validate checks complete scope and current generation.
func (g AttachmentGrant) Validate() error {
	if err := required(g.AttachmentID, g.ClientID, g.RunID, g.RepositoryID, g.Token); err != nil {
		return err
	}
	return generation(g.Generation)
}

// Validate checks closed status vocabulary and safe optional presentation fields.
func (p Projection) Validate() error {
	if err := schema2(p.SchemaVersion); err != nil {
		return err
	}
	if err := required(p.AttachmentID, p.RunID, p.ReviewState, p.Attention); err != nil {
		return err
	}
	if err := generation(p.Generation); err != nil {
		return err
	}
	switch p.ReviewState {
	case "candidate", "verified", "applied", "unknown":
	default:
		return contractError("invalid review state")
	}
	switch p.Attention {
	case "none", "review", "stop_unconfirmed", "blocked", "unknown":
	default:
		return contractError("invalid attention state")
	}
	if p.Label != nil {
		s := *p.Label
		if !utf8.ValidString(s) || utf8.RuneCountInString(s) > 160 || strings.TrimSpace(s) != s || strings.IndexFunc(s, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0 {
			return contractError("unsafe projection label")
		}
	}

	if !knownRunState(p.RunState) {
		return contractError("invalid projection run state")
	}
	for _, s := range []*string{p.TaskID, p.AttemptID, p.Label, p.Harness, p.Model, p.WaitingReason} {
		if s != nil && *s != "" {
			if err := required(*s); err != nil {
				return err
			}
		}
	}
	if err := revision(p.ProjectionRevision); err != nil {
		return err
	}
	if err := revision(p.RunRevision); err != nil {
		return err
	}
	if p.Stop != nil {
		return p.Stop.Validate()
	}
	return nil
}

// Validate requires a complete matching grant and projection.
func (o OpenAttachment) Validate() error {
	if err := o.Grant.Validate(); err != nil {
		return err
	}
	if err := o.Projection.Validate(); err != nil {
		return err
	}
	if o.Grant.AttachmentID != o.Projection.AttachmentID || o.Grant.Generation != o.Projection.Generation || o.Grant.RunID != o.Projection.RunID {
		return contractError("grant/projection identity mismatch")
	}
	return nil
}

// Validate checks the exact run/attempt, generation and expected revision.
func (s StopRequest) Validate() error {
	if err := required(s.OperationID, s.RunID, s.AttemptID); err != nil {
		return err
	}
	if err := generation(s.Generation); err != nil {
		return err
	}
	return revision(s.ExpectedRevision)
}

// Validate separates immutable acceptance from a typed rejection.
func (a StopAck) Validate() error {
	if err := required(a.OperationID); err != nil {
		return err
	}
	if err := revision(a.RunRevision); err != nil {
		return err
	}
	if a.Accepted {
		if a.ErrorCode != "" {
			return contractError("accepted acknowledgment has error")
		}
	} else if !v2contract.Code(a.ErrorCode).Valid() {
		return contractError("rejected acknowledgment lacks canonical error")
	}
	return nil
}

// Validate checks accepted acknowledgment identity and the finite reason vocabulary.
func (r StopResult) Validate() error {
	if err := required(r.OperationID, r.AttemptID); err != nil {
		return err
	}
	if err := r.Ack.Validate(); err != nil {
		return err
	}
	if r.Ack.OperationID != r.OperationID || !r.Ack.Accepted || r.Reason != "" && r.Reason != "stop_unconfirmed" && !v2contract.Code(r.Reason).Valid() {
		return contractError("invalid stop result binding")
	}
	switch r.State {
	case "requested", "unconfirmed", "confirmed":
		return nil
	}
	return contractError("invalid stop result state")
}

// Closed attachment argument shapes do not carry native data or callbacks.
type attachmentOpenParams struct {
	RunID string `json:"run_id"`
}
type attachmentGrantParams struct {
	Grant AttachmentGrant `json:"grant"`
}
type attachmentStopParams struct {
	Grant   AttachmentGrant `json:"grant"`
	Request StopRequest     `json:"request"`
}
type attachmentOperationParams struct {
	Grant       AttachmentGrant `json:"grant"`
	OperationID string          `json:"operation_id"`
}

func (p attachmentOpenParams) Validate() error  { return required(p.RunID) }
func (p attachmentGrantParams) Validate() error { return p.Grant.Validate() }
func (p attachmentStopParams) Validate() error {
	if err := p.Grant.Validate(); err != nil {
		return err
	}
	if err := p.Request.Validate(); err != nil {
		return err
	}
	if p.Request.RunID != p.Grant.RunID || p.Request.Generation != p.Grant.Generation {
		return contractError("stop grant/request mismatch")
	}
	return nil
}
func (p attachmentOperationParams) Validate() error {
	if err := p.Grant.Validate(); err != nil {
		return err
	}
	return required(p.OperationID)
}
