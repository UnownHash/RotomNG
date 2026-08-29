// Package testutil provides test utilities and builders for integration testing.
package testutil

import (
	"github.com/google/uuid"

	"github.com/UnownHash/RotomNG/libs/protos"
)

// Default test constants shared across fake devices, workers, and controllers.
const (
	defaultTestOrigin  = "test-origin"
	defaultTestVersion = "1.0.0"
	defaultTestUser    = "test-user"
	defaultTestAddr    = "127.0.0.1:0"
)

// ---------------------------------------------------------------------------
// WelcomeMessage builder
// ---------------------------------------------------------------------------

// WelcomeMessageOption is a functional option for configuring a WelcomeMessage.
type WelcomeMessageOption func(*protos.WelcomeMessage)

// WithWelcomeWorkerID sets the WorkerId field.
func WithWelcomeWorkerID(id string) WelcomeMessageOption {
	return func(m *protos.WelcomeMessage) { m.WorkerId = id }
}

// WithWelcomeOrigin sets the Origin field.
func WithWelcomeOrigin(origin string) WelcomeMessageOption {
	return func(m *protos.WelcomeMessage) { m.Origin = origin }
}

// WithWelcomeDeviceID sets the DeviceId field.
func WithWelcomeDeviceID(id string) WelcomeMessageOption {
	return func(m *protos.WelcomeMessage) { m.DeviceId = id }
}

// WithWelcomePlatform sets the Platform field.
func WithWelcomePlatform(p protos.WelcomeMessage_Platform) WelcomeMessageOption {
	return func(m *protos.WelcomeMessage) { m.Platform = p }
}

// WithWelcomeVersionCode sets the VersionCode field.
func WithWelcomeVersionCode(v int32) WelcomeMessageOption {
	return func(m *protos.WelcomeMessage) { m.VersionCode = v }
}

// WithWelcomeVersionName sets the VersionName field.
func WithWelcomeVersionName(v string) WelcomeMessageOption {
	return func(m *protos.WelcomeMessage) { m.VersionName = v }
}

// WithWelcomeUserAgent sets the Useragent field.
func WithWelcomeUserAgent(ua string) WelcomeMessageOption {
	return func(m *protos.WelcomeMessage) { m.Useragent = ua }
}

// BuildWelcomeMessage constructs a WelcomeMessage with sensible defaults.
// Functional options override specific fields.
func BuildWelcomeMessage(opts ...WelcomeMessageOption) *protos.WelcomeMessage {
	msg := &protos.WelcomeMessage{
		WorkerId:    uuid.New().String(),
		Origin:      defaultTestOrigin,
		VersionCode: 1,
		VersionName: defaultTestVersion,
		Useragent:   "fake-worker",
		Platform:    protos.WelcomeMessage_ANDROID,
	}
	for _, opt := range opts {
		opt(msg)
	}
	return msg
}

// ---------------------------------------------------------------------------
// MitmRequest builder
// ---------------------------------------------------------------------------

// MitmRequestOption is a functional option for configuring a MitmRequest.
type MitmRequestOption func(*protos.MitmRequest)

// WithRequestID sets the Id field.
func WithRequestID(id uint32) MitmRequestOption {
	return func(m *protos.MitmRequest) { m.Id = id }
}

// WithMethod sets the Method field.
func WithMethod(method protos.MitmRequest_Method) MitmRequestOption {
	return func(m *protos.MitmRequest) { m.Method = method }
}

// WithLoginPayload sets the LoginRequest payload.
func WithLoginPayload(lr *protos.MitmRequest_LoginRequest) MitmRequestOption {
	return func(m *protos.MitmRequest) {
		m.Payload = &protos.MitmRequest_LoginRequest_{LoginRequest: lr}
	}
}

// WithRPCPayload sets the RpcRequest payload.
func WithRPCPayload(rr *protos.MitmRequest_RpcRequest) MitmRequestOption {
	return func(m *protos.MitmRequest) {
		m.Payload = &protos.MitmRequest_RpcRequest_{RpcRequest: rr}
	}
}

// BuildMitmRequest constructs a MitmRequest with sensible defaults.
func BuildMitmRequest(opts ...MitmRequestOption) *protos.MitmRequest {
	msg := &protos.MitmRequest{
		Id:     1,
		Method: protos.MitmRequest_UNSET,
	}
	for _, opt := range opts {
		opt(msg)
	}
	return msg
}

// ---------------------------------------------------------------------------
// LoginRequest builder (returns a complete MitmRequest with LOGIN payload)
// ---------------------------------------------------------------------------

// LoginRequestOption is a functional option for configuring the LoginRequest
// payload inside a MitmRequest.
type LoginRequestOption func(*protos.MitmRequest_LoginRequest)

// WithLoginWorkerID sets the WorkerId field in the LoginRequest.
func WithLoginWorkerID(id string) LoginRequestOption {
	return func(lr *protos.MitmRequest_LoginRequest) { lr.WorkerId = id }
}

// WithLoginUsername sets the Username field in the LoginRequest.
func WithLoginUsername(u string) LoginRequestOption {
	return func(lr *protos.MitmRequest_LoginRequest) { lr.Username = u }
}

// WithLoginSource sets the Source field in the LoginRequest.
func WithLoginSource(s protos.MitmRequest_LoginRequest_LoginSource) LoginRequestOption {
	return func(lr *protos.MitmRequest_LoginRequest) { lr.Source = s }
}

// BuildLoginRequest constructs a MitmRequest with Method=LOGIN and a
// LoginRequest payload with sensible defaults.
func BuildLoginRequest(opts ...LoginRequestOption) *protos.MitmRequest {
	lr := &protos.MitmRequest_LoginRequest{
		WorkerId: uuid.New().String(),
		Username: defaultTestUser,
		//nolint:staticcheck
		Source: protos.MitmRequest_LoginRequest_PTC,
	}
	for _, opt := range opts {
		opt(lr)
	}
	return &protos.MitmRequest{
		Id:      1,
		Method:  protos.MitmRequest_LOGIN,
		Payload: &protos.MitmRequest_LoginRequest_{LoginRequest: lr},
	}
}

// ---------------------------------------------------------------------------
// MitmResponse builder
// ---------------------------------------------------------------------------

// MitmResponseOption is a functional option for configuring a MitmResponse.
type MitmResponseOption func(*protos.MitmResponse)

// WithResponseID sets the Id field.
func WithResponseID(id uint32) MitmResponseOption {
	return func(m *protos.MitmResponse) { m.Id = id }
}

// WithResponseStatus sets the Status field.
func WithResponseStatus(s protos.MitmResponse_Status) MitmResponseOption {
	return func(m *protos.MitmResponse) { m.Status = s }
}

// WithLoginResponse sets the LoginResponse payload.
func WithLoginResponse(lr *protos.MitmResponse_LoginResponse) MitmResponseOption {
	return func(m *protos.MitmResponse) {
		m.Payload = &protos.MitmResponse_LoginResponse_{LoginResponse: lr}
	}
}

// BuildMitmResponse constructs a MitmResponse with sensible defaults.
func BuildMitmResponse(opts ...MitmResponseOption) *protos.MitmResponse {
	msg := &protos.MitmResponse{
		Id:     1,
		Status: protos.MitmResponse_SUCCESS,
	}
	for _, opt := range opts {
		opt(msg)
	}
	return msg
}

// ---------------------------------------------------------------------------
// RegisterControllerRequest builder
// ---------------------------------------------------------------------------

// RegisterControllerRequestOption is a functional option for configuring a
// RegisterControllerRequest.
type RegisterControllerRequestOption func(*protos.RegisterControllerRequest)

// WithRegControllerID sets the Id field.
func WithRegControllerID(id string) RegisterControllerRequestOption {
	return func(m *protos.RegisterControllerRequest) { m.Id = id }
}

// WithRegWeight sets the Weight field.
func WithRegWeight(w int32) RegisterControllerRequestOption {
	return func(m *protos.RegisterControllerRequest) { m.Weight = w }
}

// WithRegProtoMajorVersion sets the ProtoMajorVersion field.
func WithRegProtoMajorVersion(v int32) RegisterControllerRequestOption {
	return func(m *protos.RegisterControllerRequest) { m.ProtoMajorVersion = v }
}

// WithRegProtoMinorVersion sets the ProtoMinorVersion field.
func WithRegProtoMinorVersion(v int32) RegisterControllerRequestOption {
	return func(m *protos.RegisterControllerRequest) { m.ProtoMinorVersion = v }
}

// BuildRegisterControllerRequest constructs a RegisterControllerRequest with
// sensible defaults.
func BuildRegisterControllerRequest(opts ...RegisterControllerRequestOption) *protos.RegisterControllerRequest {
	msg := &protos.RegisterControllerRequest{
		Id:                uuid.New().String(),
		ProtoMajorVersion: 2,
		ProtoMinorVersion: 0,
		Weight:            0,
	}
	for _, opt := range opts {
		opt(msg)
	}
	return msg
}
