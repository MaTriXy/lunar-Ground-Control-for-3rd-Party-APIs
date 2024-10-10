package streamtypes

import (
	"fmt"
	"lunar/engine/messages"
	publictypes "lunar/engine/streams/public-types"
	"strconv"
	"time"
)

func NewResponse(onResponse messages.OnResponse) publictypes.TransactionI {
	return &OnResponse{
		id:         onResponse.ID,
		sequenceID: onResponse.SequenceID,
		method:     onResponse.Method,
		url:        onResponse.URL,
		status:     onResponse.Status,
		headers:    onResponse.Headers,
		body:       onResponse.Body,
		time:       onResponse.Time,
	}
}

func (res *OnResponse) IsNewSequence() bool {
	return res.id == res.sequenceID
}

func (res *OnResponse) DoesHeaderExist(headerName string) bool {
	_, found := res.headers[headerName]
	return found
}

func (res *OnResponse) DoesHeaderValueMatch(headerName, headerValue string) bool {
	if !res.DoesHeaderExist(headerName) {
		return false
	}
	return res.headers[headerName] == headerValue
}

func (res *OnResponse) Size() int {
	if res.size > 0 {
		return res.size
	}

	if sizeStr, ok := res.headers["Content-Length"]; ok {
		size, _ := strconv.Atoi(sizeStr)
		res.size = size
		return res.size
	}

	if res.body != "" {
		res.size = len(res.body)
		return len(res.body)
	}
	return 0
}

func (res *OnResponse) DoesQueryParamExist(string) bool {
	return false
}

func (res *OnResponse) DoesQueryParamValueMatch(string, string) bool {
	return false
}

func (res *OnResponse) GetID() string {
	return res.id
}

func (res *OnResponse) GetSequenceID() string {
	return res.sequenceID
}

func (res *OnResponse) GetMethod() string {
	return res.method
}

func (res *OnResponse) GetURL() string {
	return res.url
}

func (res *OnResponse) GetStatus() int {
	return res.status
}

func (res *OnResponse) GetHeader(key string) (string, bool) {
	value, found := res.headers[key]
	if !found {
		return "", false
	}
	return value, true
}

func (res *OnResponse) GetHeaders() map[string]string {
	return res.headers
}

func (res *OnResponse) GetBody() string {
	return res.body
}

func (res *OnResponse) GetTime() time.Time {
	return res.time
}

// NewResponseAPIStream creates a new APIStream with the given OnResponse
func NewResponseAPIStream(onResponse messages.OnResponse) publictypes.APIStreamI {
	name := fmt.Sprintf("ResponseAPIStream-%s", onResponse.ID)
	apiStream := NewAPIStream(name, publictypes.StreamTypeResponse)
	apiStream.SetResponse(NewResponse(onResponse))
	return apiStream
}
