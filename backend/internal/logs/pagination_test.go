package logs

import (
	"context"
	"math"
	"testing"
)

func TestLogsPageBeyondIntegerOffsetReturnsEmptyPage(t *testing.T) {
	service := New(fakeBusiness{}, fakeTasks{})
	page, err := service.Query(context.Background(), Query{Kind: "all", State: "all", Page: math.MaxInt, PageSize: 200})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 0 || page.Total != 0 {
		t.Fatalf("out-of-range page must be empty: %#v", page)
	}
}
