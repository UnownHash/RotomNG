package testutil

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/UnownHash/RotomNG/libs/protos"
)

func TestBuildWelcomeMessageDefaults(t *testing.T) {
	msg := BuildWelcomeMessage()
	if msg.WorkerId == "" {
		t.Fatal("expected non-empty WorkerId")
	}
	if msg.Origin != "test-origin" {
		t.Fatalf("expected Origin 'test-origin', got %q", msg.Origin)
	}
	if msg.Platform != protos.WelcomeMessage_ANDROID {
		t.Fatalf("expected Platform ANDROID, got %v", msg.Platform)
	}
	if msg.VersionCode != 1 {
		t.Fatalf("expected VersionCode 1, got %d", msg.VersionCode)
	}
	if msg.VersionName != "1.0.0" {
		t.Fatalf("expected VersionName '1.0.0', got %q", msg.VersionName)
	}
}

func TestBuildWelcomeMessageWithOption(t *testing.T) {
	msg := BuildWelcomeMessage(WithWelcomeWorkerID("custom"))
	if msg.WorkerId != "custom" {
		t.Fatalf("expected WorkerId 'custom', got %q", msg.WorkerId)
	}
}

func TestBuildMitmRequestDefaults(t *testing.T) {
	req := BuildMitmRequest()
	if req.Id == 0 {
		t.Fatal("expected Id > 0")
	}
	if req.Method != protos.MitmRequest_UNSET {
		t.Fatalf("expected Method UNSET, got %v", req.Method)
	}
}

func TestBuildMitmRequestWithMethod(t *testing.T) {
	req := BuildMitmRequest(WithMethod(protos.MitmRequest_LOGIN))
	if req.Method != protos.MitmRequest_LOGIN {
		t.Fatalf("expected Method LOGIN, got %v", req.Method)
	}
}

func TestBuildLoginRequestDefaults(t *testing.T) {
	req := BuildLoginRequest()
	if req.Method != protos.MitmRequest_LOGIN {
		t.Fatalf("expected Method LOGIN, got %v", req.Method)
	}
	lr := req.GetLoginRequest()
	if lr == nil {
		t.Fatal("expected LoginRequest payload, got nil")
	}
	if lr.WorkerId == "" {
		t.Fatal("expected non-empty WorkerId in LoginRequest")
	}
	if lr.Username != "test-user" {
		t.Fatalf("expected Username 'test-user', got %q", lr.Username)
	}
	//nolint:staticcheck
	if lr.Source != protos.MitmRequest_LoginRequest_PTC {
		t.Fatalf("expected Source PTC, got %v", lr.Source)
	}
}

func TestBuildMitmResponseDefaults(t *testing.T) {
	resp := BuildMitmResponse()
	if resp.Status != protos.MitmResponse_SUCCESS {
		t.Fatalf("expected Status SUCCESS, got %v", resp.Status)
	}
}

func TestBuildMitmResponseWithStatus(t *testing.T) {
	resp := BuildMitmResponse(WithResponseStatus(protos.MitmResponse_ERROR_UNKNOWN))
	if resp.Status != protos.MitmResponse_ERROR_UNKNOWN {
		t.Fatalf("expected Status ERROR_UNKNOWN, got %v", resp.Status)
	}
}

func TestBuildRegisterControllerRequestDefaults(t *testing.T) {
	req := BuildRegisterControllerRequest()
	if req.Id == "" {
		t.Fatal("expected non-empty Id")
	}
	if req.ProtoMajorVersion != 2 {
		t.Fatalf("expected ProtoMajorVersion 2, got %d", req.ProtoMajorVersion)
	}
	if req.ProtoMinorVersion != 0 {
		t.Fatalf("expected ProtoMinorVersion 0, got %d", req.ProtoMinorVersion)
	}
}

func TestBuildersProduceMarshalableMessages(t *testing.T) {
	messages := []proto.Message{
		BuildWelcomeMessage(),
		BuildMitmRequest(),
		BuildLoginRequest(),
		BuildMitmResponse(),
		BuildRegisterControllerRequest(),
	}
	for i, msg := range messages {
		if _, err := proto.Marshal(msg); err != nil {
			t.Fatalf("message %d failed to marshal: %v", i, err)
		}
	}
}
