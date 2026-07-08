package search

import (
	"encoding/base64"
)

// YouTube InnerTube API constants and payload builders
// Reference: https://github.com/zaidkx37/tubescrape

const (
	// InnerTube API endpoints
	searchURL = "https://www.youtube.com/youtubei/v1/search"

	// YouTube WEB client context
	webClientName    = "WEB"
	webClientVersion = "2.20260227.01.00"
	webPlatform      = "DESKTOP"
	webUserAgent     = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36"
)

// SearchFilter protobuf field mappings
var (
	// Sort by options
	sortByMap = map[string]int{
		"relevance":   0,
		"upload_date": 1,
		"date":        1,
		"view_count":  2,
		"views":       2,
		"rating":      3,
	}

	// Upload date filter options
	uploadDateMap = map[string]int{
		"last_hour":  1,
		"hour":       1,
		"today":      2,
		"this_week":  3,
		"week":       3,
		"this_month": 4,
		"month":      4,
		"this_year":  5,
		"year":       5,
	}

	// Type filter options
	typeMap = map[string]int{
		"video":    1,
		"channel":  2,
		"playlist": 3,
		"movie":    4,
	}

	// Duration filter options
	durationMap = map[string]int{
		"short":  1, // Under 4 minutes
		"medium": 3, // 4-20 minutes
		"long":   2, // Over 20 minutes
	}

	// Feature filter options
	featureMap = map[string]int{
		"live":             1,
		"4k":               2,
		"hd":               3,
		"subtitles":        4,
		"cc":               4,
		"creative_commons": 5,
		"360":              6,
		"vr180":            7,
		"3d":               8,
		"hdr":              9,
		"location":         10,
		"purchased":        11,
	}
)

// SearchFilter holds search filter parameters
type SearchFilter struct {
	SortBy     string   // relevance, upload_date, view_count, rating
	UploadDate string   // last_hour, today, this_week, this_month, this_year
	Type       string   // video, channel, playlist, movie
	Duration   string   // short, medium, long
	Features   []string // live, 4k, hd, subtitles, cc, etc.
}

// BuildParams builds a protobuf-encoded search filter string
func (f *SearchFilter) BuildParams() string {
	if f == nil {
		return ""
	}

	var data []byte

	// Field 1: Sort by (varint, field number 1)
	if f.SortBy != "" {
		if val, ok := sortByMap[f.SortBy]; ok && val != 0 {
			data = appendVarintField(data, 1, val)
		}
	}

	// Field 2: Nested filter message
	var filters []byte

	if f.UploadDate != "" {
		if val, ok := uploadDateMap[f.UploadDate]; ok {
			filters = appendVarintField(filters, 1, val)
		}
	}

	if f.Type != "" {
		if val, ok := typeMap[f.Type]; ok {
			filters = appendVarintField(filters, 2, val)
		}
	}

	if f.Duration != "" {
		if val, ok := durationMap[f.Duration]; ok {
			filters = appendVarintField(filters, 3, val)
		}
	}

	for _, feat := range f.Features {
		if val, ok := featureMap[feat]; ok {
			filters = appendVarintField(filters, 4, val)
		}
	}

	if len(filters) > 0 {
		data = appendBytesField(data, 2, filters)
	}

	if len(data) == 0 {
		return ""
	}

	return base64.StdEncoding.EncodeToString(data)
}

// buildSearchPayload builds the InnerTube search request payload
func buildSearchPayload(query string, params string, continuation string) map[string]interface{} {
	payload := map[string]interface{}{
		"context": map[string]interface{}{
			"client": map[string]interface{}{
				"hl":            "en",
				"gl":            "US",
				"clientName":    webClientName,
				"clientVersion": webClientVersion,
				"platform":      webPlatform,
				"userAgent":     webUserAgent,
			},
		},
		"query": query,
	}

	if params != "" {
		payload["params"] = params
	}

	if continuation != "" {
		payload["continuation"] = continuation
	}

	return payload
}

// Protobuf encoding helpers

// appendVarint encodes an integer as a protobuf varint
func appendVarint(buf []byte, value int) []byte {
	for value > 0x7F {
		buf = append(buf, byte((value&0x7F)|0x80))
		value >>= 7
	}
	buf = append(buf, byte(value&0x7F))
	return buf
}

// appendVarintField encodes a varint field (wire type 0)
func appendVarintField(buf []byte, fieldNumber int, value int) []byte {
	tag := (fieldNumber << 3) | 0 // wire type 0 = varint
	buf = appendVarint(buf, tag)
	buf = appendVarint(buf, value)
	return buf
}

// appendBytesField encodes a length-delimited field (wire type 2)
func appendBytesField(buf []byte, fieldNumber int, value []byte) []byte {
	tag := (fieldNumber << 3) | 2 // wire type 2 = length-delimited
	buf = appendVarint(buf, tag)
	buf = appendVarint(buf, len(value))
	buf = append(buf, value...)
	return buf
}
