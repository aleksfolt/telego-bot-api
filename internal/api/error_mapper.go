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

var floodRegex = regexp.MustCompile(`FLOOD_(?:PREMIUM_)?WAIT_(\d+)`)
var migrateRegex = regexp.MustCompile(`MIGRATE_TO_(\d+)`)

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

	// 3. Forbidden errors (403)
	if strings.Contains(desc, "BOT_BLOCKED") || strings.Contains(desc, "USER_IS_BLOCKED") {
		return 403, "Forbidden: bot was blocked by the user", nil
	}
	if strings.Contains(desc, "USER_DEACTIVATED") {
		return 403, "Forbidden: user is deactivated", nil
	}

	// 4. Bad Request errors (400)
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
	case strings.Contains(desc, "BUTTON_URL_INVALID"):
		return 400, "Bad Request: BUTTON_URL_INVALID", nil
	case strings.Contains(desc, "BUTTON_DATA_INVALID"):
		return 400, "Bad Request: BUTTON_DATA_INVALID", nil
	case strings.Contains(desc, "BUSINESS_CONNECTION_INVALID") || strings.Contains(desc, "AUTH_KEY_UNREGISTERED"):
		return 400, "Bad Request: BUSINESS_CONNECTION_INVALID", nil
	case strings.Contains(desc, "BUSINESS_PEER_INVALID"):
		return 400, "Bad Request: BUSINESS_PEER_INVALID", nil
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
