package mock

import (
	"errors"
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/caos/oidc/pkg/op"
	op2 "github.com/caos/oidc/pkg/op"
)

func NewStorage(t *testing.T) op2.Storage {
	return NewMockStorage(gomock.NewController(t))
}

func NewMockStorageExpectValidClientID(t *testing.T) op2.Storage {
	m := NewStorage(t)
	ExpectValidClientID(m)
	return m
}

func NewMockStorageExpectInvalidClientID(t *testing.T) op2.Storage {
	m := NewStorage(t)
	ExpectInvalidClientID(m)
	return m
}

func NewMockStorageAny(t *testing.T) op2.Storage {
	m := NewStorage(t)
	mockS := m.(*MockStorage)
	mockS.EXPECT().GetClientByClientID(gomock.Any()).AnyTimes().Return(&ConfClient{}, nil)
	mockS.EXPECT().AuthorizeClientIDSecret(gomock.Any(), gomock.Any()).AnyTimes().Return(&ConfClient{}, nil)
	return m
}

func ExpectInvalidClientID(s op2.Storage) {
	mockS := s.(*MockStorage)
	mockS.EXPECT().GetClientByClientID(gomock.Any()).Return(nil, errors.New("client not found"))
}

func ExpectValidClientID(s op2.Storage) {
	mockS := s.(*MockStorage)
	mockS.EXPECT().GetClientByClientID(gomock.Any()).DoAndReturn(
		func(id string) (op.Client, error) {
			var appType op.ApplicationType
			switch id {
			case "web_client":
				appType = op.ApplicationTypeWeb
			case "native_client":
				appType = op.ApplicationTypeNative
			case "useragent_client":
				appType = op.ApplicationTypeUserAgent
			}
			return &ConfClient{appType: appType}, nil
		})
}

type ConfClient struct {
	appType op.ApplicationType
}

func (c *ConfClient) RedirectURIs() []string {
	return []string{
		"https://registered.com/callback",
		"http://registered.com/callback",
		"http://localhost:9999/callback",
		"custom://callback",
	}
}

func (c *ConfClient) LoginURL(id string) string {
	return "login?id=" + id
}

func (c *ConfClient) ApplicationType() op.ApplicationType {
	return c.appType
}
