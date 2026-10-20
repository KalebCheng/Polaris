package requestmeta

import (
	"context"
	"testing"
)

func TestFrom(t *testing.T) {
	want := Info{ClientIP: "203.0.113.7", Method: "POST", RequestURI: "/api/v1/users?x=1"}
	got := From(With(context.Background(), want))
	if got != want {
		t.Fatalf("request metadata = %+v, want %+v", got, want)
	}
	if got := From(context.Background()); got != (Info{}) {
		t.Fatalf("background context should have empty metadata, got %+v", got)
	}
}
