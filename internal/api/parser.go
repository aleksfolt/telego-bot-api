package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"strconv"
	"strings"

	"github.com/valyala/fasthttp"
)

func isStringField(k string) bool {
	return k == "business_connection_id" || k == "file_id" || strings.HasSuffix(k, "file_id") || k == "custom_emoji_id"
}

// bindRequest universally extracts and binds request parameters from:
// 1. application/json body
// 2. application/x-www-form-urlencoded
// 3. multipart/form-data
// 4. URL query parameters (GET or POST)
func bindRequest(ctx *fasthttp.RequestCtx, target interface{}) error {
	body := ctx.PostBody()
	isJSONBody := len(body) > 0 && (body[0] == '{' || body[0] == '[')

	// If it's a pure JSON body and no query parameters exist, try direct unmarshal first for speed
	if isJSONBody && ctx.QueryArgs().Len() == 0 {
		if err := json.Unmarshal(body, target); err == nil {
			return nil
		}
	}

	// Universal extraction into intermediate map
	data := make(map[string]interface{})

	// 1. Parse URL Query parameters
	ctx.QueryArgs().VisitAll(func(k, v []byte) {
		key := string(k)
		data[key] = parseArgValue(key, string(v))
	})

	// 2. Parse form-urlencoded POST arguments
	ctx.PostArgs().VisitAll(func(k, v []byte) {
		key := string(k)
		data[key] = parseArgValue(key, string(v))
	})

	// 3. Parse multipart/form-data if present
	if mf, err := ctx.MultipartForm(); err == nil && mf != nil {
		for k, vals := range mf.Value {
			if len(vals) > 0 {
				data[k] = parseArgValue(k, vals[0])
			}
		}
	}

	// Fallback for multipart/form-data values if raw boundary exists
	if boundary := ctx.Request.Header.MultipartFormBoundary(); len(boundary) > 0 {
		mr := multipart.NewReader(bytes.NewReader(body), string(boundary))
		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}
			cd := p.Header.Get("Content-Disposition")
			pName, pFilename := ParseContentDisposition(cd)
			if pName != "" && pFilename == "" {
				if _, exists := data[pName]; !exists {
					if valBytes, err := io.ReadAll(p); err == nil {
						data[pName] = parseArgValue(pName, string(valBytes))
					}
				}
			}
		}
	}

	// 4. If body was JSON, overlay parsed JSON fields onto data
	if isJSONBody {
		var bodyMap map[string]interface{}
		if err := json.Unmarshal(body, &bodyMap); err == nil {
			for k, v := range bodyMap {
				if str, ok := v.(string); ok {
					// Normalize numeric strings for ID fields
					if !isStringField(k) {
						if n, err := strconv.ParseInt(str, 10, 64); err == nil &&
							(strings.HasSuffix(k, "_id") || strings.HasSuffix(k, "id") || k == "offset" || k == "limit") {
							data[k] = n
							continue
						}
					}
					// Normalize JSON strings in reply_markup
					trimmed := strings.TrimSpace(str)
					if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
						(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
						var nested interface{}
						if err := json.Unmarshal([]byte(trimmed), &nested); err == nil {
							data[k] = nested
							continue
						}
					}
				}
				data[k] = v
			}
		}
	}

	// Alias 'thumb' to 'thumbnail' for backward compatibility
	if t, ok := data["thumb"]; ok {
		if _, hasThumb := data["thumbnail"]; !hasThumb {
			data["thumbnail"] = t
		}
	}
	// Alias 'png_sticker' to 'sticker' for backward compatibility
	if s, ok := data["png_sticker"]; ok {
		if _, hasSticker := data["sticker"]; !hasSticker {
			data["sticker"] = s
		}
	}

	if len(data) == 0 && len(body) == 0 {
		return nil
	}

	// Marshal combined map to JSON and unmarshal into target struct
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal arguments: %w", err)
	}

	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("bind arguments: %w", err)
	}

	return nil
}

func parseArgValue(k string, val string) interface{} {
	trimmed := strings.TrimSpace(val)
	if trimmed == "true" {
		return true
	}
	if trimmed == "false" {
		return false
	}
	// Check if JSON object or array (e.g. reply_markup, link_preview_options, allowed_updates)
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		var obj interface{}
		if err := json.Unmarshal([]byte(trimmed), &obj); err == nil {
			return obj
		}
	}
	if !isStringField(k) {
		// Try integer
		if n, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return n
		}
		// Try float
		if f, err := strconv.ParseFloat(trimmed, 64); err == nil && strings.Contains(trimmed, ".") {
			return f
		}
	}
	return val
}
