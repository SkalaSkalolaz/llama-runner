package proxy

import (
	"context"
	"testing"
)

func TestNoopSwapper(t *testing.T) {
	var s Swapper = NoopSwapper{}
	if _, err := s.SwapTo(context.Background(), "x"); err == nil {
		t.Fatal("expected error")
	}
	if s.CurrentAlias() != "" {
		t.Error("CurrentAlias != empty")
	}
	if len(s.CatalogAliases()) != 0 {
		t.Error("CatalogAliases != empty")
	}
	if err := s.WaitInflight(context.Background()); err != nil {
		t.Error(err)
	}
}
