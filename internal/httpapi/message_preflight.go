package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"unicode/utf8"

	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

const (
	maxMessageTargetBytes        = 2048
	maxMessageRequestBytes       = 6*(message.MaxBodyBytes+message.MaxIdempotencyKeyBytes) + 256
	maxMessageDeleteRequestBytes = 6*message.MaxIdempotencyKeyBytes + 128
)

var errMessageRequestTooLarge = errors.New("message request body is too large")

type messageBodyRequest struct {
	Body           string `json:"body"`
	IdempotencyKey string `json:"idempotency_key"`
	ThreadRootID   *int64 `json:"thread_root_id,omitempty"`
}

type messageDeleteRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
}

type preparedMessage struct {
	conversationID int64
	messageID      int64
	rootMessageID  int64
	beforeID       int64
	afterID        int64
	limit          int
	body           messageBodyRequest
	deletion       messageDeleteRequest
}

type preparedMessageKey struct{}

func preparedMessageFromContext(ctx context.Context) (preparedMessage, bool) {
	prepared, ok := ctx.Value(preparedMessageKey{}).(preparedMessage)
	return prepared, ok
}

func withPreparedMessage(request *http.Request, prepared preparedMessage) *http.Request {
	ctx := context.WithValue(request.Context(), preparedMessageKey{}, prepared)
	return request.WithContext(ctx)
}

func preflightMessageHistory(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		values, ok := preflightMessageTarget(w, request, validateMessagePageQuery)
		if !ok {
			return
		}
		conversationID, err := positiveMessagePathID(request, "conversation_id")
		if err != nil {
			writeMessagePreflightProblem(w, err)
			return
		}
		beforeID, limit, _ := boundedMessagePage(values)
		next.ServeHTTP(w, withPreparedMessage(request, preparedMessage{
			conversationID: conversationID, beforeID: beforeID, limit: limit,
		}))
	})
}

func preflightMessageSend(next http.Handler) http.Handler {
	return preflightMessageBodyRoute("conversation_id", func(prepared *preparedMessage, id int64) {
		prepared.conversationID = id
	}, next)
}

func preflightMessageEdit(next http.Handler) http.Handler {
	return preflightMessageBodyRoute("message_id", func(prepared *preparedMessage, id int64) {
		prepared.messageID = id
	}, next)
}

func preflightMessageBodyRoute(
	pathName string,
	setID func(*preparedMessage, int64),
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if _, ok := preflightMessageTarget(w, request, validateNoMessageQuery); !ok {
			return
		}
		id, err := positiveMessagePathID(request, pathName)
		if err != nil {
			writeMessagePreflightProblem(w, err)
			return
		}
		var body messageBodyRequest
		if err := decodeMessageJSON(w, request, maxMessageRequestBytes, &body); err != nil {
			writeMessagePreflightProblem(w, err)
			return
		}
		if len(body.Body) > message.MaxBodyBytes {
			writeMessagePreflightProblem(w, errMessageRequestTooLarge)
			return
		}
		if !message.ValidBody(body.Body) || !message.ValidIdempotencyKey(body.IdempotencyKey) ||
			(body.ThreadRootID != nil && *body.ThreadRootID <= 0) {
			writeMessagePreflightProblem(w, message.ErrInvalidInput)
			return
		}
		prepared := preparedMessage{body: body}
		setID(&prepared, id)
		next.ServeHTTP(w, withPreparedMessage(request, prepared))
	})
}

func preflightMessageDelete(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if _, ok := preflightMessageTarget(w, request, validateNoMessageQuery); !ok {
			return
		}
		messageID, err := positiveMessagePathID(request, "message_id")
		if err != nil {
			writeMessagePreflightProblem(w, err)
			return
		}
		var body messageDeleteRequest
		if err := decodeMessageJSON(w, request, maxMessageDeleteRequestBytes, &body); err != nil {
			writeMessagePreflightProblem(w, err)
			return
		}
		if !message.ValidIdempotencyKey(body.IdempotencyKey) {
			writeMessagePreflightProblem(w, message.ErrInvalidInput)
			return
		}
		next.ServeHTTP(w, withPreparedMessage(request, preparedMessage{
			messageID: messageID, deletion: body,
		}))
	})
}

type messageQueryPolicy func(url.Values) error

func preflightMessageTarget(
	w http.ResponseWriter,
	request *http.Request,
	policy messageQueryPolicy,
) (url.Values, bool) {
	if len(request.URL.EscapedPath()) > maxMessageTargetBytes || len(request.URL.RawQuery) > maxMessageTargetBytes {
		writeMessagePreflightProblem(w, message.ErrInvalidInput)
		return nil, false
	}
	values, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil || policy(values) != nil {
		writeMessagePreflightProblem(w, message.ErrInvalidInput)
		return nil, false
	}
	return values, true
}

func decodeMessageJSON(w http.ResponseWriter, request *http.Request, maxBytes int64, destination any) error {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return message.ErrInvalidInput
	}
	if request.ContentLength > maxBytes {
		return errMessageRequestTooLarge
	}
	limited := http.MaxBytesReader(w, request.Body, maxBytes)
	raw, err := io.ReadAll(limited)
	_ = limited.Close()
	request.Body = http.NoBody
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return errMessageRequestTooLarge
	}
	if err != nil || !utf8.Valid(raw) {
		return message.ErrInvalidInput
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return message.ErrInvalidInput
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return message.ErrInvalidInput
	}
	return nil
}

func validateMessagePageQuery(values url.Values) error {
	_, _, err := boundedMessagePage(values)
	return err
}

func validateNoMessageQuery(values url.Values) error {
	if len(values) != 0 {
		return message.ErrInvalidInput
	}
	return nil
}

func boundedMessagePage(values url.Values) (int64, int, error) {
	for key, entries := range values {
		if (key != "before_id" && key != "limit") || len(entries) != 1 || entries[0] == "" {
			return 0, 0, message.ErrInvalidInput
		}
	}
	var beforeID int64
	var limit int
	var err error
	if value := values.Get("before_id"); value != "" {
		beforeID, err = strconv.ParseInt(value, 10, 64)
		if err != nil || beforeID <= 0 {
			return 0, 0, message.ErrInvalidInput
		}
	}
	if value := values.Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit <= 0 || limit > message.MaxPageLimit {
			return 0, 0, message.ErrInvalidInput
		}
	}
	return beforeID, limit, nil
}

func positiveMessagePathID(request *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(request.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, message.ErrInvalidInput
	}
	return id, nil
}

func writeMessagePreflightProblem(w http.ResponseWriter, err error) {
	if errors.Is(err, errMessageRequestTooLarge) {
		WriteProblem(w, Problem{
			Status: http.StatusRequestEntityTooLarge,
			Code:   "request_too_large",
			Detail: "request body exceeds the allowed size",
		})
		return
	}
	writeMessageProblem(w, message.ErrInvalidInput)
}
