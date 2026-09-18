package newapimanagement_test

import (
	"context"
	"testing"
)

func TestChannelGroupReadRejectsMismatchedIdentityWithoutPartialSelection(t *testing.T) {
	fixture := newChannelFixture(t)
	result, err := fixture.service.ChannelsByID(context.Background(), "primary", []string{"42", "99"}, 0, 50)
	if err == nil || len(result.Items) != 0 {
		t.Fatalf("partial group returned: %+v %v", result, err)
	}
}

func TestEmptyChannelGroupReturnsEmptyPage(t *testing.T) {
	fixture := newChannelFixture(t)
	result, err := fixture.service.ChannelsByID(context.Background(), "primary", nil, 0, 50)
	if err != nil || result.Total != 0 || result.Items == nil {
		t.Fatalf("empty group failed: %+v %v", result, err)
	}
}
