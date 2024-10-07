package actions

import (
	"lunar/engine/messages"
	"lunar/engine/utils"
	sharedActions "lunar/shared-model/actions"
	"strings"

	spoe "github.com/TheLunarCompany/haproxy-spoe-go"
)

const (
	ReturnEarlyResponseActionName = "return_early_response"
	StatusCodeActionName          = "status_code"
	ResponseBodyActionName        = "response_body"

	ModifyRequestActionName   = "modify_request"
	GenerateRequestActionName = "generate_request"
	RequestHeadersActionName  = "request_headers"
	RequestBodyActionName     = "request_body"

	RequestRunResultName = "request_run_result"
)

// EarlyResponseAction
func (action *EarlyResponseAction) ReqToSpoeActions() []spoe.Action {
	actions := []spoe.Action{
		spoe.ActionSetVar{
			Name:  ReturnEarlyResponseActionName,
			Scope: spoe.VarScopeTransaction,
			Value: true,
		},
		spoe.ActionSetVar{
			Name:  StatusCodeActionName,
			Scope: spoe.VarScopeTransaction,
			Value: action.Status,
		},
		spoe.ActionSetVar{
			Name:  ResponseBodyActionName,
			Scope: spoe.VarScopeTransaction,
			Value: []byte(action.Body),
		},
		spoe.ActionSetVar{
			Name:  ResponseHeadersActionName,
			Scope: spoe.VarScopeTransaction,
			Value: utils.DumpHeaders(action.Headers),
		},
	}

	return actions
}

func (*EarlyResponseAction) ReqRunResult() sharedActions.RemedyReqRunResult {
	return sharedActions.ReqObtainedResponse
}

func (*EarlyResponseAction) EnsureRequestIsUpdated(_ *messages.OnRequest) {
}

// ModifyRequestAction
func (action *ModifyRequestAction) ReqToSpoeActions() []spoe.Action {
	actions := []spoe.Action{
		spoe.ActionSetVar{
			Name:  ModifyRequestActionName,
			Scope: spoe.VarScopeRequest,
			Value: true,
		},
		spoe.ActionSetVar{
			Name:  RequestHeadersActionName,
			Scope: spoe.VarScopeRequest,
			Value: utils.DumpHeaders(action.HeadersToSet),
		},
	}
	return actions
}

func (action *ModifyRequestAction) ReqRunResult() sharedActions.RemedyReqRunResult {
	return sharedActions.ReqModifiedRequest
}

func (action *ModifyRequestAction) EnsureRequestIsUpdated(
	onRequest *messages.OnRequest,
) {
	for name, value := range action.HeadersToSet {
		onRequest.Headers[name] = value
	}
}

func (action *GenerateRequestAction) ReqToSpoeActions() []spoe.Action {
	actions := []spoe.Action{
		spoe.ActionSetVar{
			Name:  GenerateRequestActionName,
			Scope: spoe.VarScopeRequest,
			Value: true,
		},
		spoe.ActionSetVar{
			Name:  RequestHeadersActionName,
			Scope: spoe.VarScopeRequest,
			Value: utils.DumpHeaders(action.HeadersToSet),
		},
		spoe.ActionSetVar{
			Name:  RequestBodyActionName,
			Scope: spoe.VarScopeRequest,
			Value: []byte(action.Body),
		},
	}
	return actions
}

func (action *GenerateRequestAction) ReqRunResult() sharedActions.RemedyReqRunResult {
	return sharedActions.ReqGenerateRequest
}

func (action *GenerateRequestAction) EnsureRequestIsUpdated(
	onRequest *messages.OnRequest,
) {
	for name, value := range onRequest.Headers {
		delete(onRequest.Headers, name)
		onRequest.Headers[strings.ToLower(name)] = value
	}

	for name, value := range action.HeadersToSet {
		onRequest.Headers[strings.ToLower(name)] = value
	}

	for _, value := range action.HeadersToRemove {
		delete(onRequest.Headers, value)
	}
}
