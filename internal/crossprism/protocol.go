// Package crossprism defines Prism-to-Prism protocol messages carried over NATS.
package crossprism

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const (
	SubjectContextSync       = "prism.cross.context_sync"
	SubjectTaskRequest       = "prism.cross.task_request"
	SubjectStatusRequest     = "prism.cross.status_request"
	SubjectValidationRequest = "prism.cross.validation_request"
	SubjectTaskResponse      = "prism.cross.task_response"
	SubjectTaskAccept        = "prism.cross.task_accept"
	SubjectTaskReject        = "prism.cross.task_reject"
	SubjectClarification     = "prism.cross.clarification"
	SubjectTaskProgress      = "prism.cross.task_progress"
	SubjectTaskResult        = "prism.cross.task_result"
	SubjectTaskCancel        = "prism.cross.task_cancel"

	TypeContextSync       = "context_sync"
	TypeTaskRequest       = "task_request"
	TypeStatusRequest     = "status_request"
	TypeValidationRequest = "validation_request"
	TypeTaskResponse      = "task_response"
	TypeTaskAccept        = "task_accept"
	TypeTaskReject        = "task_reject"
	TypeClarificationReq  = "clarification_request"
	TypeClarificationResp = "clarification_response"
	TypeTaskProgress      = "task_progress"
	TypeTaskResult        = "task_result"
	TypeTaskCancel        = "task_cancel"
)

// DefaultSubjects returns the narrow subject allowlist for cross-Prism traffic.
func DefaultSubjects() []string {
	return []string{
		SubjectContextSync,
		SubjectTaskRequest,
		SubjectStatusRequest,
		SubjectValidationRequest,
		SubjectTaskResponse,
		SubjectTaskAccept,
		SubjectTaskReject,
		SubjectClarification,
		SubjectTaskProgress,
		SubjectTaskResult,
		SubjectTaskCancel,
	}
}

// SubjectForType returns the protocol subject used for a message type.
func SubjectForType(messageType string) string {
	switch messageType {
	case TypeContextSync:
		return SubjectContextSync
	case TypeTaskRequest:
		return SubjectTaskRequest
	case TypeStatusRequest:
		return SubjectStatusRequest
	case TypeValidationRequest:
		return SubjectValidationRequest
	case TypeTaskAccept:
		return SubjectTaskAccept
	case TypeTaskReject:
		return SubjectTaskReject
	case TypeClarificationReq, TypeClarificationResp:
		return SubjectClarification
	case TypeTaskProgress:
		return SubjectTaskProgress
	case TypeTaskResult:
		return SubjectTaskResult
	case TypeTaskCancel:
		return SubjectTaskCancel
	default:
		return SubjectTaskResponse
	}
}

// Message is the signed envelope exchanged by two Prism instances.
type Message struct {
	From          string         `json:"from"`
	To            string         `json:"to"`
	MessageType   string         `json:"message_type"`
	Priority      string         `json:"priority,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Thread        map[string]any `json:"thread,omitempty"`
	Context       map[string]any `json:"context,omitempty"`
	Request       map[string]any `json:"request,omitempty"`
	Response      map[string]any `json:"response,omitempty"`
	Validation    map[string]any `json:"validation,omitempty"`
	Telemetry     map[string]any `json:"telemetry,omitempty"`
	Timestamp     string         `json:"timestamp"`
	Nonce         string         `json:"nonce"`
	Signature     string         `json:"signature,omitempty"`
}

// NewMessage creates an unsigned protocol message with timestamp and nonce set.
func NewMessage(from, to, messageType string) Message {
	return Message{
		From:        from,
		To:          to,
		MessageType: messageType,
		Priority:    "normal",
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
		Nonce:       newNonce(),
	}
}

// Decode parses a protocol message from JSON.
func Decode(data []byte) (Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return Message{}, fmt.Errorf("crossprism: decode: %w", err)
	}
	return msg, nil
}

// Sign calculates and stores the message HMAC signature.
func (m *Message) Sign(secret string) error {
	if secret == "" {
		return fmt.Errorf("crossprism: secret is required")
	}
	if m.Timestamp == "" {
		m.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if m.Nonce == "" {
		m.Nonce = newNonce()
	}
	canonical, err := m.canonicalJSON()
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(canonical)
	m.Signature = hex.EncodeToString(mac.Sum(nil))
	return nil
}

// Verify validates the message signature and freshness.
func (m Message) Verify(secret string, maxAge time.Duration) error {
	if secret == "" {
		return fmt.Errorf("crossprism: secret is required")
	}
	if m.Signature == "" {
		return fmt.Errorf("crossprism: signature is required")
	}
	if m.Timestamp == "" {
		return fmt.Errorf("crossprism: timestamp is required")
	}
	if m.Nonce == "" {
		return fmt.Errorf("crossprism: nonce is required")
	}
	if maxAge > 0 {
		ts, err := time.Parse(time.RFC3339Nano, m.Timestamp)
		if err != nil {
			return fmt.Errorf("crossprism: invalid timestamp: %w", err)
		}
		age := time.Since(ts)
		if age < -time.Minute || age > maxAge {
			return fmt.Errorf("crossprism: message timestamp outside allowed age")
		}
	}

	canonical, err := m.canonicalJSON()
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(canonical)
	expected := mac.Sum(nil)
	actual, err := hex.DecodeString(m.Signature)
	if err != nil {
		return fmt.Errorf("crossprism: invalid signature encoding")
	}
	if !hmac.Equal(actual, expected) {
		return fmt.Errorf("crossprism: invalid signature")
	}
	return nil
}

// Payload returns signed message JSON.
func (m Message) Payload() ([]byte, error) {
	if m.Signature == "" {
		return nil, fmt.Errorf("crossprism: message is not signed")
	}
	return json.Marshal(m)
}

func (m Message) canonicalJSON() ([]byte, error) {
	m.Signature = ""
	payload, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("crossprism: canonical marshal: %w", err)
	}
	return payload, nil
}

func newNonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
