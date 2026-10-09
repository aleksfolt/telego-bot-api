package api

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"telego-bot-api/internal/converter"

	"github.com/gotd/td/tgerr"
)

var floodRegex = regexp.MustCompile(`(?:FLOOD_(?:PREMIUM_)?WAIT_|SLOWMODE_WAIT_)(\d+)`)
var migrateRegex = regexp.MustCompile(`MIGRATE_TO_(\d+)`)

// unauthorizedRegex matches only RPC errors meaning the bot itself is not authorized.
// It must not match longer codes that merely contain these words, such as
// PAYMENT_PROVIDER_TOKEN_INVALID, because a 401 makes the server reset the session.
var unauthorizedRegex = regexp.MustCompile(`(?:^|[^A-Z_])(?:ACCESS_TOKEN_INVALID|ACCESS_TOKEN_EXPIRED|BOT_TOKEN_INVALID|EXPIRED_BOT_TOKEN|SESSION_REVOKED|AUTH_KEY_UNREGISTERED)(?:$|[^A-Z_])`)

// MapRpcError converts an internal MTProto / gotd error into standard Telegram Bot API
// HTTP status code, description, and optional ResponseParameters.
func MapRpcError(err error) (int, string, *converter.ResponseParameters) {
	if err == nil {
		return 200, "OK", nil
	}

	// 1. Check for FLOOD_WAIT via gotd helper
	if d, ok := tgerr.AsFloodWait(err); ok {
		sec := int(math.Ceil(d.Seconds()))
		if sec <= 0 {
			sec = 1
		}
		return 429, fmt.Sprintf("Too Many Requests: retry after %d", sec), &converter.ResponseParameters{
			RetryAfter: sec,
		}
	}

	// 2. Context timeout / cancellation
	if errors.Is(err, context.DeadlineExceeded) {
		return 504, "Gateway Timeout: request timed out", nil
	}

	// 3. Inspect gotd RPC error
	if rpcErr, ok := tgerr.As(err); ok {
		// The business middleware wraps this RPC error with context. Unwrapping
		// it must not turn a failed business connection into an invalid bot token.
		if rpcErr.Message == "AUTH_KEY_UNREGISTERED" && strings.Contains(strings.ToLower(err.Error()), "business") {
			return 400, "Bad Request: BUSINESS_CONNECTION_INVALID", nil
		}
		// Telegram's own 401 class means the bot itself is not authorized.
		if rpcErr.Code == 401 && !strings.Contains(strings.ToLower(err.Error()), "business") {
			return 401, "Unauthorized: bot token is invalid or revoked", nil
		}
		return MapErrorString(rpcErr.Message)
	}

	return MapErrorString(err.Error())
}

// MapErrorString converts an error message string into standard Bot API HTTP status and description.
func MapErrorString(desc string) (int, string, *converter.ResponseParameters) {
	// 1. Flood wait
	if matches := floodRegex.FindStringSubmatch(desc); len(matches) == 2 {
		sec, _ := strconv.Atoi(matches[1])
		if sec <= 0 {
			sec = 1
		}
		return 429, fmt.Sprintf("Too Many Requests: retry after %d", sec), &converter.ResponseParameters{
			RetryAfter: sec,
		}
	}

	// 2. Chat migration (group -> supergroup)
	if matches := migrateRegex.FindStringSubmatch(desc); len(matches) == 2 {
		id, _ := strconv.ParseInt(matches[1], 10, 64)
		var supergroupID int64
		if id > 0 {
			supergroupID = -1000000000000 - id
		} else {
			supergroupID = id
		}
		return 400, "Bad Request: group chat was upgraded to a supergroup chat", &converter.ResponseParameters{
			MigrateToChatID: supergroupID,
		}
	}

	// 3. Unauthorized errors (401)
	if unauthorizedRegex.MatchString(desc) && !strings.Contains(strings.ToLower(desc), "business") {
		return 401, "Unauthorized: bot token is invalid or revoked", nil
	}

	// 4. Forbidden errors (403)
	if strings.Contains(desc, "BOT_BLOCKED") || strings.Contains(desc, "USER_IS_BLOCKED") {
		return 403, "Forbidden: bot was blocked by the user", nil
	}
	if strings.Contains(desc, "USER_DEACTIVATED") {
		return 403, "Forbidden: user is deactivated", nil
	}
	if strings.Contains(desc, "CHAT_WRITE_FORBIDDEN") || strings.Contains(desc, "CHAT_SEND_PLAIN_FORBIDDEN") {
		return 403, "Forbidden: not enough rights to send text messages to the chat", nil
	}

	// 5. Bad Request errors (400)
	switch {
	case strings.Contains(desc, "CHAT_NOT_FOUND") || strings.Contains(desc, "PEER_ID_INVALID"):
		return 400, "Bad Request: chat not found", nil
	case strings.Contains(desc, "CHANNEL_PRIVATE"):
		return 400, "Bad Request: channel is private or access denied", nil
	case strings.Contains(desc, "CHAT_ADMIN_REQUIRED"):
		return 400, "Bad Request: not enough rights to manage chat", nil
	case strings.Contains(desc, "USER_NOT_PARTICIPANT"):
		return 400, "Bad Request: user not found in chat", nil
	case strings.Contains(desc, "MESSAGE_NOT_MODIFIED"):
		return 400, "Bad Request: message is not modified: specified new message content and reply markup are exactly the same as a current content and reply markup of the message", nil
	case strings.Contains(desc, "MESSAGE_ID_INVALID"):
		return 400, "Bad Request: message to edit not found", nil
	case strings.Contains(desc, "MESSAGE_EMPTY"):
		return 400, "Bad Request: message text is empty", nil
	case strings.Contains(desc, "MESSAGE_TOO_LONG"):
		return 400, "Bad Request: message is too long", nil
	case strings.Contains(desc, "MEDIA_CAPTION_TOO_LONG"):
		return 400, "Bad Request: media caption is too long", nil
	case strings.Contains(desc, "TOPIC_CLOSED"):
		return 400, "Bad Request: message thread is closed", nil
	case strings.Contains(desc, "TOPIC_DELETED"):
		return 400, "Bad Request: message thread not found", nil
	case strings.Contains(desc, "BUTTON_URL_INVALID"):
		return 400, "Bad Request: BUTTON_URL_INVALID", nil
	case strings.Contains(desc, "BUTTON_DATA_INVALID"):
		return 400, "Bad Request: BUTTON_DATA_INVALID", nil
	case strings.Contains(desc, "BUSINESS_CONNECTION_INVALID") ||
		(strings.Contains(desc, "AUTH_KEY_UNREGISTERED") && strings.Contains(strings.ToLower(desc), "business")):
		return 400, "Bad Request: BUSINESS_CONNECTION_INVALID", nil
	case strings.Contains(desc, "BUSINESS_PEER_INVALID"):
		return 400, "Bad Request: BUSINESS_PEER_INVALID", nil
	}

	// 6. Transient failures must not look like a permanent Bad Request: clients decide
	// on the status code whether to retry (e.g. a subscription check treats a 400 as
	// "not subscribed" but skips the check on 5xx).
	if code, text, ok := mapTransientError(desc); ok {
		return code, text, nil
	}

	// Clean up "rpc error code 400: ..." prefix if present
	clean := desc
	if strings.HasPrefix(clean, "rpc error code ") {
		parts := strings.SplitN(clean, ": ", 2)
		if len(parts) == 2 {
			clean = parts[1]
		}
	}
	if !strings.HasPrefix(clean, "Bad Request: ") && !strings.HasPrefix(clean, "Forbidden: ") && !strings.HasPrefix(clean, "Too Many Requests: ") {
		clean = "Bad Request: " + clean
	}

	return 400, clean, nil
}

var telegramServerErrorRegex = regexp.MustCompile(`rpc error code (?:5\d\d|-50[0-9])\b`)

// mapTransientError recognizes errors caused by timeouts, MTProto reconnects and
// Telegram server-side failures, which succeed when retried.
func mapTransientError(desc string) (int, string, bool) {
	lower := strings.ToLower(desc)
	switch {
	case strings.Contains(lower, "context deadline exceeded") || strings.Contains(lower, "i/o timeout"):
		return 504, "Gateway Timeout: request to Telegram timed out", true
	case strings.Contains(lower, "engine forcibly closed") ||
		strings.Contains(lower, "connection closed") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "broken pipe"):
		return 502, "Bad Gateway: connection to Telegram was interrupted, retry the request", true
	case telegramServerErrorRegex.MatchString(desc):
		clean := desc
		if i := strings.Index(desc, "rpc error code "); i >= 0 {
			clean = desc[i:]
		}
		return 500, "Internal Server Error: " + clean, true
	}
	return 0, "", false
}
