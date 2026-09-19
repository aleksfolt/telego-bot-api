package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"reflect"
	"strconv"
	"strings"

	"github.com/valyala/fasthttp"
)

// bindRequest universally extracts and binds request parameters from:
// 1. application/json body
// 2. application/x-www-form-urlencoded
// 3. multipart/form-data
// 4. URL query parameters (GET or POST)
func bindRequest(ctx *fasthttp.RequestCtx, target interface{}) error {
	body := bytes.TrimSpace(ctx.PostBody())
	isJSONBody := len(body) > 0 && (body[0] == '{' || body[0] == '[')

	// If it's a pure JSON body and no query parameters exist, try direct unmarshal first for speed
	if isJSONBody && ctx.QueryArgs().Len() == 0 {
		if err := json.Unmarshal(body, target); err == nil {
			return nil
		}
	}

	fields := requestFieldTypes(target)
	parse := func(k, val string) interface{} {
		switch k {
		case "thumb":
			k = "thumbnail"
		case "png_sticker":
			k = "sticker"
		}
		return parseArgValue(val, fields[k])
	}

	// Universal extraction into intermediate map
	data := make(map[string]interface{})

	// 1. Parse URL Query parameters
	ctx.QueryArgs().VisitAll(func(k, v []byte) {
		key := string(k)
		data[key] = parse(key, string(v))
	})

	// 2. Parse form-urlencoded POST arguments
	ctx.PostArgs().VisitAll(func(k, v []byte) {
		key := string(k)
		data[key] = parse(key, string(v))
	})

	// 3. Parse multipart/form-data if present
	if mf, err := ctx.MultipartForm(); err == nil && mf != nil {
		for k, vals := range mf.Value {
			if len(vals) > 0 {
				data[k] = parse(k, vals[0])
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
						data[pName] = parse(pName, string(valBytes))
					}
				}
			}
		}
	}

	// 4. If body was JSON, overlay parsed JSON fields onto data
	if isJSONBody {
		if !json.Valid(body) {
			return fmt.Errorf("invalid JSON arguments")
		}
		var bodyMap map[string]interface{}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		if err := decoder.Decode(&bodyMap); err != nil {
			return fmt.Errorf("decode JSON arguments: %w", err)
		}
		for k, v := range bodyMap {
			if str, ok := v.(string); ok {
				v = parse(k, str)
			}
			data[k] = v
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

// Derive coercion rules from the request model, never from the value's spelling.
func requestFieldTypes(target interface{}) map[string]reflect.Type {
	fields := make(map[string]reflect.Type)
	typ := reflect.TypeOf(target)
	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil || typ.Kind() != reflect.Struct {
		return fields
	}
	for _, field := range reflect.VisibleFields(typ) {
		if !field.IsExported() {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		fields[name] = field.Type
	}
	return fields
}

func parseArgValue(val string, typ reflect.Type) interface{} {
	if typ == nil {
		return val
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	trimmed := strings.TrimSpace(val)
	switch typ.Kind() {
	case reflect.String:
		return val
	case reflect.Bool:
		if v, err := strconv.ParseBool(trimmed); err == nil {
			return v
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return v
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v, err := strconv.ParseUint(trimmed, 10, 64); err == nil {
			return v
		}
	case reflect.Float32, reflect.Float64:
		if v, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return v
		}
	case reflect.Struct, reflect.Map, reflect.Slice, reflect.Array, reflect.Interface:
		var v interface{}
		if json.Valid([]byte(trimmed)) {
			decoder := json.NewDecoder(strings.NewReader(trimmed))
			decoder.UseNumber()
			if err := decoder.Decode(&v); err == nil {
				return v
			}
		}
	}
	return val
}
