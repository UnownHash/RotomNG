package protos

import (
	"fmt"
	"strconv"
	"strings"
)

// MITMRequestMethodName returns the human-readable name of a MitmRequest method.
func MITMRequestMethodName(method MitmRequest_Method) string {
	name, ok := MitmRequest_Method_name[int32(method)]
	if !ok {
		name = strconv.Itoa(int(method))
	}
	return name
}

// MITMResponseStatusName returns the human-readable name of a MitmResponse status.
func MITMResponseStatusName(status MitmResponse_Status) string {
	name, ok := MitmResponse_Status_name[int32(status)]
	if !ok {
		name = strconv.Itoa(int(status))
	}
	return name
}

// RPCStatusName returns the human-readable name of an RPC status code.
func RPCStatusName(status RpcStatus) string {
	name, ok := RpcStatus_name[int32(status)]
	if !ok {
		name = strconv.Itoa(int(status))
	}
	return name
}

// GetMethodsForRPCRequests returns whether GMO/combat methods are present and a
// string representation of all method IDs in the given RPC requests.
func GetMethodsForRPCRequests(requests []*MitmRequest_RpcRequest_SingleRpcRequest) (bool, bool, string) {
	var builder strings.Builder
	var hasGmo, hasCombat bool

	builder.WriteByte('[')
	for i, request := range requests {
		if i > 0 {
			builder.WriteByte(',')
		}
		switch request.Method {
		case 106:
			hasGmo = true
		case 992:
			hasCombat = true
		}
		builder.WriteString(strconv.Itoa(int(request.Method)))
	}
	builder.WriteByte(']')
	return hasGmo, hasCombat, builder.String()
}

// GetMethodsForRPCResponses returns a string representation of all method IDs
// in the given RPC responses.
func GetMethodsForRPCResponses(responses []*MitmResponse_RpcResponse_SingleRpcResponse) string {
	var builder strings.Builder

	builder.WriteByte('[')
	for i, response := range responses {
		if i > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.Itoa(int(response.Method)))
	}
	builder.WriteByte(']')
	return builder.String()
}

// AccountInfo holds the username and source for a MITM login request.
type AccountInfo struct {
	Username string
	Source   string
}

func (accountInfo AccountInfo) String() string {
	return fmt.Sprintf("%s[%s]", accountInfo.Username, accountInfo.Source)
}

// GetAccountInfoFromLoginRequest extracts account information from a login request.
func GetAccountInfoFromLoginRequest(loginRequest *MitmRequest_LoginRequest) AccountInfo {
	const unknown = "<unknown>"
	return AccountInfo{
		Username: func() string {
			if loginRequest.Username == "" {
				return unknown
			}
			return loginRequest.Username
		}(),
		Source: func() string {
			if loginRequest.Source == MitmRequest_LoginRequest_UNSET {
				return unknown
			}
			return loginRequest.Source.String()
		}(),
	}
}
