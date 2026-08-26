package httpapi

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

func preflightMessageThread(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		values, ok := preflightMessageTarget(w, request, validateThreadPageQuery)
		if !ok {
			return
		}
		conversationID, conversationErr := positiveMessagePathID(request, "conversation_id")
		rootID, rootErr := positiveMessagePathID(request, "root_message_id")
		if conversationErr != nil || rootErr != nil {
			writeMessagePreflightProblem(w, message.ErrInvalidInput)
			return
		}
		afterID, limit, _ := boundedThreadPage(values)
		next.ServeHTTP(w, withPreparedMessage(request, preparedMessage{
			conversationID: conversationID, rootMessageID: rootID, afterID: afterID, limit: limit,
		}))
	})
}

func preflightMessageThreadList(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if _, ok := preflightMessageTarget(w, request, validateNoMessageQuery); !ok {
			return
		}
		conversationID, err := positiveMessagePathID(request, "conversation_id")
		if err != nil {
			writeMessagePreflightProblem(w, err)
			return
		}
		next.ServeHTTP(w, withPreparedMessage(request, preparedMessage{conversationID: conversationID}))
	})
}

func preflightMessageThreadMutation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if _, ok := preflightMessageTarget(w, request, validateNoMessageQuery); !ok {
			return
		}
		conversationID, conversationErr := positiveMessagePathID(request, "conversation_id")
		rootID, rootErr := positiveMessagePathID(request, "root_message_id")
		if conversationErr != nil || rootErr != nil {
			writeMessagePreflightProblem(w, message.ErrInvalidInput)
			return
		}
		next.ServeHTTP(w, withPreparedMessage(request, preparedMessage{conversationID: conversationID, rootMessageID: rootID}))
	})
}

func validateThreadPageQuery(values url.Values) error {
	_, _, err := boundedThreadPage(values)
	return err
}

func boundedThreadPage(values url.Values) (int64, int, error) {
	for key, entries := range values {
		if (key != "after_id" && key != "limit") || len(entries) != 1 || entries[0] == "" {
			return 0, 0, message.ErrInvalidInput
		}
	}
	var afterID int64
	var limit int
	var err error
	if value := values.Get("after_id"); value != "" {
		afterID, err = strconv.ParseInt(value, 10, 64)
		if err != nil || afterID <= 0 {
			return 0, 0, message.ErrInvalidInput
		}
	}
	if value := values.Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit <= 0 || limit > message.MaxPageLimit {
			return 0, 0, message.ErrInvalidInput
		}
	}
	return afterID, limit, nil
}
